package fit

import (
	"os"
	"path/filepath"
	"testing"
)

// Real files pulled off a Forerunner 255 over the Go GFDI implementation.
const testdataDir = "../../../testdata/fitdump"

func TestDecodeRealMonitorFiles(t *testing.T) {
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		t.Skipf("no captured FIT files: %v", err)
	}
	var decoded int
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".fit" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(testdataDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if err := VerifyCRC(data); err != nil {
			t.Errorf("%s: %v", e.Name(), err)
		}
		f, err := Decode(data)
		if err != nil {
			t.Errorf("%s: decode: %v", e.Name(), err)
			continue
		}
		if len(f.Records) == 0 {
			t.Errorf("%s: no records", e.Name())
			continue
		}
		if ids := f.Of(MsgFileID); len(ids) == 0 {
			t.Errorf("%s: no file_id record", e.Name())
		}
		decoded++
		t.Logf("%s: %d records", e.Name(), len(f.Records))
	}
	if decoded == 0 {
		t.Skip("no FIT files to decode")
	}
}

func TestMonitoringFieldsResolve(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testdataDir, "MONITOR_2026-09-01_16-29-26_185.fit"))
	if err != nil {
		t.Skipf("no monitor capture: %v", err)
	}
	f, err := Decode(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	mon := f.Of(MsgMonitoring)
	if len(mon) == 0 {
		t.Fatal("no MONITORING records")
	}
	var withSteps int
	for _, r := range mon {
		if _, ok := r.Int("cycles"); ok {
			withSteps++
		}
		if r.Timestamp != 0 && r.Timestamp < GarminEpoch {
			t.Fatalf("timestamp not converted to unix: %d", r.Timestamp)
		}
	}
	t.Logf("%d monitoring records, %d with cycles", len(mon), withSteps)
	if withSteps == 0 {
		t.Error("expected at least one record carrying cycles")
	}
}
