package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestMigrateWidensFitFileKey opens a database created by the previous schema
// revision: fit_file keyed on the file number alone and sleep_session without
// stage minutes. Both must be migrated in place, keeping the stored blobs.
func TestMigrateWidensFitFileKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	h, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	_, err = h.Exec(`
CREATE TABLE fit_file (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id      INTEGER NOT NULL,
    file_number    INTEGER NOT NULL,
    data_type      INTEGER NOT NULL,
    sub_type       INTEGER NOT NULL,
    file_ts        INTEGER NOT NULL,
    flags          INTEGER NOT NULL DEFAULT 0,
    size           INTEGER NOT NULL DEFAULT 0,
    downloaded_ms  INTEGER NOT NULL,
    imported       INTEGER NOT NULL DEFAULT 0,
    data           BLOB,
    UNIQUE (device_id, file_number)
);
CREATE TABLE sleep_session (
    device_id INTEGER NOT NULL, start_ms INTEGER NOT NULL, end_ms INTEGER NOT NULL,
    awake_ms INTEGER NOT NULL DEFAULT 0, score INTEGER NOT NULL DEFAULT 0,
    start_body_battery INTEGER NOT NULL DEFAULT 0, end_body_battery INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (device_id, start_ms)
) WITHOUT ROWID;
INSERT INTO fit_file (device_id, file_number, data_type, sub_type, file_ts, downloaded_ms, data)
VALUES (1, 185, 128, 32, 100, 100, x'0102');
INSERT INTO sleep_session (device_id, start_ms, end_ms, score) VALUES (1, 10, 20, 74);`)
	if err != nil {
		t.Fatalf("seed old schema: %v", err)
	}
	if err := h.Close(); err != nil {
		t.Fatalf("close raw: %v", err)
	}

	db, err := Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	dev := &Device{Address: "AA:BB:CC:DD:EE:FF"}
	if err := db.UpsertDevice(dev); err != nil {
		t.Fatalf("upsert device: %v", err)
	}

	files, err := db.FitFiles(dev.ID)
	if err != nil {
		t.Fatalf("fit files: %v", err)
	}
	if len(files) != 1 || files[0].FileNumber != 185 || len(files[0].Data) != 2 {
		t.Fatalf("stored file lost in migration: %+v", files)
	}

	// A sleep file may reuse a monitor file's number; both must coexist now.
	sleep := &FitFile{DeviceID: dev.ID, FileNumber: 185, FileIndex: 7,
		DataType: 128, SubType: 49, FileTs: 200, DownloadedMs: 200, Data: []byte{3, 4}}
	if err := db.PutFitFile(sleep); err != nil {
		t.Fatalf("put sleep file: %v", err)
	}
	if !db.HasFitFile(dev.ID, 128, 49, 185) {
		t.Error("sleep file not found by its own type")
	}
	if db.HasFitFile(dev.ID, 128, 44, 185) {
		t.Error("a metrics file of the same number must not look downloaded")
	}
	files, err = db.FitFiles(dev.ID)
	if err != nil {
		t.Fatalf("fit files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("stored files = %d, want 2", len(files))
	}

	sessions := []SleepSession{{StartMs: 10, EndMs: 20, DeepMs: 90 * 60000}}
	if err := db.PutSleepSessions(dev.ID, sessions); err != nil {
		t.Fatalf("put sleep session: %v", err)
	}
	got, err := db.SleepSessions(dev.ID, 0, 100)
	if err != nil {
		t.Fatalf("sleep sessions: %v", err)
	}
	if len(got) != 1 || got[0].DeepMs != 90*60000 {
		t.Fatalf("stage minutes not stored: %+v", got)
	}

	// Reopening must not rebuild anything a second time.
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	again, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer again.Close()
	files, err = again.FitFiles(dev.ID)
	if err != nil {
		t.Fatalf("fit files after reopen: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files after reopen = %d, want 2", len(files))
	}
}
