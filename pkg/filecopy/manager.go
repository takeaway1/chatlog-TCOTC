package filecopy

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// PendingDeletion represents a file that failed to delete and should be retried later.
type PendingDeletion struct {
	Path      string
	AddedAt   time.Time
	Attempts  int
	LastError string
}

// Manager instances per instanceID for proper isolation.
var (
	managers   = make(map[string]*FileCopyManager)
	managersMu sync.RWMutex
)

// FileCopyManager manages temporary file copies with persistent caching capabilities.
// It provides thread-safe operations for creating, accessing, and cleaning up temporary files.
type FileCopyManager struct {
	instanceID string                    // Instance identifier for this manager
	tempDir    string                    // Base directory for storing temporary files
	fileIndex  sync.Map                  // File index: key -> *FileIndexEntry (thread-safe)
	lastAccess time.Time                 // Last access time for TTL cleanup
	startTime  time.Time                 // Manager initialization time
	ctx        context.Context           // Context for goroutine lifecycle management
	cancel     context.CancelFunc        // Cancel function for graceful shutdown
	wg         sync.WaitGroup            // WaitGroup for goroutine synchronization
	cacheSize  int64                     // Current number of cached entries (atomic)
	locks      [lockShardSize]sync.Mutex // Striped locks to prevent duplicate concurrent copies

	// Pending deletion queue for files that failed to delete (e.g., still in use)
	pendingDeletions   map[string]*PendingDeletion
	pendingDeletionsMu sync.Mutex
}

// getManager retrieves or creates a FileCopyManager for the current process instance.
// It ensures thread-safe singleton access per instance ID.
func getManager() *FileCopyManager {
	instanceID := getProcessName()

	managersMu.RLock()
	manager, exists := managers[instanceID]
	managersMu.RUnlock()

	if exists {
		return manager
	}

	managersMu.Lock()
	defer managersMu.Unlock()

	// Double-check locking
	if manager, exists = managers[instanceID]; exists {
		return manager
	}

	manager = newManager(instanceID)
	managers[instanceID] = manager
	return manager
}

// newManager initializes a new FileCopyManager with a dedicated temp directory.
// It starts the background cleanup worker.
func newManager(instanceID string) *FileCopyManager {
	// Use system temp dir with a subdirectory for this application
	tempDir := filepath.Join(os.TempDir(), "chatlog_cache", instanceID)

	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Error().Err(err).Msg("Failed to create temp directory for file copy manager")
		// Fallback to system temp without subdir if creation fails
		tempDir = os.TempDir()
	}

	ctx, cancel := context.WithCancel(context.Background())

	fm := &FileCopyManager{
		instanceID:       instanceID,
		tempDir:          tempDir,
		startTime:        time.Now(),
		lastAccess:       time.Now(),
		ctx:              ctx,
		cancel:           cancel,
		pendingDeletions: make(map[string]*PendingDeletion),
	}

	// Initialize locks
	for i := 0; i < lockShardSize; i++ {
		fm.locks[i] = sync.Mutex{}
	}

	// Start background cleanup worker
	fm.wg.Add(1)
	go fm.periodicCleanupWorker()

	log.Info().Str("instanceID", instanceID).Str("dir", tempDir).Msg("Initialized FileCopyManager")
	return fm
}

// Shutdown gracefully stops all managers and waits for background workers.
func Shutdown() {
	managersMu.Lock()
	defer managersMu.Unlock()

	var wg sync.WaitGroup
	for id, manager := range managers {
		wg.Add(1)
		go func(m *FileCopyManager) {
			defer wg.Done()
			m.Shutdown()
		}(manager)
		delete(managers, id)
	}
	wg.Wait()
}

// Shutdown stops the manager's background tasks and releases resources.
func (fm *FileCopyManager) Shutdown() {
	fm.cancel()
	fm.wg.Wait()
	log.Info().Str("instanceID", fm.instanceID).Msg("FileCopyManager shut down")
}

// NotifyFileReleased should be called when a consumer (e.g., SQLite connection) releases a temp file.
// This allows filecopy to immediately attempt deletion of pending files.
func NotifyFileReleased(tempPath string) {
	getManager().NotifyFileReleased(tempPath)
}

