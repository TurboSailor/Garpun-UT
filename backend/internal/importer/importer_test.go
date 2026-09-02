package importer

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pulse/backend/internal/analytics"
	"pulse/backend/internal/store"
)

// Files captured from a real Forerunner 255 over the Go GFDI implementation.
const testdataDir = "../../../testdata/fitdump"

func testDB(t *testing.T) (*store.DB, int64) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "pulse.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	dev := &store.Device{Address: "E9:29:A3:99:E4:4F", Name: "Forerunner 255"}
	if err := db.UpsertDevice(dev); err != nil {
		t.Fatalf("upsert device: %v", err)
	}
	return db, dev.ID
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestImportRealMonitorFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir, "MONITOR_2026-09-01_16-29-26_185.fit"))
	if err != nil {
		t.Skipf("no monitor capture: %v", err)
	}
	db, deviceID := testDB(t)
	im := New(db, quietLogger())

	res, err := im.Import(deviceID, 32, data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.FileType != "MONITOR" {
		t.Errorf("file type = %q, want MONITOR", res.FileType)
	}
	if res.Activity == 0 {
		t.Error("expected activity samples")
	}
	if res.Stress == 0 {
		t.Error("expected stress samples")
	}
	if res.BodyEnergy == 0 {
		t.Error("expected body energy samples")
	}
	if res.Respiration == 0 {
		t.Error("expected respiration samples")
	}

	// The samples must land inside the window the file claims to cover.
	samples, err := db.ActivitySamples(deviceID, 0, time.Now().Add(24*time.Hour).UnixMilli())
	if err != nil {
		t.Fatalf("read samples: %v", err)
	}
	if len(samples) == 0 {
		t.Fatal("no activity samples stored")
	}
	for _, s := range samples {
		if s.TsMs < 1_500_000_000_000 {
			t.Fatalf("implausible timestamp %d", s.TsMs)
		}
	}
	t.Logf("stored %d activity samples, first %s last %s", len(samples),
		time.UnixMilli(samples[0].TsMs).Format(time.RFC3339),
		time.UnixMilli(samples[len(samples)-1].TsMs).Format(time.RFC3339))
}

// TestCumulativeStepsBecomeDeltas guards the trap that sank naive ports: the
// watch reports running totals, so the dashboard must difference them instead
// of summing raw sample values.
func TestCumulativeStepsBecomeDeltas(t *testing.T) {
	db, deviceID := testDB(t)

	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)
	base := day.Add(8 * time.Hour)
	cumulative := []int{100, 250, 400, 900}
	samples := make([]store.ActivitySample, 0, len(cumulative))
	for i, v := range cumulative {
		samples = append(samples, store.ActivitySample{
			TsMs:      base.Add(time.Duration(i) * time.Minute).UnixMilli(),
			Steps:     v,
			HeartRate: 70 + i,
			RawKind:   KindActivity,
		})
	}
	if err := db.PutActivitySamples(deviceID, samples); err != nil {
		t.Fatalf("put samples: %v", err)
	}

	e := analytics.New(db, deviceID, analytics.DefaultSettings(), time.Local)
	totals := e.DayTotals(day)

	// The counter resets at midnight, so the day's first reading is already
	// the total walked so far (100) and the rest add their differences:
	// 100 + 150 + 150 + 500. This matches upstream convertCumulativeSteps,
	// which only rebases the first sample when an earlier one precedes it.
	if want := 900; totals.Steps != want {
		t.Errorf("steps = %d, want %d (deltas, not the raw sum %d)", totals.Steps, want, 1650)
	}
}

func TestImportChangelogIsHarmless(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir, "CHANGELOG_194.fit"))
	if err != nil {
		t.Skipf("no changelog capture: %v", err)
	}
	db, deviceID := testDB(t)
	im := New(db, quietLogger())

	res, err := im.Import(deviceID, 41, data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Records == 0 {
		t.Error("expected records to decode")
	}
	if res.Activity != 0 {
		t.Errorf("changelog produced %d activity samples", res.Activity)
	}
}

// TestReimportSameFileIsIdempotent guards the bug that doubled the dashboard
// numbers: importing the same MONITOR file twice must not change any total,
// because samples are keyed on (device, timestamp) and hold cumulative values.
func TestReimportSameFileIsIdempotent(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir, "MONITOR_2026-09-01_16-29-26_185.fit"))
	if err != nil {
		t.Skipf("no monitor capture: %v", err)
	}
	db, deviceID := testDB(t)
	im := New(db, quietLogger())

	if _, err := im.Import(deviceID, 32, data); err != nil {
		t.Fatalf("first import: %v", err)
	}
	first, err := db.ActivitySamples(deviceID, 0, 1<<62)
	if err != nil {
		t.Fatalf("read samples: %v", err)
	}
	firstSteps := 0
	for _, s := range first {
		firstSteps += s.Steps
	}

	if _, err := im.Import(deviceID, 32, data); err != nil {
		t.Fatalf("second import: %v", err)
	}
	second, err := db.ActivitySamples(deviceID, 0, 1<<62)
	if err != nil {
		t.Fatalf("read samples again: %v", err)
	}
	if len(second) != len(first) {
		t.Errorf("sample count changed after reimport: %d -> %d", len(first), len(second))
	}
	secondSteps := 0
	for _, s := range second {
		secondSteps += s.Steps
	}
	if secondSteps != firstSteps {
		t.Errorf("cumulative steps changed after reimport: %d -> %d", firstSteps, secondSteps)
	}
}

