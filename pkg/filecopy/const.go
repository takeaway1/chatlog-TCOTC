package filecopy

import "time"

// Configuration constants for cache management and behavior tuning.
const (
	// CleanupInterval defines how often to run unified cleanup (1 minute).
	// First run is delayed by CleanupInterval after manager initialization.
	CleanupInterval = 1 * time.Minute

	// OrphanFileCleanupThreshold defines when orphaned files should be cleaned up (10 minutes).
	OrphanFileCleanupThreshold = 10 * time.Minute

	// MaxCacheEntries defines the maximum number of files to keep in the cache to prevent memory leaks.
	MaxCacheEntries = 10000 // Reasonable limit for most use cases

	// RecentFileProtectionWindow prevents deletion of recently modified/accessed files.
	RecentFileProtectionWindow = 2 * CleanupInterval

	// DedupSkipWindow skips version deduplication for very recent files during periodic cleanup.
	DedupSkipWindow = CleanupInterval

	// PathHashHexLen limits path-hash length in filenames (increase to lower collision risk).
	PathHashHexLen = 12

	// DataHashHexLen limits data-hash length in filenames.
	DataHashHexLen = 16

	// MaxBaseNameLen limits base filename length in temp file naming.
	MaxBaseNameLen = 100

	// LargeFileThreshold defines the size threshold for partial hashing (10MB).
	LargeFileThreshold = 10 * 1024 * 1024

	// lockShardSize defines the number of shards for path locking.
	lockShardSize = 256
)

// Version detection policy for generating data hash in file naming and cache keys.
type VersionDetectionMode int

const (
	// VersionDetectContentHash computes data hash from entire file content (strong consistency).
	VersionDetectContentHash VersionDetectionMode = iota
	// VersionDetectSizeModTime computes data hash from size+modtime only (faster, weaker consistency).
	VersionDetectSizeModTime
)

// VersionDetection controls how data hash is computed. Adjust as needed.
const VersionDetection = VersionDetectContentHash