package filecopy

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

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
		instanceID: instanceID,
		tempDir:    tempDir,
		startTime:  time.Now(),
		lastAccess: time.Now(),
		ctx:        ctx,
		cancel:     cancel,
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