// TestGapsCarryCumulativeCounters covers the not-worn filler: those synthetic
// minutes must repeat the last cumulative value, never drop to zero. A zero
// dip made the delta pass see a counter reset and credit the whole counter a
// second time when real samples resumed, which is what doubled the dashboard.
func TestGapsCarryCumulativeCounters(t *testing.T) {
	db, deviceID := testDB(t)

	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)
	base := day.Add(8 * time.Hour)

	// 1000 steps, an unworn gap carrying the counter, then 1200.
	samples := []store.ActivitySample{
		{TsMs: base.UnixMilli(), Steps: 1000, HeartRate: 70, RawKind: KindActivity},
		{TsMs: base.Add(1 * time.Minute).UnixMilli(), Steps: 1000, HeartRate: KindNotMeasured, RawKind: KindNotWorn},
		{TsMs: base.Add(2 * time.Minute).UnixMilli(), Steps: 1000, HeartRate: KindNotMeasured, RawKind: KindNotWorn},
		{TsMs: base.Add(3 * time.Minute).UnixMilli(), Steps: 1200, HeartRate: 72, RawKind: KindActivity},
	}
	if err := db.PutActivitySamples(deviceID, samples); err != nil {
		t.Fatalf("put samples: %v", err)
	}

	e := analytics.New(db, deviceID, analytics.DefaultSettings(), time.Local)
	// The day is worth the final counter value: 1000 before the gap plus the
	// 200 after it. Re-crediting across the gap would report 2200.
	if got, want := e.DayTotals(day).Steps, 1200; got != want {
		t.Errorf("steps = %d, want %d (gap must not re-credit the counter)", got, want)
	}
}

// TestZeroDipDoesNotRecreditCounter reproduces the exact on-device failure:
// zero-valued filler minutes between two identical cumulative readings must
// not turn into a second full credit.
func TestZeroDipDoesNotRecreditCounter(t *testing.T) {
	db, deviceID := testDB(t)

	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)
	base := day.Add(19 * time.Hour)

	samples := []store.ActivitySample{
		{TsMs: base.UnixMilli(), Steps: 8171, HeartRate: 70, RawKind: KindActivity},
		{TsMs: base.Add(1 * time.Minute).UnixMilli(), Steps: 0, HeartRate: KindNotMeasured, RawKind: KindNotWorn},
		{TsMs: base.Add(2 * time.Minute).UnixMilli(), Steps: 0, HeartRate: KindNotMeasured, RawKind: KindNotWorn},
		{TsMs: base.Add(3 * time.Minute).UnixMilli(), Steps: 8171, HeartRate: 71, RawKind: KindActivity},
	}
	if err := db.PutActivitySamples(deviceID, samples); err != nil {
		t.Fatalf("put samples: %v", err)
	}

	e := analytics.New(db, deviceID, analytics.DefaultSettings(), time.Local)
	if got, want := e.DayTotals(day).Steps, 8171; got != want {
		t.Errorf("steps = %d, want %d (was doubled to 16342 before the fix)", got, want)
	}
}

// TestImportSleepFromMetricsFile pins the Forerunner 255 behaviour: the watch
// never offers the per-stage SLEEP file over the classic transfer, and ships
// the night as a DAILY_SLEEP record inside METRICS. Reading it with the wrong
// field names (start_timestamp/end_timestamp) silently produced no sleep at
// all, which is what the dashboard showed.
func TestImportSleepFromMetricsFile(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir, "METRICS_2026-09-02_daily-sleep_185.fit"))
	if err != nil {
		t.Skipf("no metrics capture: %v", err)
	}
	db, deviceID := testDB(t)
	im := New(db, quietLogger())

	res, err := im.Import(deviceID, 44, data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.FileType != "METRICS" {
		t.Errorf("file type = %q, want METRICS", res.FileType)
	}
	if res.SleepSessions != 1 {
		t.Fatalf("sleep sessions = %d, want 1", res.SleepSessions)
	}

	sessions, err := db.SleepSessions(deviceID, 0, time.Now().AddDate(1, 0, 0).UnixMilli())
	if err != nil {
		t.Fatalf("read sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("stored sessions = %d, want 1", len(sessions))
	}
	s := sessions[0]
	if s.Score != 74 {
		t.Errorf("score = %d, want 74", s.Score)
	}
	if got := (s.EndMs - s.StartMs) / 60000; got != 356 {
		t.Errorf("window = %d min, want 356", got)
	}
	if s.AwakeMs != 994*1000 {
		t.Errorf("awake = %d ms, want %d", s.AwakeMs, 994*1000)
	}
	if s.StartBodyBattery != 13 || s.EndBodyBattery != 59 {
		t.Errorf("body battery = %d -> %d, want 13 -> 59", s.StartBodyBattery, s.EndBodyBattery)
	}

	// The night must reach the dashboard even though no stage data exists.
	e := analytics.New(db, deviceID, analytics.DefaultSettings(), time.Local)
	day := time.UnixMilli(s.EndMs).Local()
	rep := e.Sleep(day)
	if rep.HasStages {
		t.Error("hasStages must stay false: the file carries no stage data")
	}
	if rep.Score != 74 {
		t.Errorf("report score = %d, want 74", rep.Score)
	}
	if rep.AsleepMinutes != 340 {
		t.Errorf("asleep = %d min, want 340 (356 in bed minus 16 awake)", rep.AsleepMinutes)
	}
	if rep.StartMs != s.StartMs || rep.EndMs != s.EndMs {
		t.Error("report must carry the sleep window")
	}
	if got := e.Today(day).SleepMinutes; got != 340 {
		t.Errorf("today sleep = %d min, want 340", got)
	}
}
