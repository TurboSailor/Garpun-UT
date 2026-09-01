package fit

import (
	"encoding/binary"
	"math"
	"testing"
)

// walkDefinitions counts the leading definition records and returns the offset
// of the first data record, asserting the stream keeps definitions up front.
func walkDefinitions(t *testing.T, stream []byte) (count, offset int) {
	t.Helper()
	pos := 0
	for pos < len(stream) && stream[pos]&headerDefinition != 0 {
		if pos+6 > len(stream) {
			t.Fatalf("truncated definition at %d", pos)
		}
		pos += 6 + 3*int(stream[pos+5])
		count++
	}
	for p := pos; p < len(stream); {
		if stream[p]&headerDefinition != 0 {
			t.Fatalf("definition record at %d follows data records", p)
		}
		// Data record length is unknown here; rely on Decode for the rest.
		break
	}
	return count, pos
}

func TestBuilderRoundTrip(t *testing.T) {
	const created = int64(1767225600) // 2026-01-01T00:00:00Z
	const lat, lon = 55.755800, 37.617300

	b := NewBuilder()
	if err := b.Add("FILE_ID", map[string]any{
		"type":          32,
		"manufacturer":  1,
		"product":       65534,
		"serial_number": 1,
		"time_created":  created,
		"number":        1,
		"product_name":  "PulseUT",
	}); err != nil {
		t.Fatalf("FILE_ID: %v", err)
	}

	weather := func(ts int64, temp int, uv float64, aqi any) map[string]any {
		return map[string]any{
			"weather_report":            0,
			"temperature":               temp,
			"condition":                 4,
			"wind_direction":            200,
			"wind_speed":                3611,
			"precipitation_probability": 55,
			"temperature_feels_like":    -12,
			"relative_humidity":         81,
			"location":                  "Москва",
			"observed_at_time":          ts - 600,
			"observed_location_lat":     lat,
			"observed_location_long":    lon,
			"uv_index":                  uv,
			"air_quality":               aqi,
			"atmospheric_pressure":      101325,
			"timestamp":                 ts,
		}
	}
	if err := b.Add("WEATHER", weather(created, -7, 3.5, nil)); err != nil {
		t.Fatalf("WEATHER: %v", err)
	}
	// Same field set: must reuse the local message type.
	if err := b.Add("weather", weather(created+3600, 2, 0.25, 3)); err != nil {
		t.Fatalf("WEATHER hour: %v", err)
	}
	if err := b.Add("MONITORING", map[string]any{
		"heart_rate":  62,
		"active_time": 12.5,   // scale 1000
		"temperature": -21.53, // scale 100
		"intensity":   3.4,    // scale 10
		"distance_16": 1234,
		"timestamp":   created + 60,
	}); err != nil {
		t.Fatalf("MONITORING: %v", err)
	}
	// Different field set for the same global message: new local type.
	if err := b.Add("MONITORING", map[string]any{
		"cycles_16": 500,
		"timestamp": created + 120,
	}); err != nil {
		t.Fatalf("MONITORING 2: %v", err)
	}

	stream := b.Records()
	defCount, dataOffset := walkDefinitions(t, stream)
	if defCount != 4 {
		t.Fatalf("definitions = %d, want 4 (FILE_ID, WEATHER, MONITORING x2)", defCount)
	}
	if defs, data := SplitRecords(stream); len(defs) != dataOffset || len(data) != len(stream)-dataOffset {
		t.Fatalf("SplitRecords boundary %d/%d, want %d", len(defs), len(data), dataOffset)
	}

	file := b.File()
	if file[0] != fileHeaderSize || file[1] != outProtocolVersion {
		t.Fatalf("header = %x", file[:14])
	}
	if got := binary.LittleEndian.Uint16(file[2:4]); got != outProfileVersion {
		t.Fatalf("profile version = %d", got)
	}
	if string(file[8:12]) != ".FIT" {
		t.Fatalf("magic = %q", file[8:12])
	}
	if got := binary.LittleEndian.Uint16(file[12:14]); got != CRC16(0, file[:12]) {
		t.Fatalf("header crc = %04x", got)
	}
	if got := int(binary.LittleEndian.Uint32(file[4:8])); got != len(stream) {
		t.Fatalf("data size = %d, want %d", got, len(stream))
	}
	if err := VerifyCRC(file); err != nil {
		t.Fatalf("VerifyCRC: %v", err)
	}

	f, err := Decode(file)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if f.ProtocolVersion != outProtocolVersion || f.ProfileVersion != outProfileVersion {
		t.Fatalf("versions = %d/%d", f.ProtocolVersion, f.ProfileVersion)
	}
	if len(f.Records) != 5 {
		t.Fatalf("records = %d, want 5", len(f.Records))
	}

	ids := f.Of(MsgFileID)
	if len(ids) != 1 {
		t.Fatalf("FILE_ID records = %d", len(ids))
	}
	if v, ok := ids[0].Int("time_created"); !ok || v != created {
		t.Fatalf("time_created = %d/%v, want %d", v, ok, created)
	}
	if v, ok := ids[0].Str("product_name"); !ok || v != "PulseUT" {
		t.Fatalf("product_name = %q/%v", v, ok)
	}
	if v, ok := ids[0].Int("type"); !ok || v != 32 {
		t.Fatalf("type = %d/%v", v, ok)
	}

	w := f.Of(MsgWeather)
	if len(w) != 2 {
		t.Fatalf("WEATHER records = %d", len(w))
	}
	if w[0].Timestamp != created {
		t.Fatalf("weather timestamp = %d, want %d", w[0].Timestamp, created)
	}
	if v, ok := w[0].Int("observed_at_time"); !ok || v != created-600 {
		t.Fatalf("observed_at_time = %d/%v", v, ok)
	}
	if v, ok := w[0].Int("temperature"); !ok || v != -7 {
		t.Fatalf("temperature = %d/%v", v, ok)
	}
	if v, ok := w[0].Int("temperature_feels_like"); !ok || v != -12 {
		t.Fatalf("feels like = %d/%v", v, ok)
	}
	if v, ok := w[0].Str("location"); !ok || v != "Москва" {
		t.Fatalf("location = %q/%v", v, ok)
	}
	if v, ok := w[0].Float("observed_location_lat"); !ok || math.Abs(v-lat) > 1e-6 {
		t.Fatalf("lat = %v/%v, want %v", v, ok, lat)
	}
	if v, ok := w[0].Float("observed_location_long"); !ok || math.Abs(v-lon) > 1e-6 {
		t.Fatalf("lon = %v/%v, want %v", v, ok, lon)
	}
	if v, ok := w[0].Float("uv_index"); !ok || math.Abs(v-3.5) > 1e-6 {
		t.Fatalf("uv_index = %v/%v", v, ok)
	}
	if v, ok := w[0].Int("atmospheric_pressure"); !ok || v != 101325 {
		t.Fatalf("pressure = %d/%v", v, ok)
	}
	// nil declares the field but writes the invalid marker, so it decodes away.
	if _, ok := w[0].Fields["air_quality"]; ok {
		t.Fatalf("air_quality should be absent, got %v", w[0].Fields["air_quality"])
	}
	if v, ok := w[1].Int("air_quality"); !ok || v != 3 {
		t.Fatalf("air_quality[1] = %d/%v", v, ok)
	}
	if v, ok := w[1].Float("uv_index"); !ok || math.Abs(v-0.25) > 1e-9 {
		t.Fatalf("uv_index[1] = %v/%v", v, ok)
	}
	if w[1].Timestamp != created+3600 {
		t.Fatalf("weather[1] timestamp = %d", w[1].Timestamp)
	}

	m := f.Of(MsgMonitoring)
	if len(m) != 2 {
		t.Fatalf("MONITORING records = %d", len(m))
	}
	if v, ok := m[0].Int("heart_rate"); !ok || v != 62 {
		t.Fatalf("heart_rate = %d/%v", v, ok)
	}
	if v, ok := m[0].Float("active_time"); !ok || math.Abs(v-12.5) > 1e-9 {
		t.Fatalf("active_time = %v/%v", v, ok)
	}
	if v, ok := m[0].Float("temperature"); !ok || math.Abs(v+21.53) > 1e-9 {
		t.Fatalf("temperature = %v/%v", v, ok)
	}
	if v, ok := m[0].Float("intensity"); !ok || math.Abs(v-3.4) > 1e-9 {
		t.Fatalf("intensity = %v/%v", v, ok)
	}
	if m[0].Timestamp != created+60 {
		t.Fatalf("monitoring timestamp = %d", m[0].Timestamp)
	}
	if v, ok := m[1].Int("cycles_16"); !ok || v != 500 {
		t.Fatalf("cycles_16 = %d/%v", v, ok)
	}
	if _, ok := m[1].Fields["heart_rate"]; ok {
		t.Fatalf("second monitoring record should not carry heart_rate")
	}
}

