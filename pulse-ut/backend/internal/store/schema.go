package store

// schema is applied on every open; every statement is idempotent.
//
// Timestamp convention: every ts_ms column is Unix milliseconds UTC. Upstream
// Gadgetbridge mixes seconds (activity samples) and milliseconds (everything
// else); that split is a bug magnet and is not reproduced here.
const schema = `
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS device (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    address         TEXT    NOT NULL UNIQUE,
    name            TEXT    NOT NULL DEFAULT '',
    model           TEXT    NOT NULL DEFAULT '',
    alias           TEXT    NOT NULL DEFAULT '',
    firmware        TEXT    NOT NULL DEFAULT '',
    unit_id         INTEGER NOT NULL DEFAULT 0,
    product_number  INTEGER NOT NULL DEFAULT 0,
    created_ms      INTEGER NOT NULL DEFAULT 0,
    last_sync_ms    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS activity_sample (
    device_id       INTEGER NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    ts_ms           INTEGER NOT NULL,
    steps           INTEGER NOT NULL DEFAULT 0,
    heart_rate      INTEGER NOT NULL DEFAULT -1,
    raw_intensity   INTEGER NOT NULL DEFAULT -1,
    raw_kind        INTEGER NOT NULL DEFAULT -1,
    distance_cm     INTEGER NOT NULL DEFAULT 0,
    active_calories INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS idx_activity_ts ON activity_sample(ts_ms);

CREATE TABLE IF NOT EXISTS stress_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL, stress INTEGER NOT NULL,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS body_energy_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL, energy INTEGER NOT NULL,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS spo2_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL,
    spo2 INTEGER NOT NULL, type_num INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS sleep_stage_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL, stage INTEGER NOT NULL,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS sleep_event (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL,
    event INTEGER NOT NULL, event_type INTEGER NOT NULL DEFAULT 0, data INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (device_id, ts_ms, event)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS sleep_stats_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL, sleep_score INTEGER NOT NULL,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS nap_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL, end_ts_ms INTEGER NOT NULL,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS restless_moments_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL, count INTEGER NOT NULL,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS hrv_value_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL, value INTEGER NOT NULL,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS hrv_summary_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL,
    weekly_average INTEGER, last_night_average INTEGER, last_night_5min_high INTEGER,
    baseline_low_upper INTEGER, baseline_balanced_lower INTEGER, baseline_balanced_upper INTEGER,
    status_num INTEGER,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS respiratory_rate_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL, rate REAL NOT NULL,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS resting_hr_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL, heart_rate INTEGER NOT NULL,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS rmr_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL, value INTEGER NOT NULL,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS intensity_minutes_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL,
    moderate INTEGER NOT NULL DEFAULT 0, vigorous INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (device_id, ts_ms)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS metric_sample (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL, metric_type INTEGER NOT NULL,
    score REAL NOT NULL, extra INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (device_id, ts_ms, metric_type)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS battery_level (
    device_id INTEGER NOT NULL, ts_ms INTEGER NOT NULL, idx INTEGER NOT NULL DEFAULT 0,
    level INTEGER NOT NULL,
    PRIMARY KEY (device_id, ts_ms, idx)
) WITHOUT ROWID;

CREATE TABLE IF NOT EXISTS workout (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id     INTEGER NOT NULL,
    start_ms      INTEGER NOT NULL,
    end_ms        INTEGER NOT NULL,
    activity_kind INTEGER NOT NULL DEFAULT 0,
    sport         INTEGER NOT NULL DEFAULT 0,
    sub_sport     INTEGER NOT NULL DEFAULT 0,
    name          TEXT    NOT NULL DEFAULT '',
    summary_json  TEXT    NOT NULL DEFAULT '{}',
    fit_file_id   INTEGER NOT NULL DEFAULT 0,
    UNIQUE (device_id, start_ms)
);

CREATE TABLE IF NOT EXISTS workout_track (
    workout_id INTEGER NOT NULL REFERENCES workout(id) ON DELETE CASCADE,
    ts_ms      INTEGER NOT NULL,
    lat        REAL,
    lon        REAL,
    altitude   REAL,
    heart_rate INTEGER,
    cadence    INTEGER,
    speed      REAL,
    power      INTEGER,
    distance   REAL,
    PRIMARY KEY (workout_id, ts_ms)
) WITHOUT ROWID;

-- Raw FIT files pulled from the watch. data is kept so a future importer
-- revision can re-derive samples without another sync.
CREATE TABLE IF NOT EXISTS fit_file (
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

CREATE TABLE IF NOT EXISTS setting (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`
