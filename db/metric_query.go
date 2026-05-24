package db

import (
	"errors"
	"fmt"
	"time"

	"github.com/alextorq/dns-filter/logger"
	"github.com/alextorq/dns-filter/metric"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"gorm.io/gorm"
)

// queryStartKey is the per-statement settings key under which the "before"
// callback stashes the operation start time for the "after" callback to read.
// It lives on the *gorm.DB statement instance (each query gets its own), so
// concurrent queries on the hot path never clobber each other's timestamp.
const queryStartKey = "metrics:query_start"

var (
	// dbQueryDuration is the wall-clock time each GORM operation spends between
	// the before- and after-callbacks — the SQLite round trip plus GORM's own
	// row marshalling. Buckets are tuned for sub-millisecond reads (the common
	// case on the filter hot path) with headroom up to 5s, so a genuinely slow
	// query still lands in a real bucket instead of collapsing into +Inf — that
	// upper tail is exactly the "the DB is lagging" signal we want to see.
	// operation is the GORM processor: create / query / update / delete / row / raw.
	dbQueryDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "db_query_duration_seconds",
		Help: "Duration in seconds of the GORM processor body per operation (the SQLite round trip + row marshalling, excluding the surrounding transaction commit for writes)",
		Buckets: []float64{
			0.00005, 0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005,
			0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5,
		},
	}, []string{"operation"})

	// dbQueryErrors counts operations that finished with a non-nil db.Error.
	// gorm.ErrRecordNotFound is deliberately excluded: an empty First() is
	// normal control flow on the DNS path (cache miss → DB lookup → not on the
	// block list), not a fault, and counting it would make the metric useless
	// for alerting.
	dbQueryErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "db_query_errors_total",
		Help: "DB operations that ended in an error, by GORM processor (record-not-found excluded)",
	}, []string{"operation"})
)

func init() {
	metric.Registry.MustRegister(dbQueryDuration, dbQueryErrors)
}

// recordQueryStart stamps the operation start time onto the statement. Paired
// with recordQueryEnd through queryStartKey.
func recordQueryStart(db *gorm.DB) {
	db.InstanceSet(queryStartKey, time.Now())
}

// recordQueryEnd observes the elapsed time and bumps the error counter for op.
// It is defensive about the start stamp: if it is missing or the wrong type
// (e.g. another plugin reset the statement), it skips the observation rather
// than panic on the DNS hot path.
func recordQueryEnd(op string) func(*gorm.DB) {
	return func(db *gorm.DB) {
		v, ok := db.InstanceGet(queryStartKey)
		if !ok {
			return
		}
		start, ok := v.(time.Time)
		if !ok {
			return
		}
		dbQueryDuration.WithLabelValues(op).Observe(time.Since(start).Seconds())
		if db.Error != nil && !errors.Is(db.Error, gorm.ErrRecordNotFound) {
			dbQueryErrors.WithLabelValues(op).Inc()
		}
	}
}

// instrumentQueries wires the duration/error callbacks onto every GORM
// processor. A registration error is only possible on a bad anchor name; we
// log it and carry on, because metrics are observability and must never be a
// reason to refuse to serve DNS.
//
// The callbacks anchor on the core processor step (e.g. "gorm:create"), so the
// measured span is the processor body — the SQLite round trip plus GORM's row
// marshalling. For writes that means the transaction commit/rollback wrapper
// (gorm:begin_transaction / gorm:commit_or_rollback_transaction) is NOT
// included; with synchronous=NORMAL + WAL the commit is cheap, so this stays a
// faithful "how slow is the query" signal, just not literal end-to-end time.
func instrumentQueries(conn *gorm.DB) {
	l := logger.GetLogger()
	cb := conn.Callback()
	logErr := func(name string, err error) {
		if err != nil {
			l.Error(fmt.Errorf("db metrics: register %s callback: %w", name, err))
		}
	}

	logErr("before_create", cb.Create().Before("gorm:create").Register("metrics:before_create", recordQueryStart))
	logErr("after_create", cb.Create().After("gorm:create").Register("metrics:after_create", recordQueryEnd("create")))
	logErr("before_query", cb.Query().Before("gorm:query").Register("metrics:before_query", recordQueryStart))
	logErr("after_query", cb.Query().After("gorm:query").Register("metrics:after_query", recordQueryEnd("query")))
	logErr("before_update", cb.Update().Before("gorm:update").Register("metrics:before_update", recordQueryStart))
	logErr("after_update", cb.Update().After("gorm:update").Register("metrics:after_update", recordQueryEnd("update")))
	logErr("before_delete", cb.Delete().Before("gorm:delete").Register("metrics:before_delete", recordQueryStart))
	logErr("after_delete", cb.Delete().After("gorm:delete").Register("metrics:after_delete", recordQueryEnd("delete")))
	logErr("before_row", cb.Row().Before("gorm:row").Register("metrics:before_row", recordQueryStart))
	logErr("after_row", cb.Row().After("gorm:row").Register("metrics:after_row", recordQueryEnd("row")))
	logErr("before_raw", cb.Raw().Before("gorm:raw").Register("metrics:before_raw", recordQueryStart))
	logErr("after_raw", cb.Raw().After("gorm:raw").Register("metrics:after_raw", recordQueryEnd("raw")))
}

// instrumentPool registers the database/sql pool-stats collector (open / idle /
// in-use connections, wait count, cumulative wait time). For the glebarez
// (modernc) SQLite driver the connection pool is the write-serialisation point,
// so a climbing go_sql_wait_duration is the clearest "the DB is the bottleneck"
// signal. db_name="main" labels every series so a future second pool can't
// collide. Register (not MustRegister) so a duplicate call only logs.
func instrumentPool(conn *gorm.DB) {
	l := logger.GetLogger()
	sqlDB, err := conn.DB()
	if err != nil {
		l.Error(fmt.Errorf("db metrics: cannot reach *sql.DB for pool stats: %w", err))
		return
	}
	if err := metric.Registry.Register(collectors.NewDBStatsCollector(sqlDB, "main")); err != nil {
		l.Error(fmt.Errorf("db metrics: register pool-stats collector: %w", err))
	}
}

// instrumentConnection attaches all DB-level metrics to conn: per-operation
// latency/error callbacks and the connection-pool stats collector. Called once
// from GetConnection after the pool is open and PRAGMAs are applied.
func instrumentConnection(conn *gorm.DB) {
	instrumentQueries(conn)
	instrumentPool(conn)
}
