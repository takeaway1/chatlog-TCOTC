package filecopy

import (
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// FileIndexEntry represents an indexed temporary file with comprehensive metadata.
// It provides O(1) lookup and intelligent file lifecycle management.
// Thread-safe for concurrent access through atomic operations and mutex protection.
type FileIndexEntry struct {
	mu           sync.RWMutex // Protects concurrent access to mutable fields
	TempPath     string       // Path to the temporary file copy (immutable after creation)
	OriginalPath string       // Original source file path (protected by mu)
	Size         int64        // Size of the original file in bytes (immutable after creation)
	ModTime      time.Time    // Modification time of the original file (immutable after creation)
	lastAccess   int64        // Unix timestamp of most recent access (atomic)
	PathHash     string       // Path hash for collision detection (immutable after creation)
	DataHash     string       // Content hash for file integrity verification (immutable after creation)
	BaseName     string       // Base name for multi-version cleanup (immutable after creation)
	Extension    string       // File extension (normalized, without leading dot) (immutable after creation)
}

// GetLastAccess atomically retrieves the last access timestamp.
func (e *FileIndexEntry) GetLastAccess() time.Time {
	return time.Unix(atomic.LoadInt64(&e.lastAccess), 0)
}

// SetLastAccess atomically updates the last access timestamp.
func (e *FileIndexEntry) SetLastAccess(t time.Time) {
	atomic.StoreInt64(&e.lastAccess, t.Unix())
}

// GetOriginalPath safely retrieves the original file path.
func (e *FileIndexEntry) GetOriginalPath() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.OriginalPath
}

// SetOriginalPath safely updates the original file path.
func (e *FileIndexEntry) SetOriginalPath(path string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.OriginalPath = path
}

// indexCandidate represents a potential file to be added to the index during reconstruction.
// It holds parsed metadata from a temporary filename.
type indexCandidate struct {
	filePath  string
	baseName  string
	ext       string
	hash      string // Combined hash (pathHash_dataHash)
	timestamp time.Time
	fileInfo  os.FileInfo
}

// toIndexEntry converts a candidate to a full index entry.
func (c *indexCandidate) toIndexEntry() *FileIndexEntry {
	pathHash, dataHash := parseHashComponents(c.hash)

	entry := &FileIndexEntry{
		TempPath:     c.filePath,
		OriginalPath: "", // Unknown during reconstruction, will be populated on first access if possible
		Size:         c.fileInfo.Size(),
		ModTime:      c.fileInfo.ModTime(), // Use temp file modtime as proxy
		PathHash:     pathHash,
		DataHash:     dataHash,
		BaseName:     c.baseName,
		Extension:    c.ext,
	}
	entry.SetLastAccess(c.timestamp)
	return entry
}