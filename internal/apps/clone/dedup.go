// Package clone provides website cloning functionality.
//
// dedup.go: Content-based deduplication for cloned pages.
// Uses SHA-256 hashing to detect identical page content, and creates
// hard links instead of duplicate files to save disk space.
// This is particularly effective for URLs with query parameters that
// return the same content (e.g., ?page=1 vs ?page=2 on listing pages).
package clone

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
)

// ContentDeduper manages content-based deduplication using SHA-256 hashes.
// Thread-safe for concurrent use.
type ContentDeduper struct {
	mu       sync.RWMutex
	hashes   map[string]string // SHA-256 hex → first file path.
	savings  int64             // Total bytes saved by dedup.
	count    int               // Number of deduplicated files.
	disabled bool
}

// NewContentDeduper creates a new content deduper.
// If disabled is true, all operations become no-ops.
func NewContentDeduper(disabled bool) *ContentDeduper {
	return &ContentDeduper{
		hashes:   make(map[string]string),
		disabled: disabled,
	}
}

// TryDedup checks if identical content already exists on disk.
// If so, it creates a hard link from targetPath to the existing file
// and returns true. Otherwise, it records this content's hash and
// returns false (caller should write the file normally).
//
// Caller must provide the content bytes and the target file path.
// The content hash is computed and checked against known content.
func (cd *ContentDeduper) TryDedup(content []byte, targetPath string) (bool, error) {
	if cd.disabled {
		return false, nil
	}

	hash := sha256Hex(content)

	cd.mu.Lock()
	defer cd.mu.Unlock()

	// Check if identical content already exists.
	if existingPath, exists := cd.hashes[hash]; exists {
		// Verify the source file still exists.
		if _, err := os.Stat(existingPath); err != nil {
			// Source file gone, update map and treat as new.
			delete(cd.hashes, hash)
			return false, nil
		}

		// Remove target if it already exists (don't want stale data).
		os.Remove(targetPath)

		// Create hard link.
		if err := os.Link(existingPath, targetPath); err != nil {
			return false, fmt.Errorf("hard link: %w", err)
		}

		cd.savings += int64(len(content))
		cd.count++

		return true, nil
	}

	// First time seeing this content; record it.
	cd.hashes[hash] = targetPath
	return false, nil
}

// MarkWritten records a hash→path mapping for file that was already written
// to disk by the caller. Call after TryDedup returned false and the file
// was written successfully.
func (cd *ContentDeduper) MarkWritten(content []byte, targetPath string) {
	if cd.disabled {
		return
	}
	hash := sha256Hex(content)
	cd.mu.Lock()
	cd.hashes[hash] = targetPath
	cd.mu.Unlock()
}

// Savings returns the total bytes saved and the number of deduplicated files.
func (cd *ContentDeduper) Savings() (filesDeduped int, bytesSaved int64) {
	cd.mu.RLock()
	defer cd.mu.RUnlock()
	return cd.count, cd.savings
}

// Stats returns a human-readable summary of dedup statistics.
func (cd *ContentDeduper) Stats() string {
	if cd.disabled {
		return "dedup: disabled"
	}
	cd.mu.RLock()
	defer cd.mu.RUnlock()
	return fmt.Sprintf("dedup: %d files, %s saved",
		cd.count, humanBytes(cd.savings))
}

// Reset clears all dedup state.
func (cd *ContentDeduper) Reset() {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	cd.hashes = make(map[string]string)
	cd.savings = 0
	cd.count = 0
}

// sha256Hex computes the hex-encoded SHA-256 hash of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// humanBytes formats a byte count in human-readable form.
func humanBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	if n < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
}
