package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Workout is one recorded activity summary.
type Workout struct {
	ID           int64           `json:"id"`
	DeviceID     int64           `json:"-"`
	StartMs      int64           `json:"startMs"`
	EndMs        int64           `json:"endMs"`
	ActivityKind int             `json:"kind"`
	Sport        int             `json:"sport"`
	SubSport     int             `json:"subSport"`
	Name         string          `json:"name"`
	Summary      json.RawMessage `json:"summary"`
	FitFileID    int64           `json:"fitFileId"`
	Track        []TrackPoint    `json:"track,omitempty"`
}

// TrackPoint is one recorded sample inside a workout.
type TrackPoint struct {
	TsMs      int64    `json:"tsMs"`
	Lat       *float64 `json:"lat,omitempty"`
	Lon       *float64 `json:"lon,omitempty"`
	Altitude  *float64 `json:"altitude,omitempty"`
	HeartRate *int     `json:"heartRate,omitempty"`
	Cadence   *int     `json:"cadence,omitempty"`
	Speed     *float64 `json:"speed,omitempty"`
	Power     *int     `json:"power,omitempty"`
	Distance  *float64 `json:"distance,omitempty"`
}

// PutWorkout inserts or updates a workout keyed by (device, start) and stores
// its track.
func (db *DB) PutWorkout(w *Workout) error {
	if len(w.Summary) == 0 {
		w.Summary = json.RawMessage("{}")
	}
	return db.tx(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO workout (device_id, start_ms, end_ms, activity_kind, sport, sub_sport, name, summary_json, fit_file_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(device_id, start_ms) DO UPDATE SET
    end_ms = excluded.end_ms, activity_kind = excluded.activity_kind,
    sport = excluded.sport, sub_sport = excluded.sub_sport,
    name = excluded.name, summary_json = excluded.summary_json,
    fit_file_id = excluded.fit_file_id`,
			w.DeviceID, w.StartMs, w.EndMs, w.ActivityKind, w.Sport, w.SubSport, w.Name,
			string(w.Summary), w.FitFileID)
		if err != nil {
			return fmt.Errorf("store: put workout: %w", err)
		}
		if err := tx.QueryRow(`SELECT id FROM workout WHERE device_id = ? AND start_ms = ?`,
			w.DeviceID, w.StartMs).Scan(&w.ID); err != nil {
			return err
		}
		if len(w.Track) == 0 {
			return nil
		}
		if _, err := tx.Exec(`DELETE FROM workout_track WHERE workout_id = ?`, w.ID); err != nil {
			return err
		}
		st, err := tx.Prepare(`
INSERT INTO workout_track (workout_id, ts_ms, lat, lon, altitude, heart_rate, cadence, speed, power, distance)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, p := range w.Track {
			if _, err := st.Exec(w.ID, p.TsMs, p.Lat, p.Lon, p.Altitude, p.HeartRate,
				p.Cadence, p.Speed, p.Power, p.Distance); err != nil {
				return err
			}
		}
		return nil
	})
}

