package web

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type testLogger struct {
	errs  []error
	infos []string
}

func (l *testLogger) Error(err error) { l.errs = append(l.errs, err) }
func (l *testLogger) Info(args ...any) {
	l.infos = append(l.infos, fmt.Sprint(args...))
}

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

func TestDownloadDb_SanitizesSecretSettings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	srcPath := filepath.Join(t.TempDir(), "source.sqlite")
	db := openTestDB(t, srcPath)
	t.Cleanup(func() { closeTestDB(t, db) })

	if err := db.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)").Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	const secret = "vt-super-secret-value"
	for key, value := range map[string]string{
		"virustotal_key": secret,
		"log_level":      "WARN",
	} {
		if err := db.Exec("INSERT INTO settings(key, value) VALUES (?, ?)", key, value).Error; err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
	}

	log := &testLogger{}
	providerCalls := 0
	h := &Handlers{
		DB:     db,
		DBPath: srcPath,
		Log:    log,
		SecretKeys: func() []string {
			providerCalls++
			return []string{"virustotal_key"}
		},
	}
	r := gin.New()
	r.GET("/download", h.DownloadDb)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/download", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if providerCalls != 1 {
		t.Fatalf("secret provider calls = %d, want 1", providerCalls)
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "filter.sqlite") {
		t.Fatalf("missing attachment filename: %q", w.Header().Get("Content-Disposition"))
	}
	if bytes.Contains(w.Body.Bytes(), []byte(secret)) {
		t.Fatal("secret bytes remain in downloaded snapshot")
	}

	snapshotPath := filepath.Join(t.TempDir(), "download.sqlite")
	if err := os.WriteFile(snapshotPath, w.Body.Bytes(), 0o600); err != nil {
		t.Fatalf("write response: %v", err)
	}
	snapshot := openTestDB(t, snapshotPath)
	defer closeTestDB(t, snapshot)

	var secretCount, publicCount int64
	if err := snapshot.Table("settings").Where("key = ?", "virustotal_key").Count(&secretCount).Error; err != nil {
		t.Fatalf("count secret: %v", err)
	}
	if err := snapshot.Table("settings").Where("key = ? AND value = ?", "log_level", "WARN").Count(&publicCount).Error; err != nil {
		t.Fatalf("count public setting: %v", err)
	}
	if secretCount != 0 || publicCount != 1 {
		t.Fatalf("snapshot rows: secret=%d public=%d", secretCount, publicCount)
	}
	if len(log.errs) != 0 || len(log.infos) != 1 {
		t.Fatalf("unexpected logs: errors=%v infos=%v", log.errs, log.infos)
	}
}

func TestDownloadDb_SnapshotFailureReturns500AndLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openTestDB(t, filepath.Join(t.TempDir(), "source.sqlite"))
	closeTestDB(t, db)

	log := &testLogger{}
	h := &Handlers{
		DB:         db,
		DBPath:     "source.sqlite",
		Log:        log,
		SecretKeys: func() []string { return nil },
	}
	r := gin.New()
	r.GET("/download", h.DownloadDb)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/download", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if len(log.errs) != 1 || !strings.Contains(log.errs[0].Error(), "snapshot") {
		t.Fatalf("expected snapshot error log, got %v", log.errs)
	}
}

func TestDownloadDb_IncompleteWiringFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openTestDB(t, filepath.Join(t.TempDir(), "source.sqlite"))
	t.Cleanup(func() { closeTestDB(t, db) })
	if err := db.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)").Error; err != nil {
		t.Fatalf("create settings: %v", err)
	}
	const secret = "must-not-leak"
	if err := db.Exec("INSERT INTO settings(key, value) VALUES (?, ?)", "virustotal_key", secret).Error; err != nil {
		t.Fatalf("seed secret: %v", err)
	}

	tests := []struct {
		name string
		h    *Handlers
	}{
		{name: "nil handlers"},
		{
			name: "missing secret provider",
			h:    &Handlers{DB: db, DBPath: "source.sqlite", Log: &testLogger{}},
		},
		{
			name: "missing database",
			h: &Handlers{
				DBPath:     "source.sqlite",
				Log:        &testLogger{},
				SecretKeys: func() []string { return []string{"virustotal_key"} },
			},
		},
		{
			name: "missing logger",
			h: &Handlers{
				DB:         db,
				DBPath:     "source.sqlite",
				SecretKeys: func() []string { return []string{"virustotal_key"} },
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/download", tc.h.DownloadDb)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/download", nil))

			if w.Code != http.StatusInternalServerError {
				t.Fatalf("expected 500, got %d", w.Code)
			}
			if bytes.Contains(w.Body.Bytes(), []byte(secret)) {
				t.Fatal("incomplete wiring leaked secret bytes")
			}
			if strings.Contains(w.Header().Get("Content-Disposition"), "filter.sqlite") {
				t.Fatalf("incomplete wiring returned an attachment: %q", w.Header().Get("Content-Disposition"))
			}
		})
	}
}
