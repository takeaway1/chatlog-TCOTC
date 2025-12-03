package filecopy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// GetTempCopy is a convenience wrapper for the default manager.
func GetTempCopy(originalPath string) (string, error) {
	return getManager().GetTempCopy(originalPath)
}

// GetTempCopy creates or retrieves a temporary copy of the specified file.
// It handles caching, versioning, and concurrent access automatically.
func (fm *FileCopyManager) GetTempCopy(originalPath string) (string, error) {
	// 1. Input validation
	if originalPath == "" {
		return "", fmt.Errorf("empty path provided")
	}

	// 2. Check source file existence and get metadata
	info, err := os.Stat(originalPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("source path is a directory: %s", originalPath)
	}

	// 3. Generate cache keys and paths
	// This step might involve hashing the file content if VersionDetectContentHash is used
	tempPath, pathHash, dataHash, err := fm.generateTempPath(originalPath, info)
	if err != nil {
		return "", err
	}

	cacheKey := fm.generateCacheKey(pathHash, dataHash)

	// 4. Check cache (Fast path)
	if val, ok := fm.fileIndex.Load(cacheKey); ok {
		entry := val.(*FileIndexEntry)
		// Verify file actually exists on disk
		if _, err := os.Stat(entry.TempPath); err == nil {
			entry.SetLastAccess(time.Now())
			// Update original path if it changed (e.g. different source file with same content)
			if entry.GetOriginalPath() != originalPath {
				entry.SetOriginalPath(originalPath)
			}
			return entry.TempPath, nil
		}
		// Cache entry exists but file is missing - remove invalid entry
		fm.fileIndex.Delete(cacheKey)
		atomic.AddInt64(&fm.cacheSize, -1)
	}

	// 5. Acquire lock for this specific file path to prevent thundering herd
	// We lock based on the destination path to allow concurrent copies of different files
	lock := fm.getPathLock(tempPath)
	lock.Lock()
	defer lock.Unlock()

	// 6. Double-check cache after acquiring lock
	if val, ok := fm.fileIndex.Load(cacheKey); ok {
		entry := val.(*FileIndexEntry)
		if _, err := os.Stat(entry.TempPath); err == nil {
			entry.SetLastAccess(time.Now())
			return entry.TempPath, nil
		}
		fm.fileIndex.Delete(cacheKey)
		atomic.AddInt64(&fm.cacheSize, -1)
	}

	// 7. Perform copy
	if err := fm.atomicCopyFile(originalPath, tempPath, info); err != nil {
		return "", fmt.Errorf("failed to copy file: %w", err)
	}

	// 8. Update cache
	baseName := extractBaseName(originalPath)
	ext := extractFileExtension(originalPath)

	entry := &FileIndexEntry{
		TempPath:     tempPath,
		OriginalPath: originalPath,
		Size:         info.Size(),
		ModTime:      info.ModTime(),
		PathHash:     pathHash,
		DataHash:     dataHash,
		BaseName:     baseName,
		Extension:    ext,
	}
	entry.SetLastAccess(time.Now())

	fm.fileIndex.Store(cacheKey, entry)
	atomic.AddInt64(&fm.cacheSize, 1)

	// 9. Trigger cleanup if cache is too large (async)
	if atomic.LoadInt64(&fm.cacheSize) > MaxCacheEntries {
		select {
		case <-fm.ctx.Done():
			// Context cancelled, skip cleanup
		default:
			go fm.performCacheCleanup()
		}
	}

	return tempPath, nil
}

// getPathLock returns a mutex for the given path using sharding.
func (fm *FileCopyManager) getPathLock(path string) *sync.Mutex {
	// Simple hash of the path to select a lock shard
	h := uint32(0)
	for i := 0; i < len(path); i++ {
		h = 31*h + uint32(path[i])
	}
	return &fm.locks[h%lockShardSize]
}

// generateTempPath creates a unique temporary path and returns associated hashes.
func (fm *FileCopyManager) generateTempPath(originalPath string, info os.FileInfo) (string, string, string, error) {
	// Generate path hash (identifies the source path)
	pathHash := hashString(originalPath)
	if len(pathHash) > PathHashHexLen {
		pathHash = pathHash[:PathHashHexLen]
	}

	// Generate data hash (identifies the content version)
	var dataHash string
	var err error

	if VersionDetection == VersionDetectContentHash {
		// Strong consistency: hash the content
		dataHash, err = hashFileContent(originalPath, info.Size())
		if err != nil {
			return "", "", "", err
		}
	} else {
		// Weak consistency: hash metadata
		metaString := fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano())
		dataHash = hashString(metaString)
	}

	if len(dataHash) > DataHashHexLen {
		dataHash = dataHash[:DataHashHexLen]
	}

	baseName := extractBaseName(originalPath)
	ext := extractFileExtension(originalPath)

	tempPath := fm.generateTempPathWithHash(originalPath, baseName, ext, pathHash, dataHash)
	return tempPath, pathHash, dataHash, nil
}

// generateTempPathWithHash constructs the final temporary file path.
// Format: {tempDir}/{instanceID}_+{baseName}_+{ext}_+{pathHash}_+{dataHash}.{ext}
// The "_+" separator is used to safely parse components later.
func (fm *FileCopyManager) generateTempPathWithHash(originalPath, baseName, ext, pathHash, dataHash string) string {
	// Sanitize baseName
	if len(baseName) > MaxBaseNameLen {
		baseName = baseName[:MaxBaseNameLen]
	}
	baseName = strings.ReplaceAll(baseName, "_+", "_") // Avoid confusion with separator

	fileName := fmt.Sprintf("%s_+%s_+%s_+%s_+%s.%s",
		fm.instanceID, baseName, ext, pathHash, dataHash, ext)

	return filepath.Join(fm.tempDir, fileName)
}

// atomicCopyFile copies a file atomically by writing to a temp file and renaming.
func (fm *FileCopyManager) atomicCopyFile(src, dst string, info os.FileInfo) error {
	// Create temp file in the same directory to ensure atomic rename works
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(dir, "copy_tmp_*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()

	// Ensure cleanup in case of error
	defer func() {
		_ = tmpFile.Close()
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Use io.CopyBuffer for potentially better performance with large files
	// 32KB buffer is standard for io.Copy, but we can be explicit
	buf := make([]byte, 32*1024)
	if _, err := io.CopyBuffer(tmpFile, srcFile, buf); err != nil {
		return err
	}

	// Sync to ensure data is on disk
	if err := tmpFile.Sync(); err != nil {
		return err
	}

	// Close explicitly to check for errors
	if err := tmpFile.Close(); err != nil {
		return err
	}

	// Preserve modification time
	if err := os.Chtimes(tmpName, time.Now(), info.ModTime()); err != nil {
		log.Warn().Err(err).Str("file", tmpName).Msg("Failed to set modtime on temp file")
	}

	// Atomic rename
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}

	return nil
}

// generateCacheKey creates a unique key for the cache map.
func (fm *FileCopyManager) generateCacheKey(pathHash, dataHash string) string {
	return pathHash + "_" + dataHash
}

// generateVersionKey creates a key for version deduplication (pathHash only).
func (fm *FileCopyManager) generateVersionKey(pathHash string) string {
	return pathHash
}