// Workouts lists the most recent workouts without their tracks.
func (db *DB) Workouts(limit int) ([]Workout, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.sql.Query(`
SELECT id, device_id, start_ms, end_ms, activity_kind, sport, sub_sport, name, summary_json, fit_file_id
FROM workout ORDER BY start_ms DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: workouts: %w", err)
	}
	defer rows.Close()
	var out []Workout
	for rows.Next() {
		w, err := scanWorkout(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// Workout returns one workout with its track.
func (db *DB) Workout(id int64) (*Workout, error) {
	row := db.sql.QueryRow(`
SELECT id, device_id, start_ms, end_ms, activity_kind, sport, sub_sport, name, summary_json, fit_file_id
FROM workout WHERE id = ?`, id)
	w, err := scanWorkout(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := db.sql.Query(`
SELECT ts_ms, lat, lon, altitude, heart_rate, cadence, speed, power, distance
FROM workout_track WHERE workout_id = ? ORDER BY ts_ms`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p TrackPoint
		if err := rows.Scan(&p.TsMs, &p.Lat, &p.Lon, &p.Altitude, &p.HeartRate,
			&p.Cadence, &p.Speed, &p.Power, &p.Distance); err != nil {
			return nil, err
		}
		w.Track = append(w.Track, p)
	}
	return &w, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanWorkout(s scanner) (Workout, error) {
	var w Workout
	var summary string
	err := s.Scan(&w.ID, &w.DeviceID, &w.StartMs, &w.EndMs, &w.ActivityKind, &w.Sport,
		&w.SubSport, &w.Name, &summary, &w.FitFileID)
	if err != nil {
		return w, err
	}
	w.Summary = json.RawMessage(summary)
	return w, nil
}

// FitFile is a raw file pulled from the watch.
type FitFile struct {
	ID           int64
	DeviceID     int64
	FileNumber   int
	DataType     int
	SubType      int
	FileTs       int64
	Flags        int
	Size         int
	DownloadedMs int64
	Imported     bool
	Data         []byte
}

// PutFitFile stores a downloaded file, replacing an earlier copy with the same
// file number.
func (db *DB) PutFitFile(f *FitFile) error {
	db.write.Lock()
	defer db.write.Unlock()
	_, err := db.sql.Exec(`
INSERT INTO fit_file (device_id, file_number, data_type, sub_type, file_ts, flags, size, downloaded_ms, imported, data)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(device_id, file_number) DO UPDATE SET
    data_type = excluded.data_type, sub_type = excluded.sub_type, file_ts = excluded.file_ts,
    flags = excluded.flags, size = excluded.size, downloaded_ms = excluded.downloaded_ms,
    imported = excluded.imported, data = excluded.data`,
		f.DeviceID, f.FileNumber, f.DataType, f.SubType, f.FileTs, f.Flags, f.Size,
		f.DownloadedMs, boolInt(f.Imported), f.Data)
	if err != nil {
		return fmt.Errorf("store: put fit file: %w", err)
	}
	return db.sql.QueryRow(`SELECT id FROM fit_file WHERE device_id = ? AND file_number = ?`,
		f.DeviceID, f.FileNumber).Scan(&f.ID)
}

// HasFitFile reports whether a file number was already downloaded.
func (db *DB) HasFitFile(deviceID int64, fileNumber int) bool {
	var n int
	err := db.sql.QueryRow(`SELECT COUNT(1) FROM fit_file WHERE device_id = ? AND file_number = ?`,
		deviceID, fileNumber).Scan(&n)
	return err == nil && n > 0
}

// FitFiles returns every stored file for a device, oldest first, with the raw
// blob attached so the importer can replay them.
func (db *DB) FitFiles(deviceID int64) ([]FitFile, error) {
	rows, err := db.sql.Query(`
SELECT id, device_id, file_number, data_type, sub_type, file_ts, flags, size, downloaded_ms, imported, data
FROM fit_file WHERE device_id = ? AND data IS NOT NULL ORDER BY file_ts, file_number`, deviceID)
	if err != nil {
		return nil, fmt.Errorf("store: fit files: %w", err)
	}
	defer rows.Close()
	var out []FitFile
	for rows.Next() {
		var f FitFile
		var imported int
		if err := rows.Scan(&f.ID, &f.DeviceID, &f.FileNumber, &f.DataType, &f.SubType,
			&f.FileTs, &f.Flags, &f.Size, &f.DownloadedMs, &imported, &f.Data); err != nil {
			return nil, err
		}
		f.Imported = imported != 0
		out = append(out, f)
	}
	return out, rows.Err()
}

// ResetDerived drops every table the importer rebuilds from FIT files. The
// files themselves and the device row survive, so a reimport restores the
// dashboard without touching the watch.
func (db *DB) ResetDerived(deviceID int64) error {
	return db.tx(func(tx *sql.Tx) error {
		// Track rows hang off workout, which has no device column of its own.
		if _, err := tx.Exec(`
DELETE FROM workout_track WHERE workout_id IN (SELECT id FROM workout WHERE device_id = ?)`,
			deviceID); err != nil {
			return fmt.Errorf("store: reset workout_track: %w", err)
		}
		for _, t := range []string{
			"activity_sample", "stress_sample", "body_energy_sample", "spo2_sample",
			"sleep_stage_sample", "sleep_stats_sample", "sleep_event", "nap_sample", "sleep_session",
			"restless_moments_sample", "hrv_value_sample", "hrv_summary_sample",
			"respiratory_rate_sample", "resting_hr_sample", "rmr_sample",
			"intensity_minutes_sample", "metric_sample", "workout",
		} {
			if _, err := tx.Exec(`DELETE FROM `+t+` WHERE device_id = ?`, deviceID); err != nil {
				return fmt.Errorf("store: reset %s: %w", t, err)
			}
		}
		return nil
	})
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
