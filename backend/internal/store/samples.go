package store

import (
	"database/sql"
	"fmt"
)

// ActivitySample is one minute of movement data.
type ActivitySample struct {
	TsMs           int64 `json:"tsMs"`
	Steps          int   `json:"steps"`
	HeartRate      int   `json:"heartRate"`
	RawIntensity   int   `json:"rawIntensity"`
	RawKind        int   `json:"rawKind"`
	DistanceCm     int   `json:"distanceCm"`
	ActiveCalories int   `json:"activeCalories"`
}

// PutActivitySamples upserts a batch, summing nothing: the importer already
// resolved per-minute values.
func (db *DB) PutActivitySamples(deviceID int64, samples []ActivitySample) error {
	if len(samples) == 0 {
		return nil
	}
	return db.tx(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
INSERT INTO activity_sample (device_id, ts_ms, steps, heart_rate, raw_intensity, raw_kind, distance_cm, active_calories)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(device_id, ts_ms) DO UPDATE SET
    steps = MAX(activity_sample.steps, excluded.steps),
    heart_rate = CASE WHEN excluded.heart_rate > 0 THEN excluded.heart_rate ELSE activity_sample.heart_rate END,
    raw_intensity = MAX(activity_sample.raw_intensity, excluded.raw_intensity),
    raw_kind = CASE WHEN excluded.raw_kind >= 0 THEN excluded.raw_kind ELSE activity_sample.raw_kind END,
    distance_cm = MAX(activity_sample.distance_cm, excluded.distance_cm),
    active_calories = MAX(activity_sample.active_calories, excluded.active_calories)`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, s := range samples {
			if _, err := st.Exec(deviceID, s.TsMs, s.Steps, s.HeartRate, s.RawIntensity,
				s.RawKind, s.DistanceCm, s.ActiveCalories); err != nil {
				return err
			}
		}
		return nil
	})
}

// ActivitySamples returns samples in [fromMs, toMs).
func (db *DB) ActivitySamples(deviceID, fromMs, toMs int64) ([]ActivitySample, error) {
	rows, err := db.sql.Query(`
SELECT ts_ms, steps, heart_rate, raw_intensity, raw_kind, distance_cm, active_calories
FROM activity_sample WHERE device_id = ? AND ts_ms >= ? AND ts_ms < ? ORDER BY ts_ms`,
		deviceID, fromMs, toMs)
	if err != nil {
		return nil, fmt.Errorf("store: activity samples: %w", err)
	}
	defer rows.Close()
	var out []ActivitySample
	for rows.Next() {
		var s ActivitySample
		if err := rows.Scan(&s.TsMs, &s.Steps, &s.HeartRate, &s.RawIntensity, &s.RawKind,
			&s.DistanceCm, &s.ActiveCalories); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Point is one value of a simple time series.
type Point struct {
	TsMs  int64   `json:"tsMs"`
	Value float64 `json:"value"`
}

// Series names the simple one-value tables the API can query generically.
var seriesTables = map[string]struct{ table, column string }{
	"stress":      {"stress_sample", "stress"},
	"body_energy": {"body_energy_sample", "energy"},
	"spo2":        {"spo2_sample", "spo2"},
	"hrv":         {"hrv_value_sample", "value"},
	"respiration": {"respiratory_rate_sample", "rate"},
	"resting_hr":  {"resting_hr_sample", "heart_rate"},
	"rmr":         {"rmr_sample", "value"},
	"sleep_score": {"sleep_stats_sample", "sleep_score"},
	"restless":    {"restless_moments_sample", "count"},
}

// PutSeries upserts points into one of the simple series tables.
func (db *DB) PutSeries(name string, deviceID int64, points []Point) error {
	spec, ok := seriesTables[name]
	if !ok {
		return fmt.Errorf("store: unknown series %q", name)
	}
	if len(points) == 0 {
		return nil
	}
	q := fmt.Sprintf(`
INSERT INTO %s (device_id, ts_ms, %s) VALUES (?, ?, ?)
ON CONFLICT(device_id, ts_ms) DO UPDATE SET %s = excluded.%s`,
		spec.table, spec.column, spec.column, spec.column)
	return db.tx(func(tx *sql.Tx) error {
		st, err := tx.Prepare(q)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, p := range points {
			if _, err := st.Exec(deviceID, p.TsMs, p.Value); err != nil {
				return err
			}
		}
		return nil
	})
}

// Series reads points in [fromMs, toMs).
func (db *DB) Series(name string, deviceID, fromMs, toMs int64) ([]Point, error) {
	spec, ok := seriesTables[name]
	if !ok {
		return nil, fmt.Errorf("store: unknown series %q", name)
	}
	q := fmt.Sprintf(`SELECT ts_ms, %s FROM %s WHERE device_id = ? AND ts_ms >= ? AND ts_ms < ? ORDER BY ts_ms`,
		spec.column, spec.table)
	rows, err := db.sql.Query(q, deviceID, fromMs, toMs)
	if err != nil {
		return nil, fmt.Errorf("store: series %s: %w", name, err)
	}
	defer rows.Close()
	var out []Point
	for rows.Next() {
		var p Point
		if err := rows.Scan(&p.TsMs, &p.Value); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// LatestSeries returns the newest point at or before tsMs.
func (db *DB) LatestSeries(name string, deviceID, tsMs int64) (Point, bool) {
	spec, ok := seriesTables[name]
	if !ok {
		return Point{}, false
	}
	q := fmt.Sprintf(`SELECT ts_ms, %s FROM %s WHERE device_id = ? AND ts_ms <= ? ORDER BY ts_ms DESC LIMIT 1`,
		spec.column, spec.table)
	var p Point
	if err := db.sql.QueryRow(q, deviceID, tsMs).Scan(&p.TsMs, &p.Value); err != nil {
		return Point{}, false
	}
	return p, true
}

// SleepStage is one scored sleep interval sample.
type SleepStage struct {
	TsMs  int64 `json:"tsMs"`
	Stage int   `json:"stage"`
}

// PutSleepStages upserts sleep stage samples.
func (db *DB) PutSleepStages(deviceID int64, stages []SleepStage) error {
	if len(stages) == 0 {
		return nil
	}
	return db.tx(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
INSERT INTO sleep_stage_sample (device_id, ts_ms, stage) VALUES (?, ?, ?)
ON CONFLICT(device_id, ts_ms) DO UPDATE SET stage = excluded.stage`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, s := range stages {
			if _, err := st.Exec(deviceID, s.TsMs, s.Stage); err != nil {
				return err
			}
		}
		return nil
	})
}

