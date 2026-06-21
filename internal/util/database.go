// Package util provides shared utility types and helpers.
package util

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/mattn/go-sqlite3"
)

// DatabasePool provides a shared SQLite connection pool that multiple
// subsystems (session, memory, todo, recall) can use without opening
// separate connections to the same database file.
//
// SQLite supports multiple connections, but each connection maintains
// its own cache and write-ahead log state. Sharing a single pool
// simplifies lifecycle management and reduces resource contention.
type DatabasePool struct {
	mu sync.Mutex
	db *sql.DB

	path string
}

// NewDatabasePool creates a new shared database pool.
// The underlying database connection is opened lazily on first use.
func NewDatabasePool(path string) *DatabasePool {
	return &DatabasePool{path: path}
}

// GetDB returns the shared database connection, opening it if necessary.
// This is safe for concurrent use. The caller must NOT close the returned
// *sql.DB; lifecycle is managed by DatabasePool.Close.
func (p *DatabasePool) GetDB() (*sql.DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.db != nil {
		return p.db, nil
	}

	db, err := sql.Open("sqlite3", p.path+
		"?_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=ON"+
		"&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite pool %q: %w", p.path, err)
	}

	// Allow up to 4 concurrent connections in WAL mode.
	// WAL supports multiple readers + one writer with page-level
	// locking. _busy_timeout=5000ms handles transient lock contention
	// by waiting instead of immediately returning SQLITE_BUSY.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)

	// Apply PRAGMAs (DSN params handle most, these are safety nets).
	_, _ = db.Exec("PRAGMA journal_mode=WAL")
	_, _ = db.Exec("PRAGMA foreign_keys=ON")
	_, _ = db.Exec("PRAGMA synchronous=NORMAL")
	_, _ = db.Exec("PRAGMA busy_timeout=5000")

	p.db = db
	return db, nil
}

// Close closes the shared database connection after performing a
// WAL checkpoint to flush all pending writes to the main database
// file. This ensures zero data loss on graceful shutdown.
//
// If the database has already been closed by another consumer
// (shared-pool mode), the checkpoint is silently skipped. WAL data
// is still safe and will be replayed on next open.
//
// It is safe to call multiple times; subsequent calls are no-ops.
func (p *DatabasePool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.db == nil {
		return nil
	}

	// Only checkpoint if the database is still open. In shared-pool
	// mode, another service (session/memory) may have already closed
	// the underlying *sql.DB. The WAL will be replayed automatically
	// when the database is next opened.
	if err := p.db.Ping(); err == nil {
		// Flush WAL to main database file for zero-loss shutdown.
		_, ckErr := p.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		if ckErr != nil {
			Logger.Warn("WAL checkpoint before close failed "+
				"(non-fatal: WAL will replay on next open)",
				"error", ckErr.Error())
		}
	}

	err := p.db.Close()
	p.db = nil
	return err
}