func TestBuilderLocalTypeCycling(t *testing.T) {
	b := NewBuilder()
	// 17 distinct field sets force the local types to wrap past 15.
	for i := range 17 {
		fields := map[string]any{"timestamp": int64(1767225600 + i)}
		for j := range i {
			fields[[]string{
				"heart_rate", "calories", "distance", "cycles", "active_time",
				"activity_type", "activity_subtype", "activity_level",
				"distance_16", "cycles_16", "active_time_16", "local_timestamp",
				"temperature", "temperature_min", "temperature_max", "intensity",
			}[j]] = j + 1
		}
		if err := b.Add("MONITORING", fields); err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
	}
	f, err := Decode(b.File())
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(f.Records) != 17 {
		t.Fatalf("records = %d, want 17", len(f.Records))
	}
	if v, ok := f.Records[16].Int("intensity"); !ok || v != 16 {
		t.Fatalf("last record intensity = %d/%v", v, ok)
	}
}

func TestBuilderErrors(t *testing.T) {
	b := NewBuilder()
	if err := b.Add("NOPE", map[string]any{"timestamp": 1}); err == nil {
		t.Fatal("expected error for unknown message")
	}
	if err := b.Add("WEATHER", map[string]any{"nope": 1}); err == nil {
		t.Fatal("expected error for unknown field")
	}
	if err := b.Add("WEATHER", nil); err == nil {
		t.Fatal("expected error for empty field set")
	}
	if err := b.Add("WEATHER", map[string]any{"location": 5}); err == nil {
		t.Fatal("expected error for non-string in a string field")
	}
	if len(b.Records()) != 0 {
		t.Fatalf("failed adds must not emit records, got %d bytes", len(b.Records()))
	}
}

func TestBuilderOutOfRangeBecomesInvalid(t *testing.T) {
	b := NewBuilder()
	if err := b.Add("WEATHER", map[string]any{
		"temperature":       500, // does not fit SINT8
		"relative_humidity": 255, // the UINT8 invalid marker itself
		"wind_direction":    120,
		"timestamp":         int64(1767225600),
	}); err != nil {
		t.Fatalf("add: %v", err)
	}
	f, err := Decode(b.File())
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	rec := f.Of(MsgWeather)[0]
	if _, ok := rec.Fields["temperature"]; ok {
		t.Fatal("out of range temperature should be invalid")
	}
	if _, ok := rec.Fields["relative_humidity"]; ok {
		t.Fatal("invalid marker value should stay invalid")
	}
	if v, ok := rec.Int("wind_direction"); !ok || v != 120 {
		t.Fatalf("wind_direction = %d/%v", v, ok)
	}
}