// SleepStages reads stage samples in a window.
func (db *DB) SleepStages(deviceID, fromMs, toMs int64) ([]SleepStage, error) {
	rows, err := db.sql.Query(`
SELECT ts_ms, stage FROM sleep_stage_sample
WHERE device_id = ? AND ts_ms >= ? AND ts_ms < ? ORDER BY ts_ms`, deviceID, fromMs, toMs)
	if err != nil {
		return nil, fmt.Errorf("store: sleep stages: %w", err)
	}
	defer rows.Close()
	var out []SleepStage
	for rows.Next() {
		var s SleepStage
		if err := rows.Scan(&s.TsMs, &s.Stage); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SleepEvent marks falling asleep (eventType 0) and waking (1).
type SleepEvent struct {
	TsMs      int64 `json:"tsMs"`
	Event     int   `json:"event"`
	EventType int   `json:"eventType"`
	Data      int   `json:"data"`
}

func (db *DB) PutSleepEvents(deviceID int64, events []SleepEvent) error {
	if len(events) == 0 {
		return nil
	}
	return db.tx(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
INSERT INTO sleep_event (device_id, ts_ms, event, event_type, data) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(device_id, ts_ms, event) DO UPDATE SET event_type = excluded.event_type, data = excluded.data`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, e := range events {
			if _, err := st.Exec(deviceID, e.TsMs, e.Event, e.EventType, e.Data); err != nil {
				return err
			}
		}
		return nil
	})
}

// Nap is a detected daytime sleep session.
type Nap struct {
	StartMs int64 `json:"startMs"`
	EndMs   int64 `json:"endMs"`
}

func (db *DB) PutNaps(deviceID int64, naps []Nap) error {
	if len(naps) == 0 {
		return nil
	}
	return db.tx(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
INSERT INTO nap_sample (device_id, ts_ms, end_ts_ms) VALUES (?, ?, ?)
ON CONFLICT(device_id, ts_ms) DO UPDATE SET end_ts_ms = excluded.end_ts_ms`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, n := range naps {
			if _, err := st.Exec(deviceID, n.StartMs, n.EndMs); err != nil {
				return err
			}
		}
		return nil
	})
}

func (db *DB) Naps(deviceID, fromMs, toMs int64) ([]Nap, error) {
	rows, err := db.sql.Query(`
SELECT ts_ms, end_ts_ms FROM nap_sample WHERE device_id = ? AND ts_ms >= ? AND ts_ms < ? ORDER BY ts_ms`,
		deviceID, fromMs, toMs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Nap
	for rows.Next() {
		var n Nap
		if err := rows.Scan(&n.StartMs, &n.EndMs); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// IntensityMinutes is the weekly moderate/vigorous accumulator.
type IntensityMinutes struct {
	TsMs     int64 `json:"tsMs"`
	Moderate int   `json:"moderate"`
	Vigorous int   `json:"vigorous"`
}

func (db *DB) PutIntensityMinutes(deviceID int64, items []IntensityMinutes) error {
	if len(items) == 0 {
		return nil
	}
	return db.tx(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
INSERT INTO intensity_minutes_sample (device_id, ts_ms, moderate, vigorous) VALUES (?, ?, ?, ?)
ON CONFLICT(device_id, ts_ms) DO UPDATE SET moderate = excluded.moderate, vigorous = excluded.vigorous`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, i := range items {
			if _, err := st.Exec(deviceID, i.TsMs, i.Moderate, i.Vigorous); err != nil {
				return err
			}
		}
		return nil
	})
}

func (db *DB) IntensityMinutes(deviceID, fromMs, toMs int64) ([]IntensityMinutes, error) {
	rows, err := db.sql.Query(`
SELECT ts_ms, moderate, vigorous FROM intensity_minutes_sample
WHERE device_id = ? AND ts_ms >= ? AND ts_ms < ? ORDER BY ts_ms`, deviceID, fromMs, toMs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IntensityMinutes
	for rows.Next() {
		var i IntensityMinutes
		if err := rows.Scan(&i.TsMs, &i.Moderate, &i.Vigorous); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// HRVSummary is the nightly heart rate variability summary.
type HRVSummary struct {
	TsMs                  int64 `json:"tsMs"`
	WeeklyAverage         int   `json:"weeklyAverage"`
	LastNightAverage      int   `json:"lastNightAverage"`
	LastNight5MinHigh     int   `json:"lastNight5MinHigh"`
	BaselineLowUpper      int   `json:"baselineLowUpper"`
	BaselineBalancedLower int   `json:"baselineBalancedLower"`
	BaselineBalancedUpper int   `json:"baselineBalancedUpper"`
	StatusNum             int   `json:"statusNum"`
}

func (db *DB) PutHRVSummaries(deviceID int64, items []HRVSummary) error {
	if len(items) == 0 {
		return nil
	}
	return db.tx(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
INSERT INTO hrv_summary_sample (device_id, ts_ms, weekly_average, last_night_average, last_night_5min_high,
    baseline_low_upper, baseline_balanced_lower, baseline_balanced_upper, status_num)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(device_id, ts_ms) DO UPDATE SET
    weekly_average = excluded.weekly_average,
    last_night_average = excluded.last_night_average,
    last_night_5min_high = excluded.last_night_5min_high,
    baseline_low_upper = excluded.baseline_low_upper,
    baseline_balanced_lower = excluded.baseline_balanced_lower,
    baseline_balanced_upper = excluded.baseline_balanced_upper,
    status_num = excluded.status_num`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, i := range items {
			if _, err := st.Exec(deviceID, i.TsMs, i.WeeklyAverage, i.LastNightAverage,
				i.LastNight5MinHigh, i.BaselineLowUpper, i.BaselineBalancedLower,
				i.BaselineBalancedUpper, i.StatusNum); err != nil {
				return err
			}
		}
		return nil
	})
}

// MetricSample is a generic scored metric (VO2max, training readiness, ...).
type MetricSample struct {
	TsMs  int64   `json:"tsMs"`
	Type  int     `json:"type"`
	Score float64 `json:"score"`
	Extra int64   `json:"extra"`
}

func (db *DB) PutMetrics(deviceID int64, items []MetricSample) error {
	if len(items) == 0 {
		return nil
	}
	return db.tx(func(tx *sql.Tx) error {
		st, err := tx.Prepare(`
INSERT INTO metric_sample (device_id, ts_ms, metric_type, score, extra) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(device_id, ts_ms, metric_type) DO UPDATE SET score = excluded.score, extra = excluded.extra`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, i := range items {
			if _, err := st.Exec(deviceID, i.TsMs, i.Type, i.Score, i.Extra); err != nil {
				return err
			}
		}
		return nil
	})
}

func (db *DB) LatestMetric(deviceID int64, metricType int) (MetricSample, bool) {
	var m MetricSample
	err := db.sql.QueryRow(`
SELECT ts_ms, metric_type, score, extra FROM metric_sample
WHERE device_id = ? AND metric_type = ? ORDER BY ts_ms DESC LIMIT 1`, deviceID, metricType).
		Scan(&m.TsMs, &m.Type, &m.Score, &m.Extra)
	if err != nil {
		return MetricSample{}, false
	}
	return m, true
}

// PutBattery records a battery reading.
func (db *DB) PutBattery(deviceID, tsMs int64, level int) error {
	db.write.Lock()
	defer db.write.Unlock()
	_, err := db.sql.Exec(`
INSERT INTO battery_level (device_id, ts_ms, idx, level) VALUES (?, ?, 0, ?)
ON CONFLICT(device_id, ts_ms, idx) DO UPDATE SET level = excluded.level`, deviceID, tsMs, level)
	return err
}

// LatestBattery returns the newest battery percentage.
func (db *DB) LatestBattery(deviceID int64) (int, bool) {
	var level int
	err := db.sql.QueryRow(`SELECT level FROM battery_level WHERE device_id = ? ORDER BY ts_ms DESC LIMIT 1`,
		deviceID).Scan(&level)
	return level, err == nil
}

func (db *DB) tx(fn func(*sql.Tx) error) error {
	db.write.Lock()
	defer db.write.Unlock()
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
