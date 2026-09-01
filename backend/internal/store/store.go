// Package store is the on-device SQLite database: devices, health samples,
// workouts, raw FIT files and user settings.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite handle. SQLite serialises writers itself, but a mutex
// around write batches keeps transactions from interleaving under WAL.
type DB struct {
	sql   *sql.DB
	path  string
	write sync.Mutex
}

// Open creates or migrates the database at path.
func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("store: create %s: %w", dir, err)
		}
	}
	h, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	// modernc's driver is not safe to hammer from many connections at once for
	// writes; one connection keeps ordering deterministic and is plenty fast
	// for a phone-sized dataset.
	h.SetMaxOpenConns(1)
	if _, err := h.Exec(schema); err != nil {
		h.Close()
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return &DB{sql: h, path: path}, nil
}

func (db *DB) Close() error { return db.sql.Close() }

// Path reports the database file location.
func (db *DB) Path() string { return db.path }

// ---------------------------------------------------------------- devices ---

// Device is a paired watch.
type Device struct {
	ID            int64  `json:"-"`
	Address       string `json:"address"`
	Name          string `json:"name"`
	Model         string `json:"model"`
	Alias         string `json:"alias"`
	Firmware      string `json:"firmware"`
	UnitID        int64  `json:"unitId"`
	ProductNumber int64  `json:"productNumber"`
	CreatedMs     int64  `json:"createdMs"`
	LastSyncMs    int64  `json:"lastSyncMs"`
}

// UpsertDevice inserts or updates a device by address and fills in its ID.
func (db *DB) UpsertDevice(d *Device) error {
	db.write.Lock()
	defer db.write.Unlock()
	_, err := db.sql.Exec(`
INSERT INTO device (address, name, model, alias, firmware, unit_id, product_number, created_ms, last_sync_ms)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(address) DO UPDATE SET
    name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE device.name END,
    model = CASE WHEN excluded.model <> '' THEN excluded.model ELSE device.model END,
    firmware = CASE WHEN excluded.firmware <> '' THEN excluded.firmware ELSE device.firmware END,
    unit_id = CASE WHEN excluded.unit_id <> 0 THEN excluded.unit_id ELSE device.unit_id END,
    product_number = CASE WHEN excluded.product_number <> 0 THEN excluded.product_number ELSE device.product_number END`,
		d.Address, d.Name, d.Model, d.Alias, d.Firmware, d.UnitID, d.ProductNumber, d.CreatedMs, d.LastSyncMs)
	if err != nil {
		return fmt.Errorf("store: upsert device: %w", err)
	}
	return db.sql.QueryRow(`SELECT id FROM device WHERE address = ?`, d.Address).Scan(&d.ID)
}

// DeviceByAddress looks a device up.
func (db *DB) DeviceByAddress(addr string) (*Device, error) {
	var d Device
	err := db.sql.QueryRow(`
SELECT id, address, name, model, alias, firmware, unit_id, product_number, created_ms, last_sync_ms
FROM device WHERE address = ?`, addr).Scan(&d.ID, &d.Address, &d.Name, &d.Model, &d.Alias,
		&d.Firmware, &d.UnitID, &d.ProductNumber, &d.CreatedMs, &d.LastSyncMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: device %s: %w", addr, err)
	}
	return &d, nil
}

// Devices lists all known devices, most recently synced first.
func (db *DB) Devices() ([]Device, error) {
	rows, err := db.sql.Query(`
SELECT id, address, name, model, alias, firmware, unit_id, product_number, created_ms, last_sync_ms
FROM device ORDER BY last_sync_ms DESC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("store: devices: %w", err)
	}
	defer rows.Close()
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.Address, &d.Name, &d.Model, &d.Alias, &d.Firmware,
			&d.UnitID, &d.ProductNumber, &d.CreatedMs, &d.LastSyncMs); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// TouchSync records the end of a successful sync.
func (db *DB) TouchSync(deviceID, tsMs int64) error {
	db.write.Lock()
	defer db.write.Unlock()
	_, err := db.sql.Exec(`UPDATE device SET last_sync_ms = ? WHERE id = ?`, tsMs, deviceID)
	return err
}

// ForgetDevice removes a device and every sample attached to it.
func (db *DB) ForgetDevice(addr string) error {
	db.write.Lock()
	defer db.write.Unlock()
	dev, err := db.DeviceByAddress(addr)
	if err != nil || dev == nil {
		return err
	}
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range []string{
		"activity_sample", "stress_sample", "body_energy_sample", "spo2_sample",
		"sleep_stage_sample", "sleep_event", "sleep_stats_sample", "nap_sample",
		"restless_moments_sample", "hrv_value_sample", "hrv_summary_sample",
		"respiratory_rate_sample", "resting_hr_sample", "rmr_sample",
		"intensity_minutes_sample", "metric_sample", "battery_level", "workout", "fit_file",
	} {
		if _, err := tx.Exec(`DELETE FROM `+t+` WHERE device_id = ?`, dev.ID); err != nil {
			return fmt.Errorf("store: purge %s: %w", t, err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM device WHERE id = ?`, dev.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// --------------------------------------------------------------- settings ---

// Setting reads a key, returning def when absent.
func (db *DB) Setting(key, def string) string {
	var v string
	if err := db.sql.QueryRow(`SELECT value FROM setting WHERE key = ?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

// SetSetting writes a key.
func (db *DB) SetSetting(key, value string) error {
	db.write.Lock()
	defer db.write.Unlock()
	_, err := db.sql.Exec(`
INSERT INTO setting (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// AllSettings returns every stored key.
func (db *DB) AllSettings() (map[string]string, error) {
	rows, err := db.sql.Query(`SELECT key, value FROM setting`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}
