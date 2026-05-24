package db

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"gorm.io/gorm"
)

// histSampleCount reports how many observations the duration histogram has
// recorded for op so far. The metric is process-global, so tests assert on the
// delta around an operation rather than an absolute value.
func histSampleCount(t *testing.T, op string) uint64 {
	t.Helper()
	obs, err := dbQueryDuration.GetMetricWithLabelValues(op)
	if err != nil {
		t.Fatalf("get histogram for %q: %v", op, err)
	}
	m := &dto.Metric{}
	if err := obs.(prometheus.Metric).Write(m); err != nil {
		t.Fatalf("write histogram for %q: %v", op, err)
	}
	return m.GetHistogram().GetSampleCount()
}

func instrumentedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	conn := newTestDB(t)
	instrumentQueries(conn)
	return conn
}

// Happy path: a successful create and query each bump the duration histogram
// for their GORM processor and leave the error counter untouched.
func TestInstrumentQueries_RecordsDurationPerOperation(t *testing.T) {
	conn := instrumentedTestDB(t)

	createBefore := histSampleCount(t, "create")
	queryBefore := histSampleCount(t, "query")
	errBefore := testutil.ToFloat64(dbQueryErrors.WithLabelValues("create")) +
		testutil.ToFloat64(dbQueryErrors.WithLabelValues("query"))

	if err := conn.Create(&testItem{Name: "alpha", Age: 1}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got testItem
	if err := conn.Where("name = ?", "alpha").First(&got).Error; err != nil {
		t.Fatalf("query: %v", err)
	}

	if after := histSampleCount(t, "create"); after <= createBefore {
		t.Errorf("create histogram not incremented: before=%d after=%d", createBefore, after)
	}
	if after := histSampleCount(t, "query"); after <= queryBefore {
		t.Errorf("query histogram not incremented: before=%d after=%d", queryBefore, after)
	}
	errAfter := testutil.ToFloat64(dbQueryErrors.WithLabelValues("create")) +
		testutil.ToFloat64(dbQueryErrors.WithLabelValues("query"))
	if errAfter != errBefore {
		t.Errorf("error counter moved on the happy path: before=%v after=%v", errBefore, errAfter)
	}
}

// Negative path: a real SQL failure bumps the error counter for its processor.
func TestInstrumentQueries_CountsQueryErrors(t *testing.T) {
	conn := instrumentedTestDB(t)

	before := testutil.ToFloat64(dbQueryErrors.WithLabelValues("raw"))
	// Exec runs through the gorm:raw processor; an unknown table is a hard error.
	if err := conn.Exec("SELECT * FROM table_that_does_not_exist").Error; err == nil {
		t.Fatal("expected an error querying a missing table, got nil")
	}
	if after := testutil.ToFloat64(dbQueryErrors.WithLabelValues("raw")); after != before+1 {
		t.Errorf("raw error counter: before=%v after=%v, want +1", before, after)
	}
}

// Edge case: ErrRecordNotFound is normal control flow (the DNS path looks a
// domain up and expects misses), not a fault, so an empty First() must NOT bump
// the error counter even though db.Error is non-nil.
func TestInstrumentQueries_RecordNotFoundIsNotAnError(t *testing.T) {
	conn := instrumentedTestDB(t)

	before := testutil.ToFloat64(dbQueryErrors.WithLabelValues("query"))
	var got testItem
	err := conn.Where("name = ?", "missing").First(&got).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
	if after := testutil.ToFloat64(dbQueryErrors.WithLabelValues("query")); after != before {
		t.Errorf("record-not-found bumped the error counter: before=%v after=%v", before, after)
	}
}
