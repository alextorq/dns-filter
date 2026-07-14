// Package snapshot provides the infrastructure adapter for exporting a
// sanitized SQLite database snapshot. It owns VACUUM INTO, temporary files,
// opening the copied database and scrubbing secret settings from that copy.
package snapshot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Exporter creates sanitized snapshots from one live SQLite connection. The
// secret-key set is resolved, validated and copied during construction so an
// incomplete settings registry fails at startup instead of leaking data during
// a request.
type Exporter struct {
	db         *gorm.DB
	secretKeys []string
}

// NewExporter constructs the production snapshot adapter. At least one valid
// secret key is required by this application: accepting a missing/empty
// provider would silently turn the security-sensitive download into an
// unsanitized database export.
func NewExporter(db *gorm.DB, secretKeys func() []string) (*Exporter, error) {
	if db == nil {
		return nil, fmt.Errorf("snapshot exporter: database is required")
	}
	if secretKeys == nil {
		return nil, fmt.Errorf("snapshot exporter: secret-key provider is required")
	}

	provided := secretKeys()
	if len(provided) == 0 {
		return nil, fmt.Errorf("snapshot exporter: secret-key set is empty")
	}
	keys := make([]string, 0, len(provided))
	seen := make(map[string]struct{}, len(provided))
	for _, key := range provided {
		if key == "" || strings.TrimSpace(key) != key {
			return nil, fmt.Errorf("snapshot exporter: invalid blank secret key")
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("snapshot exporter: secret-key set is empty")
	}
	return &Exporter{db: db, secretKeys: keys}, nil
}

// Export creates a consistent SQLite snapshot, removes every configured secret
// setting, compacts the copy to scrub freed pages, then returns a reader over
// the finished file. Closing the reader removes the entire private temporary
// directory, including any SQLite journal/WAL sidecars.
func (e *Exporter) Export(ctx context.Context) (content io.ReadCloser, size int64, err error) {
	if ctx == nil {
		return nil, 0, fmt.Errorf("export snapshot: context is required")
	}

	tmpDir, err := os.MkdirTemp("", "filter-snapshot-")
	if err != nil {
		return nil, 0, fmt.Errorf("create private snapshot directory: %w", err)
	}
	keepDir := false
	defer func() {
		if !keepDir {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	tmpPath := filepath.Join(tmpDir, "filter.sqlite")
	if err := snapshotWithoutSecrets(ctx, e.db, tmpPath, e.secretKeys); err != nil {
		return nil, 0, err
	}

	file, err := os.Open(tmpPath)
	if err != nil {
		return nil, 0, fmt.Errorf("open completed snapshot: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, 0, errors.Join(fmt.Errorf("stat completed snapshot: %w", err), file.Close())
	}

	keepDir = true
	return &cleanupReadCloser{file: file, dir: tmpDir}, info.Size(), nil
}

// snapshotWithoutSecrets makes an atomic copy of src via VACUUM INTO, deletes
// secret settings from the copy and runs a final VACUUM. The final rewrite is
// required because SQLite's default secure_delete=OFF can leave deleted values
// readable in free pages.
func snapshotWithoutSecrets(ctx context.Context, src *gorm.DB, dstPath string, secretKeys []string) (err error) {
	// SQLite accepts a literal expression, not a placeholder, after VACUUM INTO.
	// dstPath is server-controlled inside a fresh 0700 directory; quote escaping
	// additionally handles an apostrophe in an unusual TMPDIR path.
	escaped := strings.ReplaceAll(dstPath, "'", "''")
	if err := src.WithContext(ctx).Exec("VACUUM INTO '" + escaped + "'").Error; err != nil {
		return fmt.Errorf("vacuum into snapshot: %w", err)
	}

	dst, err := gorm.Open(sqlite.Open(dstPath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open snapshot database: %w", err)
	}
	sqlDB, err := dst.DB()
	if err != nil {
		return fmt.Errorf("access snapshot database handle: %w", err)
	}
	defer func() {
		if closeErr := sqlDB.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close snapshot database: %w", closeErr))
		}
	}()

	if err := dst.WithContext(ctx).Exec("DELETE FROM settings WHERE key IN ?", secretKeys).Error; err != nil {
		return fmt.Errorf("strip secrets from snapshot: %w", err)
	}
	if err := dst.WithContext(ctx).Exec("VACUUM").Error; err != nil {
		return fmt.Errorf("vacuum snapshot to scrub freed pages: %w", err)
	}
	return nil
}

// cleanupReadCloser couples the response stream to the lifetime of all export
// artifacts. Close is idempotent because HTTP error paths and callers may both
// attempt cleanup.
type cleanupReadCloser struct {
	file *os.File
	dir  string
	once sync.Once
	err  error
}

func (r *cleanupReadCloser) Read(p []byte) (int, error) { return r.file.Read(p) }

func (r *cleanupReadCloser) Close() error {
	r.once.Do(func() {
		r.err = errors.Join(r.file.Close(), os.RemoveAll(r.dir))
	})
	return r.err
}
