package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrationSQLFreshSQLiteSequence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "fresh.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sqlite db handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	for _, name := range []string{
		"0001_init.sqlite.sql",
		"0002_runtime_settings.sqlite.sql",
		"0003_sync_skipped_rows.sqlite.sql",
	} {
		if err := execSQLFile(db, name); err != nil {
			t.Fatalf("exec %s: %v", name, err)
		}
	}

	var skippedCol int
	if err := db.Raw(`SELECT COUNT(*) FROM pragma_table_info('sync_runs') WHERE name = 'rows_skipped'`).Scan(&skippedCol).Error; err != nil {
		t.Fatalf("check rows_skipped: %v", err)
	}
	if skippedCol != 1 {
		t.Fatalf("rows_skipped columns = %d, want 1", skippedCol)
	}
	if err := db.Exec(`INSERT INTO sync_skipped_rows (run_id, source_type, line_number, town, raw_record_json) VALUES ('r1', 'full', 1, '除外町', '[]')`).Error; err != nil {
		t.Fatalf("insert skipped row: %v", err)
	}
}

func TestMigrationSQLMySQLSkippedSchemaOnlyIn0003(t *testing.T) {
	initSQL := readMigration(t, "0001_init.mysql.sql")
	if strings.Contains(initSQL, "rows_skipped") || strings.Contains(initSQL, "sync_skipped_rows") {
		t.Fatal("0001_init.mysql.sql must not contain skipped-row schema; 0003 owns that migration")
	}

	migrationSQL := readMigration(t, "0003_sync_skipped_rows.mysql.sql")
	if got := strings.Count(migrationSQL, "ADD COLUMN rows_skipped"); got != 1 {
		t.Fatalf("0003 mysql ADD COLUMN rows_skipped count=%d, want 1", got)
	}
	if got := strings.Count(migrationSQL, "CREATE TABLE IF NOT EXISTS sync_skipped_rows"); got != 1 {
		t.Fatalf("0003 mysql sync_skipped_rows create count=%d, want 1", got)
	}
}

func execSQLFile(db *gorm.DB, name string) error {
	sql := readMigrationNoTest(filepath.Join("..", "..", "migrations", name))
	for _, stmt := range strings.Split(sql, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func readMigration(t *testing.T, name string) string {
	t.Helper()
	return readMigrationNoTest(filepath.Join("..", "..", "migrations", name))
}

func readMigrationNoTest(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(b)
}
