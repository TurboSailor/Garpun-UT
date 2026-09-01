package weather

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"pulse/backend/internal/fit"
)

// fakeOpenMeteo serves a canned forecast anchored on the current hour and
// counts requests so the cache can be observed.
func fakeOpenMeteo(t *testing.T, hits *atomic.Int64, query *atomic.Pointer[url.Values]) *httptest.Server {
	t.Helper()
	hour := time.Now().Truncate(time.Hour).Unix()
	day := time.Now().Truncate(time.Hour).Add(-2 * time.Hour).Unix()

	hourlyTimes := make([]int64, 0, 30)
	for i := range 30 {
		hourlyTimes = append(hourlyTimes, hour+int64(i)*3600)
	}
	series := func(f func(i int) float64) []float64 {
		out := make([]float64, len(hourlyTimes))
		for i := range out {
			out[i] = f(i)
		}
		return out
	}

	body := map[string]any{
		"latitude":           55.75,
		"longitude":          37.62,
		"timezone":           "Europe/Moscow",
		"utc_offset_seconds": 3 * 3600,
		"current": map[string]any{
			"time":                 hour,
			"temperature_2m":       -6.7,
			"relative_humidity_2m": 81,
			"apparent_temperature": -11.4,
			"weather_code":         73, // moderate snow -> OWM 601 -> FIT SNOW(4)
			"wind_speed_10m":       13.0,
			"wind_direction_10m":   200,
			"surface_pressure":     1013.25,
			"cloud_cover":          90,
			"dew_point_2m":         -9.1,
		},
		"hourly": map[string]any{
			"time":                      hourlyTimes,
			"temperature_2m":            series(func(i int) float64 { return -6.7 + float64(i) }),
			"apparent_temperature":      series(func(i int) float64 { return -11.4 + float64(i) }),
			"weather_code":              series(func(int) float64 { return 3 }), // -> OWM 804 -> CLOUDY(22)
			"relative_humidity_2m":      series(func(int) float64 { return 77 }),
			"wind_speed_10m":            series(func(int) float64 { return 18 }),
			"wind_direction_10m":        series(func(int) float64 { return 355 }),
			"precipitation_probability": series(func(int) float64 { return 40 }),
			"uv_index":                  series(func(int) float64 { return 12.5 }), // clamps to 10
			"dew_point_2m":              series(func(int) float64 { return -9 }),
			"visibility":                series(func(int) float64 { return 24140 }),
			"surface_pressure":          series(func(int) float64 { return 1009.5 }),
			"cloud_cover":               series(func(int) float64 { return 88 }),
		},
		"daily": map[string]any{
			"time":                          []int64{day, day + 86400, day + 2*86400, day + 3*86400, day + 4*86400, day + 5*86400, day + 6*86400},
			"weather_code":                  []float64{71, 3, 0, 95, 61, 1, 2},
			"temperature_2m_max":            []float64{-2.1, 0.4, 3.2, 5.0, 6.1, 7.7, 8.3},
			"temperature_2m_min":            []float64{-9.4, -6.2, -3.0, -1.1, 0.5, 1.2, 2.0},
			"sunrise":                       []int64{day + 30000, day + 116400, day + 202800, day + 289200, day + 375600, day + 462000, day + 548400},
			"sunset":                        []int64{day + 60000, day + 146400, day + 232800, day + 319200, day + 405600, day + 492000, day + 578400},
			"uv_index_max":                  []float64{1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5},
			"precipitation_probability_max": []float64{65, 30, 5, 80, 55, 10, 15},
			"wind_speed_10m_max":            []float64{21.6, 18.0, 14.4, 25.2, 20.0, 12.0, 10.0},
			"wind_direction_10m_dominant":   []float64{190, 210, 240, 270, 300, 330, 10},
		},
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if query != nil {
			q := r.URL.Query()
			query.Store(&q)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func newTestService(t *testing.T, srv *httptest.Server) *Service {
	t.Helper()
	return New(nil, Options{BaseURL: srv.URL, Client: srv.Client()})
}

// wrapFile turns a bare record stream into a decodable FIT file: FitPayload
// emits only records, since that is what the GFDI transport carries.
func wrapFile(records []byte) []byte {
	out := make([]byte, 14, 14+len(records)+2)
	out[0] = 14
	out[1] = 16
	binary.LittleEndian.PutUint16(out[2:4], 21117)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(records)))
	copy(out[8:12], ".FIT")
	binary.LittleEndian.PutUint16(out[12:14], fit.CRC16(0, out[:12]))
	out = append(out, records...)
	return binary.LittleEndian.AppendUint16(out, fit.CRC16(0, out))
}

func TestSemicirclesToDegrees(t *testing.T) {
	// The watch sends 55.7558 N as this value in WEATHER_REQUEST.
	const semicircles = int32(665192604)
	if got := SemicirclesToDegrees(semicircles); math.Abs(got-55.7558) > 1e-6 {
		t.Fatalf("got %v, want 55.7558", got)
	}
	if got := SemicirclesToDegrees(0); got != 0 {
		t.Fatalf("got %v, want 0", got)
	}
	// Southern/western coordinates arrive as negative semicircles.
	if got := SemicirclesToDegrees(-semicircles); math.Abs(got+55.7558) > 1e-6 {
		t.Fatalf("got %v, want -55.7558", got)
	}
}

func TestFitPayloadRecords(t *testing.T) {
	var hits atomic.Int64
	var query atomic.Pointer[url.Values]
	srv := fakeOpenMeteo(t, &hits, &query)
	defer srv.Close()
	s := newTestService(t, srv)

	blob, err := s.FitPayload(context.Background(), 55.7558, 37.6173, 6)
	if err != nil {
		t.Fatalf("FitPayload: %v", err)
	}
	if q := query.Load(); q == nil || q.Get("wind_speed_unit") != "kmh" {
		t.Fatalf("wind speed unit = %v, want kmh", q)
	}
	// Definitions must precede data so the GFDI split works.
	defs, data := fit.SplitRecords(blob)
	if len(defs) == 0 || len(data) == 0 {
		t.Fatalf("split = %d/%d bytes", len(defs), len(data))
	}

	f, err := fit.Decode(wrapFile(blob))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	recs := f.Of(fit.MsgWeather)
	if len(recs) != 1+6+forecastDayRecords {
		t.Fatalf("weather records = %d, want %d", len(recs), 1+6+forecastDayRecords)
	}

	cur := recs[0]
	if v, _ := cur.Int("weather_report"); v != reportCurrent {
		t.Fatalf("weather_report = %d", v)
	}
	// -6.7 C -> 266 K -> 266-273 = -7 on the wire (issue #4313 shift).
	if v, ok := cur.Int("temperature"); !ok || v != -7 {
		t.Fatalf("temperature = %d/%v, want -7", v, ok)
	}
	if v, ok := cur.Int("temperature_feels_like"); !ok || v != -11 {
		t.Fatalf("feels like = %d/%v, want -11", v, ok)
	}
	if v, ok := cur.Int("dew_point"); !ok || v != -9 {
		t.Fatalf("dew point = %d/%v, want -9", v, ok)
	}
	if v, ok := cur.Int("condition"); !ok || v != condSnow {
		t.Fatalf("condition = %d/%v, want %d", v, ok, condSnow)
	}
	// 13 km/h -> 3611 mm/s.
	if v, ok := cur.Int("wind_speed"); !ok || v != 3611 {
		t.Fatalf("wind speed = %d/%v, want 3611", v, ok)
	}
	if v, ok := cur.Int("wind_direction"); !ok || v != 200 {
		t.Fatalf("wind direction = %d/%v", v, ok)
	}
	if v, ok := cur.Int("relative_humidity"); !ok || v != 81 {
		t.Fatalf("humidity = %d/%v", v, ok)
	}
	if v, ok := cur.Str("location"); !ok || v != "Moscow" {
		t.Fatalf("location = %q/%v", v, ok)
	}
	if v, ok := cur.Float("observed_location_lat"); !ok || math.Abs(v-55.7558) > 1e-5 {
		t.Fatalf("lat = %v/%v", v, ok)
	}
	if v, ok := cur.Float("observed_location_long"); !ok || math.Abs(v-37.6173) > 1e-5 {
		t.Fatalf("lon = %v/%v", v, ok)
	}
	// Today's high/low come from daily[0]: 5.0 -> 278 K -> 5, -9.4 -> 264 -> -9.
	if v, ok := cur.Int("high_temperature"); !ok || v != -2 {
		t.Fatalf("high = %d/%v, want -2", v, ok)
	}
	if v, ok := cur.Int("low_temperature"); !ok || v != -9 {
		t.Fatalf("low = %d/%v, want -9", v, ok)
	}
	if _, ok := cur.Fields["air_quality"]; ok {
		t.Fatal("air_quality must stay invalid, Open-Meteo forecast has no AQI")
	}
	if cur.Timestamp == 0 {
		t.Fatal("current record has no timestamp")
	}

	hour := recs[1]
	if v, _ := hour.Int("weather_report"); v != reportHourly {
		t.Fatalf("hourly weather_report = %d", v)
	}
	if v, ok := hour.Int("condition"); !ok || v != condCloudy {
		t.Fatalf("hourly condition = %d/%v, want %d", v, ok, condCloudy)
	}
	// uv_index clamps at 10 rather than rescaling.
	if v, ok := hour.Float("uv_index"); !ok || math.Abs(v-10) > 1e-6 {
		t.Fatalf("hourly uv = %v/%v, want 10", v, ok)
	}
	// 1009.5 mbar -> 100950 Pa.
	if v, ok := hour.Int("atmospheric_pressure"); !ok || v != 100950 {
		t.Fatalf("hourly pressure = %d/%v", v, ok)
	}
	if v, ok := hour.Int("wind_direction"); !ok || v != 355 {
		t.Fatalf("hourly wind direction = %d/%v", v, ok)
	}

	// All hourly records share one field set, so they share one local type;
	// the daily block adds a third.
	first := recs[1+6]
	if v, _ := first.Int("weather_report"); v != reportDaily {
		t.Fatalf("daily weather_report = %d", v)
	}
	if v, ok := first.Int("day_of_week"); !ok || v != int64(weekdayAt(first.Timestamp, 3*3600)) {
		t.Fatalf("day_of_week = %d/%v", v, ok)
	}
	// Each daily record carries its own day, one 24h step apart.
	second := recs[1+6+1]
	if second.Timestamp-first.Timestamp != 86400 {
		t.Fatalf("daily timestamps %d and %d are not a day apart", first.Timestamp, second.Timestamp)
	}
	if v, ok := second.Int("condition"); !ok || v != condCloudy {
		t.Fatalf("daily[1] condition = %d/%v", v, ok)
	}
	if _, ok := first.Fields["relative_humidity"]; ok {
		t.Fatal("Open-Meteo has no daily humidity; field must be invalid")
	}
	if v, ok := first.Int("wind_speed"); !ok || v != 6000 {
		t.Fatalf("daily wind speed = %d/%v, want 6000", v, ok)
	}

	if hits.Load() != 1 {
		t.Fatalf("http hits = %d, want 1", hits.Load())
	}
}

func TestFitPayloadLocalTypeReuse(t *testing.T) {
	var hits atomic.Int64
	srv := fakeOpenMeteo(t, &hits, nil)
	defer srv.Close()
	s := newTestService(t, srv)

	blob, err := s.FitPayload(context.Background(), 55.7558, 37.6173, 12)
	if err != nil {
		t.Fatalf("FitPayload: %v", err)
	}
	defs, _ := fit.SplitRecords(blob)
	count, pos := 0, 0
	for pos < len(defs) {
		pos += 6 + 3*int(defs[pos+5])
		count++
	}
	if count != 3 {
		t.Fatalf("definitions = %d, want 3 (current, hourly, daily)", count)
	}
}

func TestCacheAndStaleFallback(t *testing.T) {
	var hits atomic.Int64
	srv := fakeOpenMeteo(t, &hits, nil)
	s := New(nil, Options{BaseURL: srv.URL, Client: srv.Client(), TTL: time.Hour})

	for range 3 {
		if _, err := s.Current(context.Background(), 55.7558, 37.6173); err != nil {
			t.Fatalf("Current: %v", err)
		}
	}
	// Rounding to four decimals keeps near-identical coordinates on one entry.
	if _, err := s.Daily(context.Background(), 55.75581, 37.61731, 3); err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("http hits = %d, want 1", hits.Load())
	}
	// A different location is a different cache key.
	if _, err := s.Current(context.Background(), 48.8566, 2.3522); err != nil {
		t.Fatalf("Current Paris: %v", err)
	}
	if hits.Load() != 2 {
		t.Fatalf("http hits = %d, want 2", hits.Load())
	}

	// Once the server is gone a stale forecast still answers.
	srv.Close()
	s.mu.Lock()
	for k, e := range s.cache {
		s.cache[k] = cached{at: e.at.Add(-2 * time.Hour), spec: e.spec}
	}
	s.mu.Unlock()
	if _, err := s.Current(context.Background(), 55.7558, 37.6173); err != nil {
		t.Fatalf("stale fallback: %v", err)
	}
}

