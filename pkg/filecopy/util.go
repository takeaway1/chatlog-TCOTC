package filecopy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cespare/xxhash"
)

func extractBaseName(originalPath string) string {
	fileName := filepath.Base(originalPath)
	fileExt := filepath.Ext(fileName)
	baseName := fileName
	if len(fileExt) > 0 && len(fileName) > len(fileExt) {
		baseName = fileName[:len(fileName)-len(fileExt)]
	}
	if baseName == "" || baseName == fileExt {
		baseName = "file"
	}
	return baseName
}

func hashString(s string) string {
	h := xxhash.New()
	// write never errors for xxhash
	_, _ = h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum64())
}

func hashFileContent(filePath string, size int64) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	h := xxhash.New()

	if size > LargeFileThreshold {
		// Partial hashing for large files: 1MB head + 1MB tail
		const chunkSize = 1024 * 1024

		// Read head
		if _, err := io.CopyN(h, file, chunkSize); err != nil && err != io.EOF {
			return "", err
		}

		// Seek to tail if file is large enough
		if size > 2*chunkSize {
			if _, err := file.Seek(-chunkSize, io.SeekEnd); err != nil {
				return "", err
			}
			if _, err := io.Copy(h, file); err != nil {
				return "", err
			}
		} else if size > chunkSize {
			// If file is between 1MB and 2MB, just read the rest
			if _, err := io.Copy(h, file); err != nil {
				return "", err
			}
		}
	} else {
		// Use xxhash for complete file hashing - benchmark shows 3.3x faster than SHA-256
		if _, err := io.Copy(h, file); err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("%x", h.Sum64()), nil
}

func getProcessName() string {
	executable, err := os.Executable()
	if err != nil {
		return "unknown"
	}

	// Extract base name (without extension)
	baseName := filepath.Base(executable)
	ext := filepath.Ext(baseName)
	if ext != "" {
		baseName = baseName[:len(baseName)-len(ext)]
	}

	// Sanitize name to contain only safe characters
	baseName = cleanProcessName(baseName)
	return baseName
}

// cleanProcessName sanitizes a process name by replacing invalid characters with underscores.
// Keeps only alphanumeric characters, hyphens, and underscores for filesystem safety.
func cleanProcessName(name string) string {
	result := make([]rune, 0, len(name))
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			result = append(result, r)
		} else {
			result = append(result, '_')
		}
	}
	return string(result)
}

func extractFileExtension(filePath string) string {
	ext := strings.TrimPrefix(filepath.Ext(filePath), ".")
	if ext == "" {
		return "bin"
	}
	return ext
}

// parseHashComponents splits combined hash into pathHash and dataHash
func parseHashComponents(combinedHash string) (pathHash, dataHash string) {
	parts := strings.Split(combinedHash, "_")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return "", ""
}

// declaredExtFromName extracts the declared extension (without dot) from a temp filename
// using the naming convention: instanceID_+baseName_+ext_+pathHash_+dataHash.ext
// Returns ext and true on success; otherwise false.
func declaredExtFromName(fileName string) (string, bool) {
	parts := strings.Split(fileName, "_+")
	if len(parts) < 5 {
		return "", false
	}
	// ext is the third from the end
	return parts[len(parts)-3], true
}