// NotifyFileReleased handles notification that a temp file has been released by its consumer.
func (fm *FileCopyManager) NotifyFileReleased(tempPath string) {
	fm.pendingDeletionsMu.Lock()
	pending, exists := fm.pendingDeletions[tempPath]
	if exists {
		delete(fm.pendingDeletions, tempPath)
	}
	fm.pendingDeletionsMu.Unlock()

	if exists {
		log.Debug().Str("path", tempPath).Msg("File released, attempting cleanup")
		if err := os.Remove(tempPath); err != nil && !os.IsNotExist(err) {
			log.Debug().Err(err).Str("path", tempPath).Msg("Failed to remove released file")
		} else {
			log.Debug().Str("path", tempPath).Msg("Successfully removed released file")
		}
	}

	// Also try to remove associated SQLite sidecar files
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		sidecarPath := tempPath + suffix
		_ = os.Remove(sidecarPath) // Best effort, ignore errors
	}

	// Try to remove from index if present
	if pending != nil {
		fm.fileIndex.Range(func(key, value interface{}) bool {
			entry := value.(*FileIndexEntry)
			if entry.TempPath == tempPath {
				fm.fileIndex.Delete(key)
				return false
			}
			return true
		})
	}
}

// AddPendingDeletion adds a file to the pending deletion queue.
func (fm *FileCopyManager) AddPendingDeletion(path string, err error) {
	fm.pendingDeletionsMu.Lock()
	defer fm.pendingDeletionsMu.Unlock()

	if existing, ok := fm.pendingDeletions[path]; ok {
		existing.Attempts++
		existing.LastError = err.Error()
	} else {
		fm.pendingDeletions[path] = &PendingDeletion{
			Path:      path,
			AddedAt:   time.Now(),
			Attempts:  1,
			LastError: err.Error(),
		}
	}
}

// ProcessPendingDeletions attempts to delete files in the pending queue.
func (fm *FileCopyManager) ProcessPendingDeletions() {
	fm.pendingDeletionsMu.Lock()
	toProcess := make([]*PendingDeletion, 0, len(fm.pendingDeletions))
	for _, pd := range fm.pendingDeletions {
		toProcess = append(toProcess, pd)
	}
	fm.pendingDeletionsMu.Unlock()

	for _, pd := range toProcess {
		if err := os.Remove(pd.Path); err != nil {
			if os.IsNotExist(err) {
				// File already gone, remove from queue
				fm.pendingDeletionsMu.Lock()
				delete(fm.pendingDeletions, pd.Path)
				fm.pendingDeletionsMu.Unlock()
			} else {
				// Still in use, update attempt count
				fm.pendingDeletionsMu.Lock()
				if existing, ok := fm.pendingDeletions[pd.Path]; ok {
					existing.Attempts++
					// If too many attempts and file is old enough, log and eventually give up
					if existing.Attempts > 10 && time.Since(existing.AddedAt) > OrphanFileCleanupThreshold {
						log.Debug().Str("path", pd.Path).Int("attempts", existing.Attempts).Msg("Giving up on pending deletion")
						delete(fm.pendingDeletions, pd.Path)
					}
				}
				fm.pendingDeletionsMu.Unlock()
			}
		} else {
			// Successfully deleted
			log.Debug().Str("path", pd.Path).Msg("Successfully deleted pending file")
			fm.pendingDeletionsMu.Lock()
			delete(fm.pendingDeletions, pd.Path)
			fm.pendingDeletionsMu.Unlock()

			// Also try sidecar files
			for _, suffix := range []string{"-wal", "-shm", "-journal"} {
				_ = os.Remove(pd.Path + suffix)
			}
		}
	}
}

// GetPendingDeletionCount returns the number of files waiting to be deleted.
func (fm *FileCopyManager) GetPendingDeletionCount() int {
	fm.pendingDeletionsMu.Lock()
	defer fm.pendingDeletionsMu.Unlock()
	return len(fm.pendingDeletions)
}