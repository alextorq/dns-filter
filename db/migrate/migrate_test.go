package migrate

import (
	"testing"

	clientsdb "github.com/alextorq/dns-filter/clients/db"
	settingsdb "github.com/alextorq/dns-filter/settings/db"
	trafficdb "github.com/alextorq/dns-filter/traffic/db"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	conn, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	return conn
}

func TestMigrateCreatesSchemaOnProvidedConnection(t *testing.T) {
	conn := openTestDB(t)

	Migrate(conn)

	for name, model := range map[string]any{
		"clients":        &clientsdb.Client{},
		"settings":       &settingsdb.Setting{},
		"domain traffic": &trafficdb.DomainTraffic{},
	} {
		if !conn.Migrator().HasTable(model) {
			t.Errorf("provided connection is missing %s table", name)
		}
	}
}

func TestMigrateConvertsLegacyExcludedClients(t *testing.T) {
	conn := openTestDB(t)
	if err := conn.Exec(`CREATE TABLE exclude_clients (
		id INTEGER PRIMARY KEY,
		user_id TEXT NOT NULL,
		active NUMERIC NOT NULL,
		deleted_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if err := conn.Exec(`INSERT INTO exclude_clients (user_id, active, deleted_at) VALUES
		('10.0.0.1', 1, NULL),
		('10.0.0.2', 0, NULL),
		('10.0.0.3', 1, '2025-01-01 00:00:00')`).Error; err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}

	Migrate(conn)

	if conn.Migrator().HasTable("exclude_clients") {
		t.Fatal("legacy exclude_clients table must be dropped")
	}

	// A normal restart runs the same migration again after the legacy table is
	// gone. It must neither recreate legacy data nor duplicate client rows.
	Migrate(conn)

	var rows []clientsdb.Client
	if err := conn.Order("ip").Find(&rows).Error; err != nil {
		t.Fatalf("read migrated clients: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("migrated clients = %d, want 2 non-deleted rows", len(rows))
	}
	if rows[0].IP != "10.0.0.1" || rows[0].Filtered {
		t.Fatalf("active legacy exclusion migrated as %+v, want Filtered=false", rows[0])
	}
	if rows[1].IP != "10.0.0.2" || !rows[1].Filtered {
		t.Fatalf("inactive legacy exclusion migrated as %+v, want Filtered=true", rows[1])
	}
}

func TestMigratePanicsWhenProvidedConnectionIsClosed(t *testing.T) {
	conn := openTestDB(t)
	sqlDB, err := conn.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close test db: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("Migrate must surface an unusable connection by panicking")
		}
	}()
	Migrate(conn)
}
