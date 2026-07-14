package snapshot

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func closeTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}
}

func TestNewExporter_RejectsUnsafeDependencies(t *testing.T) {
	db := openTestDB(t, filepath.Join(t.TempDir(), "source.sqlite"))
	t.Cleanup(func() { closeTestDB(t, db) })

	tests := []struct {
		name     string
		db       *gorm.DB
		provider func() []string
	}{
		{name: "missing database", provider: func() []string { return []string{"secret"} }},
		{name: "missing secret provider", db: db},
		{name: "empty secret list", db: db, provider: func() []string { return nil }},
		{name: "blank secret key", db: db, provider: func() []string { return []string{" "} }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewExporter(tc.db, tc.provider); err == nil {
				t.Fatal("expected constructor error")
			}
		})
	}
}

func TestExporter_ExportSanitizesSecretsAndKeepsPublicSettings(t *testing.T) {
	srcPath := filepath.Join(t.TempDir(), "source.sqlite")
	db := openTestDB(t, srcPath)
	t.Cleanup(func() { closeTestDB(t, db) })

	if err := db.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)").Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	const (
		virusTotalSecret   = "vt-super-secret-value"
		safeBrowsingSecret = "sb-super-secret-value"
	)
	for key, value := range map[string]string{
		"virustotal_key":   virusTotalSecret,
		"safebrowsing_key": safeBrowsingSecret,
		"log_level":        "WARN",
	} {
		if err := db.Exec("INSERT INTO settings(key, value) VALUES (?, ?)", key, value).Error; err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
	}

	providerCalls := 0
	exporter, err := NewExporter(db, func() []string {
		providerCalls++
		return []string{"virustotal_key", "safebrowsing_key"}
	})
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}
	if providerCalls != 1 {
		t.Fatalf("secret provider calls = %d, want 1 during construction", providerCalls)
	}

	content, size, err := exporter.Export(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	snapshotBytes, err := io.ReadAll(content)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if int64(len(snapshotBytes)) != size {
		t.Fatalf("snapshot size = %d, read %d bytes", size, len(snapshotBytes))
	}
	if bytes.Contains(snapshotBytes, []byte(virusTotalSecret)) || bytes.Contains(snapshotBytes, []byte(safeBrowsingSecret)) {
		t.Fatal("secret bytes remain in exported snapshot")
	}
	if err := content.Close(); err != nil {
		t.Fatalf("close snapshot: %v", err)
	}

	downloadPath := filepath.Join(t.TempDir(), "download.sqlite")
	if err := os.WriteFile(downloadPath, snapshotBytes, 0o600); err != nil {
		t.Fatalf("write downloaded snapshot: %v", err)
	}
	snapshotDB := openTestDB(t, downloadPath)
	defer closeTestDB(t, snapshotDB)

	var secretCount, publicCount, sourceSecretCount int64
	if err := snapshotDB.Table("settings").Where("key IN ?", []string{"virustotal_key", "safebrowsing_key"}).Count(&secretCount).Error; err != nil {
		t.Fatalf("count snapshot secret: %v", err)
	}
	if err := snapshotDB.Table("settings").Where("key = ? AND value = ?", "log_level", "WARN").Count(&publicCount).Error; err != nil {
		t.Fatalf("count public setting: %v", err)
	}
	if err := db.Table("settings").Where("key IN ?", []string{"virustotal_key", "safebrowsing_key"}).Count(&sourceSecretCount).Error; err != nil {
		t.Fatalf("count source secret: %v", err)
	}
	if secretCount != 0 || publicCount != 1 || sourceSecretCount != 2 {
		t.Fatalf("rows: snapshot secret=%d public=%d source secret=%d", secretCount, publicCount, sourceSecretCount)
	}
}

func TestExporter_CloseRemovesPrivateTemporaryDirectory(t *testing.T) {
	db := openTestDB(t, filepath.Join(t.TempDir(), "source.sqlite"))
	t.Cleanup(func() { closeTestDB(t, db) })
	if err := db.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)").Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}

	exporter, err := NewExporter(db, func() []string { return []string{"secret_key"} })
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}
	content, _, err := exporter.Export(context.Background())
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	managed, ok := content.(*cleanupReadCloser)
	if !ok {
		t.Fatalf("snapshot content type = %T, want managed cleanup reader", content)
	}
	if info, err := os.Stat(managed.dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("private temp dir: info=%v err=%v", info, err)
	}
	if err := content.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := content.Close(); err != nil {
		t.Fatalf("idempotent close: %v", err)
	}
	if _, err := os.Stat(managed.dir); !os.IsNotExist(err) {
		t.Fatalf("temporary directory still exists after Close: %v", err)
	}
}

func TestExporter_SnapshotFailureReturnsNoContent(t *testing.T) {
	tmpRoot := t.TempDir()
	t.Setenv("TMPDIR", tmpRoot)
	db := openTestDB(t, filepath.Join(t.TempDir(), "source.sqlite"))
	closeTestDB(t, db)
	exporter, err := NewExporter(db, func() []string { return []string{"secret_key"} })
	if err != nil {
		t.Fatalf("NewExporter: %v", err)
	}

	content, size, err := exporter.Export(context.Background())

	if err == nil || !strings.Contains(err.Error(), "vacuum into snapshot") {
		t.Fatalf("expected VACUUM failure, got %v", err)
	}
	if content != nil || size != 0 {
		t.Fatalf("failed export returned content=%v size=%d", content, size)
	}
	entries, readErr := os.ReadDir(tmpRoot)
	if readErr != nil {
		t.Fatalf("read temp root: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed export leaked temporary artifacts: %v", entries)
	}
}