func TestFetchErrors(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"reason":"Cannot initialize WeatherVariable"}`))
	}))
	defer bad.Close()
	s := newTestService(t, bad)
	_, err := s.FitPayload(context.Background(), 1, 2, 12)
	if err == nil || !strings.Contains(err.Error(), "http 400") {
		t.Fatalf("err = %v, want an http 400 error", err)
	}
	if !strings.Contains(err.Error(), "Cannot initialize") {
		t.Fatalf("err = %v, want the upstream reason included", err)
	}

	// An unreachable endpoint must surface an error, not panic.
	dead := New(nil, Options{BaseURL: "http://127.0.0.1:1/forecast", Client: &http.Client{Timeout: time.Second}})
	if _, err := dead.Current(context.Background(), 1, 2); err == nil {
		t.Fatal("expected a network error")
	}
}

func TestProxyShapes(t *testing.T) {
	var hits atomic.Int64
	srv := fakeOpenMeteo(t, &hits, nil)
	defer srv.Close()
	s := newTestService(t, srv)
	ctx := context.Background()

	cur, err := s.Current(ctx, 55.7558, 37.6173)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	c := cur.(*Current)
	if c.Temperature == nil || c.Temperature.Value != -7 || c.Temperature.Units != unitCelsius {
		t.Fatalf("temperature = %+v", c.Temperature)
	}
	if c.Wind == nil || c.Wind.Speed == nil || math.Abs(c.Wind.Speed.Value-13.0/3.6) > 1e-9 {
		t.Fatalf("wind = %+v", c.Wind)
	}
	if c.Wind.Speed.Units != unitMetersPerSec {
		t.Fatalf("wind unit = %q", c.Wind.Speed.Units)
	}
	if c.Wind.DirectionString != "S" {
		t.Fatalf("direction string = %q, want S", c.Wind.DirectionString)
	}
	if c.LocationName != "Moscow" {
		t.Fatalf("location = %q", c.LocationName)
	}
	if c.Description != "Snow" {
		t.Fatalf("description = %q", c.Description)
	}
	if c.Icon == nil || *c.Icon != 38 {
		t.Fatalf("icon = %v, want 38", c.Icon)
	}
	if c.Pressure == nil || math.Abs(c.Pressure.Value-1013.25*0.02953) > 1e-9 {
		t.Fatalf("pressure = %+v", c.Pressure)
	}
	if c.Visibility == nil || c.Visibility.Value != 24140 {
		t.Fatalf("visibility = %+v", c.Visibility)
	}

	hourly, err := s.Hourly(ctx, 55.7558, 37.6173, 13)
	if err != nil {
		t.Fatalf("Hourly: %v", err)
	}
	hours := hourly.([]Hour)
	if len(hours) != 13 {
		t.Fatalf("hours = %d, want 13", len(hours))
	}
	if hours[0].UvIndex == nil || *hours[0].UvIndex != 12.5 {
		t.Fatalf("uv = %v (the HTTP path does not clamp)", hours[0].UvIndex)
	}
	if hours[0].Pressure == nil || hours[0].Pressure.Units != unitMillibar {
		t.Fatalf("hour pressure = %+v", hours[0].Pressure)
	}
	if hours[0].Wind == nil || hours[0].Wind.DirectionString != "N" {
		t.Fatalf("hour wind = %+v", hours[0].Wind)
	}
	if hours[0].AirQuality != nil {
		t.Fatalf("airQuality = %v, want omitted", hours[0].AirQuality)
	}

	daily, err := s.Daily(ctx, 55.7558, 37.6173, 5)
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	days := daily.([]Day)
	if len(days) != 5 {
		t.Fatalf("days = %d, want 5", len(days))
	}
	if days[0].DayOfWeek < 1 || days[0].DayOfWeek > 7 {
		t.Fatalf("dayOfWeek = %d", days[0].DayOfWeek)
	}
	if days[0].High == nil || days[0].High.Value != -2 {
		t.Fatalf("high = %+v", days[0].High)
	}
	if days[0].Wind == nil || days[0].Wind.Speed == nil || days[0].Wind.Speed.Units != unitKmPerHour {
		t.Fatalf("day wind = %+v", days[0].Wind)
	}
	if days[0].EpochSunrise == nil || days[0].EpochSunset == nil {
		t.Fatal("sunrise/sunset missing")
	}
	if days[0].Humidity != nil {
		t.Fatalf("humidity = %v, want omitted", days[0].Humidity)
	}

	winds, err := s.PointWinds(ctx, 55.7558, 37.6173)
	if err != nil {
		t.Fatalf("PointWinds: %v", err)
	}
	pw := winds.(*PointWindsResponse)
	if len(pw.CcPointWinds.W) != 4 {
		t.Fatalf("point winds = %d, want 4", len(pw.CcPointWinds.W))
	}
	if pw.CcPointWinds.W[0].T != 0 || pw.CcPointWinds.W[1].T != 3600 {
		t.Fatalf("offsets = %d, %d", pw.CcPointWinds.W[0].T, pw.CcPointWinds.W[1].T)
	}
	if math.Abs(pw.CcPointWinds.W[0].S-18.0/1.852) > 1e-9 {
		t.Fatalf("knots = %v", pw.CcPointWinds.W[0].S)
	}

	// Nulls must be dropped, like Gson without serializeNulls.
	raw, err := json.Marshal(hours[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "null") {
		t.Fatalf("json carries nulls: %s", raw)
	}
	if !strings.Contains(string(raw), `"epochSeconds"`) {
		t.Fatalf("json missing epochSeconds: %s", raw)
	}
}

func TestConditionMapping(t *testing.T) {
	// WMO -> OWM -> FIT WeatherCondition, per the upstream two-step mapping.
	cases := []struct {
		wmo  int
		owm  int
		cond any
		icon int
	}{
		{0, 800, condClear, 5},
		{1, 801, condPartlyCloudy, 8},
		{2, 802, condPartlyCloudy, 8},
		{3, 804, condCloudy, 15},
		{45, 741, condFog, 47},
		{48, 741, condFog, 47},
		{51, 300, condLightRain, 17},
		{53, 301, condLightRain, 17},
		{55, 302, condHeavyRain, 17},
		{56, 511, condUnknownPrecipitation, 40},
		{57, 511, condUnknownPrecipitation, 40},
		{61, 500, condLightRain, 17},
		{63, 501, condRain, 17},
		{65, 502, condHeavyRain, 17},
		{66, 511, condUnknownPrecipitation, 40},
		{67, 511, condUnknownPrecipitation, 40},
		{71, 600, condLightSnow, 38},
		{73, 601, condSnow, 38},
		{75, 602, condHeavySnow, 38},
		{77, 611, condWintryMix, 38},
		{80, 520, condLightRain, 17},
		{81, 521, condLightRain, 17},
		{82, 522, condHeavyRain, 17},
		{85, 620, condSnow, 38},
		{86, 621, condSnow, 38},
		{95, 211, condThunderstorms, 27},
		{96, 212, condThunderstorms, 27},
		{99, 212, condThunderstorms, 27},
		{123, 800, condClear, 5}, // unknown WMO falls back to clear
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("wmo%d", c.wmo), func(t *testing.T) {
			owm := wmoToOwm(c.wmo)
			if owm != c.owm {
				t.Fatalf("owm = %d, want %d", owm, c.owm)
			}
			if got := owmToFitCondition(owm); got != c.cond {
				t.Fatalf("condition = %v, want %v", got, c.cond)
			}
			if got := owmToGarminIcon(owm); got != c.icon {
				t.Fatalf("icon = %d, want %d", got, c.icon)
			}
		})
	}
	// Codes Garmin has no bucket for stay invalid rather than becoming clear.
	for _, code := range []int{903, 904, 951, 960, 961} {
		if got := owmToFitCondition(code); got != nil {
			t.Fatalf("owm %d mapped to %v, want nil", code, got)
		}
	}
}

func TestFitConversions(t *testing.T) {
	if got := fitTemp(0); got != nil {
		t.Fatalf("unknown temperature = %v, want nil", got)
	}
	if got := fitTemp(300); got != 27 {
		t.Fatalf("300 K = %v, want 27", got)
	}
	if got := fitWindSpeed(-1); got != nil {
		t.Fatalf("unknown wind = %v", got)
	}
	if got := fitWindSpeed(1000); got != 0xFFFE {
		t.Fatalf("clamped wind = %v, want 65534", got)
	}
	if got := fitWindDirection(725); got != 5 {
		t.Fatalf("725 deg = %v, want 5", got)
	}
	if got := fitPercent(140); got != 100 {
		t.Fatalf("140%% = %v, want 100", got)
	}
	if got := fitUVIndex(15); got != 10.0 {
		t.Fatalf("uv 15 = %v, want 10", got)
	}
	if got := fitPressure(1013.25); got != int64(101325) {
		t.Fatalf("pressure = %v, want 101325", got)
	}
	if got := fitPressure(0); got != nil {
		t.Fatalf("unknown pressure = %v", got)
	}
	if got := compassPoint(-45); got != "NW" {
		t.Fatalf("compass(-45) = %q, want NW", got)
	}
	if got := locationFromTimezone("America/New_York"); got != "New York" {
		t.Fatalf("location = %q", got)
	}
}

func TestWeekdayUsesForecastOffset(t *testing.T) {
	// Open-Meteo reports daily timestamps as local midnight, which in Auckland
	// (+13) is still the previous day in UTC. The watch shows the local day.
	const offset = 13 * 3600
	tuesday := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Unix() - offset
	if got := weekdayAt(tuesday, offset); got != time.Tuesday {
		t.Fatalf("weekday = %v, want Tuesday", got)
	}
	if got := isoDayOfWeek(tuesday, offset); got != 2 {
		t.Fatalf("iso day = %d, want 2", got)
	}
	if got := fitDayOfWeek(tuesday, offset); got != 2 {
		t.Fatalf("fit day = %v, want 2", got)
	}
	// Sunday is 0 for FIT and 7 for the v2 JSON.
	sunday := time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC).Unix() - offset
	if got := fitDayOfWeek(sunday, offset); got != 0 {
		t.Fatalf("fit sunday = %v, want 0", got)
	}
	if got := isoDayOfWeek(sunday, offset); got != 7 {
		t.Fatalf("iso sunday = %d, want 7", got)
	}
}
