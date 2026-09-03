package importer

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pulse/backend/internal/analytics"
	"pulse/backend/internal/fit"
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

// TestSawToothCounterCreditsOnlyGrowth covers the shape that inflated active
// calories several times over: a cumulative row that dips because the watch
// left an activity type out of one minute. Only the growth above the
// high-water mark may be credited, so 200 -> 121 -> 201 is worth one calorie,
// not another 121.
func TestSawToothCounterCreditsOnlyGrowth(t *testing.T) {
	db, deviceID := testDB(t)

	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local)
	base := day.Add(14 * time.Hour)
	samples := []store.ActivitySample{
		{TsMs: base.UnixMilli(), ActiveCalories: 200, HeartRate: 70, RawKind: KindActivity},
		{TsMs: base.Add(1 * time.Minute).UnixMilli(), ActiveCalories: 121, HeartRate: 71, RawKind: KindActivity},
		{TsMs: base.Add(2 * time.Minute).UnixMilli(), ActiveCalories: 201, HeartRate: 72, RawKind: KindActivity},
	}
	if err := db.PutActivitySamples(deviceID, samples); err != nil {
		t.Fatalf("put samples: %v", err)
	}

	e := analytics.New(db, deviceID, analytics.DefaultSettings(), time.Local)
	if got, want := e.DayTotals(day).ActiveCalories, 201; got != want {
		t.Errorf("active calories = %d, want %d (the dip must credit 0 and the recovery +1)", got, want)
	}
}

// TestPartialActivityTypeReportKeepsSum is the import side of the same bug.
// Garmin reports each counter per activity type and not in every record, so
// the last value of every type has to survive across timestamps: a minute
// that mentions one type only must not drop the stored total to that type's
// share.
func TestPartialActivityTypeReportKeepsSum(t *testing.T) {
	db, deviceID := testDB(t)
	base := time.Date(2026, 3, 1, 8, 0, 0, 0, time.Local).Unix()

	b := fit.NewBuilder()
	add := func(fields map[string]any) {
		if err := b.Add("MONITORING", fields); err != nil {
			t.Fatalf("build monitoring: %v", err)
		}
	}
	if err := b.Add("FILE_ID", map[string]any{"type": 32, "time_created": base}); err != nil {
		t.Fatalf("build file id: %v", err)
	}
	// Minute one: generic and walking both report. Minute two: generic only,
	// the shape that used to halve the sample. Minute three: both again.
	add(map[string]any{"timestamp": base, "activity_type": 0, "cycles": 100, "active_calories": 120, "heart_rate": 70})
	add(map[string]any{"timestamp": base, "activity_type": 6, "cycles": 50, "active_calories": 80})
	add(map[string]any{"timestamp": base + 60, "activity_type": 0, "cycles": 110, "active_calories": 121, "heart_rate": 72})
	add(map[string]any{"timestamp": base + 120, "activity_type": 0, "cycles": 120, "active_calories": 122, "heart_rate": 71})
	add(map[string]any{"timestamp": base + 120, "activity_type": 6, "cycles": 50, "active_calories": 80})

	im := New(db, quietLogger())
	if _, err := im.Import(deviceID, 32, b.File()); err != nil {
		t.Fatalf("import: %v", err)
	}

	samples, err := db.ActivitySamples(deviceID, 0, 1<<62)
	if err != nil {
		t.Fatalf("read samples: %v", err)
	}
	if len(samples) != 3 {
		t.Fatalf("stored %d samples, want 3", len(samples))
	}
	wantCal := []int{200, 201, 202}
	wantSteps := []int{150, 160, 170}
	for i, s := range samples {
		if s.ActiveCalories != wantCal[i] || s.Steps != wantSteps[i] {
			t.Errorf("sample %d = %d kcal / %d steps, want %d / %d",
				i, s.ActiveCalories, s.Steps, wantCal[i], wantSteps[i])
		}
	}

	e := analytics.New(db, deviceID, analytics.DefaultSettings(), time.Local)
	totals := e.DayTotals(time.Unix(base, 0).Local())
	if totals.ActiveCalories != 202 {
		t.Errorf("day active calories = %d, want 202 (the counter's day total)", totals.ActiveCalories)
	}
	if totals.Steps != 170 {
		t.Errorf("day steps = %d, want 170", totals.Steps)
	}
}

