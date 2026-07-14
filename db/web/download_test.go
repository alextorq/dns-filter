package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type testLogger struct {
	errs  []error
	infos []string
}

func (l *testLogger) Error(err error) { l.errs = append(l.errs, err) }
func (l *testLogger) Info(args ...any) {
	l.infos = append(l.infos, fmt.Sprint(args...))
}

type trackingReadCloser struct {
	*bytes.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

type fakeSnapshotExporter struct {
	body       []byte
	err        error
	nilContent bool
	calls      int
	ctx        context.Context
	reader     *trackingReadCloser
}

func (f *fakeSnapshotExporter) Export(ctx context.Context) (io.ReadCloser, int64, error) {
	f.calls++
	f.ctx = ctx
	if f.err != nil {
		return nil, 0, f.err
	}
	if f.nilContent {
		return nil, 0, nil
	}
	f.reader = &trackingReadCloser{Reader: bytes.NewReader(f.body)}
	return f.reader, int64(len(f.body)), nil
}

func TestDownloadDb_InvalidExporterResultFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := &testLogger{}
	h := &Handlers{Exporter: &fakeSnapshotExporter{nilContent: true}, Log: log}
	r := gin.New()
	r.GET("/download", h.DownloadDb)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/download", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if len(log.errs) != 1 || !strings.Contains(log.errs[0].Error(), "invalid snapshot") {
		t.Fatalf("expected invalid snapshot log, got %v", log.errs)
	}
	if strings.Contains(w.Header().Get("Content-Disposition"), "filter.sqlite") {
		t.Fatalf("invalid snapshot returned an attachment: %q", w.Header().Get("Content-Disposition"))
	}
}

func TestDownloadDb_StreamsExportedSnapshotAndClosesIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	exporter := &fakeSnapshotExporter{body: []byte("sqlite snapshot")}
	log := &testLogger{}
	h := &Handlers{Exporter: exporter, Log: log}
	r := gin.New()
	r.GET("/download", h.DownloadDb)
	req := httptest.NewRequest(http.MethodGet, "/download", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "sqlite snapshot" {
		t.Fatalf("body = %q, want exported snapshot", got)
	}
	if exporter.calls != 1 {
		t.Fatalf("Export calls = %d, want 1", exporter.calls)
	}
	if exporter.ctx != req.Context() {
		t.Fatal("request context was not forwarded to exporter")
	}
	if exporter.reader == nil || !exporter.reader.closed {
		t.Fatal("exported snapshot was not closed after streaming")
	}
	if got := w.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type = %q, want application/octet-stream", got)
	}
	if got := w.Header().Get("Content-Disposition"); !strings.Contains(got, "filter.sqlite") {
		t.Fatalf("Content-Disposition = %q, want attachment filename", got)
	}
	if len(log.errs) != 0 || len(log.infos) != 1 {
		t.Fatalf("unexpected logs: errors=%v infos=%v", log.errs, log.infos)
	}
}

func TestDownloadDb_ExporterFailureReturns500AndLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	exporter := &fakeSnapshotExporter{err: errors.New("sqlite unavailable")}
	log := &testLogger{}
	h := &Handlers{Exporter: exporter, Log: log}
	r := gin.New()
	r.GET("/download", h.DownloadDb)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/download", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if len(log.errs) != 1 || !strings.Contains(log.errs[0].Error(), "export snapshot") {
		t.Fatalf("expected exporter error log, got %v", log.errs)
	}
	if strings.Contains(w.Header().Get("Content-Disposition"), "filter.sqlite") {
		t.Fatalf("failed export returned an attachment: %q", w.Header().Get("Content-Disposition"))
	}
}

func TestDownloadDb_IncompleteWiringFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		h    *Handlers
	}{
		{name: "nil handlers"},
		{name: "missing exporter", h: &Handlers{Log: &testLogger{}}},
		{name: "missing logger", h: &Handlers{Exporter: &fakeSnapshotExporter{body: []byte("must-not-stream")}}},
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
			if strings.Contains(w.Header().Get("Content-Disposition"), "filter.sqlite") {
				t.Fatalf("incomplete wiring returned an attachment: %q", w.Header().Get("Content-Disposition"))
			}
		})
	}
}
