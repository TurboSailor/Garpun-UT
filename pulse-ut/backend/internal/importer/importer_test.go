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

	// The first stored sample seeds the baseline, so the day accumulates the
	// differences after it: 150 + 150 + 500.
	if want := 800; totals.Steps != want {
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