// TestImportSleepSummaryBreakdown covers the night on a watch that hands over
// a SLEEP file with summary and nap records but no scored stages: the minutes
// per stage must reach the report even though no hypnogram can be drawn, and
// the awake/unmeasurable-only stage records must not be stored at all.
func TestImportSleepSummaryBreakdown(t *testing.T) {
	db, deviceID := testDB(t)

	start := time.Date(2026, 3, 1, 23, 0, 0, 0, time.Local).Unix()
	end := time.Date(2026, 3, 2, 7, 0, 0, 0, time.Local).Unix()
	napStart := time.Date(2026, 3, 2, 10, 0, 0, 0, time.Local).Unix()

	b := fit.NewBuilder()
	if err := b.Add("FILE_ID", map[string]any{"type": 49, "time_created": start}); err != nil {
		t.Fatalf("build file id: %v", err)
	}
	if err := b.Add("SLEEP_SUMMARY", map[string]any{
		"timestamp": end, "sleep_start_timestamp_utc": start, "sleep_end_timestamp_utc": end,
		"sleep_score": 81, "deep_duration": 90, "light_duration": 250, "rem_duration": 80,
		"awake_duration": 60, "unmeasurable_duration": 0,
	}); err != nil {
		t.Fatalf("build sleep summary: %v", err)
	}
	if err := b.Add("NAP", map[string]any{
		"timestamp": napStart, "start_timestamp": napStart, "end_timestamp": napStart + 40*60,
	}); err != nil {
		t.Fatalf("build nap: %v", err)
	}
	// A retracted nap must be ignored.
	if err := b.Add("NAP", map[string]any{
		"timestamp": napStart, "start_timestamp": napStart + 3600,
		"end_timestamp": napStart + 5400, "deleted": 1,
	}); err != nil {
		t.Fatalf("build deleted nap: %v", err)
	}
	for i, stage := range []int{SleepAwake, SleepUnmeasurable, SleepAwake} {
		if err := b.Add("SLEEP_STAGE", map[string]any{
			"timestamp": start + int64(i+1)*600, "sleep_stage": stage,
		}); err != nil {
			t.Fatalf("build sleep stage: %v", err)
		}
	}

	im := New(db, quietLogger())
	res, err := im.Import(deviceID, 49, b.File())
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.FileType != "SLEEP" {
		t.Errorf("file type = %q, want SLEEP", res.FileType)
	}
	if res.SleepStages != 0 {
		t.Errorf("stored %d stages, want 0: awake and unmeasurable alone are no hypnogram", res.SleepStages)
	}
	if res.SleepSessions != 1 {
		t.Fatalf("sleep sessions = %d, want 1", res.SleepSessions)
	}
	if res.Naps != 1 {
		t.Errorf("naps = %d, want 1 (the deleted one must be skipped)", res.Naps)
	}

	sessions, err := db.SleepSessions(deviceID, 0, 1<<62)
	if err != nil {
		t.Fatalf("read sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("stored sessions = %d, want 1", len(sessions))
	}
	s := sessions[0]
	if s.DeepMs != 90*60000 || s.LightMs != 250*60000 || s.RemMs != 80*60000 || s.AwakeMs != 60*60000 {
		t.Errorf("stage durations = %d/%d/%d/%d ms, want 90/250/80/60 minutes",
			s.DeepMs, s.LightMs, s.RemMs, s.AwakeMs)
	}

	e := analytics.New(db, deviceID, analytics.DefaultSettings(), time.Local)
	rep := e.Sleep(time.Unix(end, 0).Local())
	if rep.HasStages {
		t.Error("hasStages must stay false without stage samples")
	}
	if !rep.HasBreakdown {
		t.Error("hasBreakdown must be true: the summary carries per-stage minutes")
	}
	if rep.Totals.Deep != 90 || rep.Totals.Light != 250 || rep.Totals.REM != 80 || rep.Totals.Awake != 60 {
		t.Errorf("report totals = %+v, want 90/250/80/60", rep.Totals)
	}
	if rep.AsleepMinutes != 420 {
		t.Errorf("asleep = %d min, want 420 (deep+light+rem)", rep.AsleepMinutes)
	}
	if rep.Score != 81 {
		t.Errorf("score = %d, want 81", rep.Score)
	}
	if len(rep.Naps) != 1 || rep.Naps[0].Minutes != 40 {
		t.Errorf("naps = %+v, want one 40 minute nap", rep.Naps)
	}
}

// TestImportSleepStagesBuildHypnogram checks the other half: real stages are
// stored, and they imply both a hypnogram and a breakdown.
func TestImportSleepStagesBuildHypnogram(t *testing.T) {
	db, deviceID := testDB(t)

	start := time.Date(2026, 3, 1, 23, 0, 0, 0, time.Local).Unix()
	b := fit.NewBuilder()
	if err := b.Add("FILE_ID", map[string]any{"type": 49, "time_created": start}); err != nil {
		t.Fatalf("build file id: %v", err)
	}
	// Stage timestamps are upper bounds, so the first record only opens the
	// window: 30 minutes light, 60 deep, 30 rem.
	stages := []struct {
		afterMin int64
		stage    int
	}{{0, SleepAwake}, {30, SleepLight}, {90, SleepDeep}, {120, SleepREM}}
	for _, s := range stages {
		if err := b.Add("SLEEP_STAGE", map[string]any{
			"timestamp": start + s.afterMin*60, "sleep_stage": s.stage,
		}); err != nil {
			t.Fatalf("build sleep stage: %v", err)
		}
	}

	im := New(db, quietLogger())
	res, err := im.Import(deviceID, 49, b.File())
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.SleepStages != 4 {
		t.Fatalf("stored %d stages, want 4", res.SleepStages)
	}

	e := analytics.New(db, deviceID, analytics.DefaultSettings(), time.Local)
	rep := e.Sleep(time.Date(2026, 3, 2, 9, 0, 0, 0, time.Local))
	if !rep.HasStages || !rep.HasBreakdown {
		t.Fatalf("hasStages = %v, hasBreakdown = %v, want both true", rep.HasStages, rep.HasBreakdown)
	}
	if rep.Totals.Light != 30 || rep.Totals.Deep != 60 || rep.Totals.REM != 30 {
		t.Errorf("totals = %+v, want light 30, deep 60, rem 30", rep.Totals)
	}
	if rep.AsleepMinutes != 120 {
		t.Errorf("asleep = %d min, want 120", rep.AsleepMinutes)
	}
	if len(rep.Stages) != 3 {
		t.Errorf("hypnogram spans = %d, want 3", len(rep.Stages))
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
	if rep.HasStages || rep.HasBreakdown {
		t.Error("hasStages/hasBreakdown must stay false: the file carries no stage data")
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
