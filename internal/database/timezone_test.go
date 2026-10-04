package database

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestScheduleTimezoneMigration(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "migration.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT, updated_at DATETIME);
CREATE TABLE task_rules (id INTEGER PRIMARY KEY, trigger_type TEXT, next_run_at DATETIME);
INSERT INTO task_rules VALUES (1,'daily','2026-10-05 09:00:00+00:00'), (2,'cron','2026-10-05 09:00:00+00:00'), (3,'once','2026-10-05 09:00:00+08:00'), (4,'interval','2026-10-05 09:00:00-05:00');`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := sqliteMigrations.ReadFile("migrations/000016_schedule_timezone.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	var zone string
	if err := db.QueryRow("SELECT value FROM settings WHERE key='system.timezone'").Scan(&zone); err != nil {
		t.Fatal(err)
	}
	if zone != "Asia/Shanghai" {
		t.Fatal(zone)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM task_rules WHERE id IN (1,2) AND next_run_at IS NULL").Scan(&count); err != nil || count != 2 {
		t.Fatalf("calendar count=%d err=%v", count, err)
	}
	for id, want := range map[int]string{3: "2026-10-05 01:00:00.000", 4: "2026-10-05 14:00:00.000"} {
		var got string
		if err := db.QueryRow("SELECT CAST(next_run_at AS TEXT) FROM task_rules WHERE id=?", id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("id=%d next=%s want=%s", id, got, want)
		}
	}
}
