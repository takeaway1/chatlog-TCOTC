package filecopy

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// periodicCleanupWorker runs in the background to clean up old files.
func (fm *FileCopyManager) periodicCleanupWorker() {
	defer fm.wg.Done()

	// Initial delay to avoid contention during startup
	select {
	case <-time.After(CleanupInterval):
	case <-fm.ctx.Done():
		return
	}

	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()

	// Run immediately after initial delay
	fm.rebuildIndexAndCleanup()

	for {
		select {
		case <-ticker.C:
			fm.rebuildIndexAndCleanup()
		case <-fm.ctx.Done():
			return
		}
	}
}

// processDeletionInline removes a file from disk and updates the cache size.
func (fm *FileCopyManager) processDeletionInline(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Str("path", path).Msg("Failed to remove file during cleanup")
	}
}

// rebuildIndexAndCleanup scans the temp directory to rebuild the in-memory index
// and removes orphaned or outdated files.
func (fm *FileCopyManager) rebuildIndexAndCleanup() {
	// We use a map to track the latest version of each file (by pathHash)
	// key: pathHash, value: *indexCandidate
	latestVersions := make(map[string]*indexCandidate)

	// Files that don't belong to us or are malformed
	var filesToDelete []string

	// Current time for TTL checks
	now := time.Now()

	entries, err := os.ReadDir(fm.tempDir)
	if err != nil {
		log.Error().Err(err).Str("dir", fm.tempDir).Msg("Failed to read temp directory")
		return
	}

	// 1. Scan directory
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		name := entry.Name()
		path := filepath.Join(fm.tempDir, name)

		// Check if file belongs to this instance
		if !strings.HasPrefix(name, fm.instanceID+"_+") {
			// File from another instance or legacy file
			// Check if it's old enough to be considered orphaned
			if now.Sub(info.ModTime()) > OrphanFileCleanupThreshold {
				filesToDelete = append(filesToDelete, path)
			}
			continue
		}

		// Parse filename
		// Format: {instanceID}_+{baseName}_+{ext}_+{pathHash}_+{dataHash}.{ext}
		// We need to extract components.
		parts := strings.Split(name, "_+")
		if len(parts) < 5 {
			// Malformed, delete if old
			if now.Sub(info.ModTime()) > OrphanFileCleanupThreshold {
				filesToDelete = append(filesToDelete, path)
			}
			continue
		}

		// parts[0] is instanceID (checked above)
		baseName := parts[1]
		ext := parts[2]
		pathHash := parts[3]
		// parts[4] is dataHash.ext

		// Extract dataHash
		lastPart := parts[4]
		dataHash := strings.TrimSuffix(lastPart, "."+ext)

		// Reconstruct combined hash for index
		combinedHash := pathHash + "_" + dataHash

		candidate := &indexCandidate{
			filePath:  path,
			baseName:  baseName,
			ext:       ext,
			hash:      combinedHash,
			timestamp: info.ModTime(), // Use modtime as last access proxy
			fileInfo:  info,
		}

		// Check if we already have this file in our index
		if _, ok := fm.fileIndex.Load(combinedHash); ok {
			// Already indexed, just update latestVersions logic
			// If we have multiple files for same pathHash, we want to keep the newest
		} else {
			// Not in index, add it
			fm.fileIndex.Store(combinedHash, candidate.toIndexEntry())
			atomic.AddInt64(&fm.cacheSize, 1)
		}

		// Version deduplication logic
		// We want to keep only the latest version for each pathHash
		existing, exists := latestVersions[pathHash]
		if !exists || candidate.timestamp.After(existing.timestamp) {
			// If we are replacing an existing candidate, the old one is a candidate for deletion
			if exists != false {
				// Only delete if it's not protected by RecentFileProtectionWindow
				// AND not protected by DedupSkipWindow (very recent files might be in use)
				if now.Sub(existing.timestamp) > DedupSkipWindow {
					filesToDelete = append(filesToDelete, existing.filePath)
					// Also remove from index
					fm.fileIndex.Delete(existing.hash)
					atomic.AddInt64(&fm.cacheSize, -1)
				}
			}
			latestVersions[pathHash] = candidate
		} else {
			// This candidate is older than what we have
			if now.Sub(candidate.timestamp) > DedupSkipWindow {
				filesToDelete = append(filesToDelete, candidate.filePath)
				fm.fileIndex.Delete(candidate.hash)
				atomic.AddInt64(&fm.cacheSize, -1)
			}
		}
	}

	// 2. Process deletions
	for _, path := range filesToDelete {
		fm.processDeletionInline(path)
	}

	// 3. Check cache size limits
	if atomic.LoadInt64(&fm.cacheSize) > MaxCacheEntries {
		fm.performCacheCleanup()
	}
}

// performCacheCleanup enforces the maximum cache size by removing least recently used items.
func (fm *FileCopyManager) performCacheCleanup() {
	type cacheEntry struct {
		key        string
		lastAccess time.Time
		entry      *FileIndexEntry
	}

	var entries []cacheEntry

	// Snapshot the index
	fm.fileIndex.Range(func(key, value interface{}) bool {
		entry := value.(*FileIndexEntry)
		entries = append(entries, cacheEntry{
			key:        key.(string),
			lastAccess: entry.GetLastAccess(),
			entry:      entry,
		})
		return true
	})

	// If we are within limits, do nothing
	if int64(len(entries)) <= MaxCacheEntries {
		atomic.StoreInt64(&fm.cacheSize, int64(len(entries)))
		return
	}

	// Sort by last access time (oldest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lastAccess.Before(entries[j].lastAccess)
	})

	// Calculate how many to remove
	toRemove := int64(len(entries)) - MaxCacheEntries
	// Remove at least 10% of max to avoid frequent cleanups
	if toRemove < MaxCacheEntries/10 {
		toRemove = MaxCacheEntries / 10
	}

	now := time.Now()
	deletedCount := 0

	for i := 0; i < len(entries) && int64(deletedCount) < toRemove; i++ {
		e := entries[i]

		// Skip recently accessed files (protection window)
		if now.Sub(e.lastAccess) < RecentFileProtectionWindow {
			continue
		}

		// Delete file
		fm.processDeletionInline(e.entry.TempPath)

		// Remove from index
		fm.fileIndex.Delete(e.key)
		deletedCount++
	}

	// Update cache size
	atomic.AddInt64(&fm.cacheSize, int64(-deletedCount))
}
