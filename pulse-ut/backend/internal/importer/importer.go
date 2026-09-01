// Package importer turns FIT files pulled off a watch into database rows.
package importer

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"

	"pulse/backend/internal/fit"
	"pulse/backend/internal/store"
)

// Activity kinds, matching the upstream ActivityKind bit values so exported
// data stays comparable with Gadgetbridge.
const (
	KindNotMeasured = -1
	KindUnknown     = 0x0
	KindActivity    = 0x1
	KindLightSleep  = 0x2
	KindDeepSleep   = 0x4
	KindNotWorn     = 0x8
	KindREMSleep    = 0x01000000
	KindAwakeSleep  = 0x02000000
)

// Sleep stage values as emitted by the SLEEP_STAGE message.
const (
	SleepUnmeasurable = 0
	SleepAwake        = 1
	SleepLight        = 2
	SleepDeep         = 3
	SleepREM          = 4
)

// Generic metric ids, matching MetricSample.Metric upstream.
const (
	MetricEnduranceScore           = 1
	MetricFunctionalThresholdPower = 2
	MetricHillEndurance            = 3
	MetricHillScore                = 4
	MetricHillStrength             = 5
	MetricMaxVO2                   = 6
	MetricRunningLactateThreshold  = 7
	MetricTrainingReadiness        = 8
	MetricTrainingLoadAcute        = 9
	MetricTrainingLoadChronic      = 10
	MetricRestingMetabolicRate     = 11
)

// notWornThresholdSec matches THRESHOLD_NOT_WORN upstream: a gap longer than
// ten minutes means the watch was off the wrist.
const notWornThresholdSec = 600

// Importer writes decoded FIT data into the store.
type Importer struct {
	db  *store.DB
	log *slog.Logger
}

func New(db *store.DB, log *slog.Logger) *Importer {
	return &Importer{db: db, log: log}
}

// Result reports what one file contributed.
type Result struct {
	FileType    string `json:"fileType"`
	Records     int    `json:"records"`
	Activity    int    `json:"activitySamples"`
	Stress      int    `json:"stress"`
	BodyEnergy  int    `json:"bodyEnergy"`
	Spo2        int    `json:"spo2"`
	Respiration int    `json:"respiration"`
	SleepStages int    `json:"sleepStages"`
	HRV         int    `json:"hrv"`
	Workouts    int    `json:"workouts"`
	Metrics     int    `json:"metrics"`
}

// Import decodes data and persists everything it recognises. The caller passes
// the directory entry sub type so the importer knows which file it is looking
// at without re-reading file_id.
func (im *Importer) Import(deviceID int64, subType uint8, data []byte) (*Result, error) {
	f, err := fit.Decode(data)
	if err != nil && len(f.Records) == 0 {
		return nil, fmt.Errorf("importer: decode: %w", err)
	}
	if err != nil {
		// A truncated tail still yields usable records; keep what parsed.
		im.log.Warn("importer: partial decode", "err", err, "records", len(f.Records))
	}

	res := &Result{Records: len(f.Records)}
	res.FileType = fileTypeName(f, subType)

	// Battery and generic metrics are collected from every file type.
	im.importBattery(deviceID, f)
	res.Metrics += im.importMetrics(deviceID, f)

	switch res.FileType {
	case "MONITOR", "MONITOR_A", "MONITOR_DAILY":
		im.importMonitor(deviceID, f, res)
	case "SLEEP":
		im.importSleep(deviceID, f, res)
	case "HRV_STATUS":
		im.importHRV(deviceID, f, res)
	case "METRICS":
		// Already covered by importMetrics.
	case "ACTIVITY":
		im.importActivity(deviceID, f, res)
	default:
		// Files such as CHANGELOG carry no health data; battery and metrics
		// above are all we want from them.
		im.log.Debug("importer: no specific handler", "type", res.FileType)
	}
	return res, nil
}

func fileTypeName(f *fit.File, subType uint8) string {
	if ids := f.Of(fit.MsgFileID); len(ids) > 0 {
		if v, ok := ids[0].Int("type"); ok {
			subType = uint8(v)
		}
	}
	switch subType {
	case 4:
		return "ACTIVITY"
	case 15:
		return "MONITOR_A"
	case 28:
		return "MONITOR_DAILY"
	case 32:
		return "MONITOR"
	case 41:
		return "CHANGELOG"
	case 44:
		return "METRICS"
	case 49:
		return "SLEEP"
	case 68:
		return "HRV_STATUS"
	case 2:
		return "SETTINGS"
	case 9:
		return "WEIGHT"
	}
	return fmt.Sprintf("SUBTYPE_%d", subType)
}

// ------------------------------------------------------------- monitoring ---

type monitorBucket struct {
	tsSec          int64
	heartRate      int
	intensity      int
	activityKind   int
	stepsPerType   map[int]int
	distPerType    map[int]int
	caloriePerType map[int]int
	moderate       int
	vigorous       int
}

func (im *Importer) importMonitor(deviceID int64, f *fit.File, res *Result) {
	buckets := map[int64]*monitorBucket{}
	var lastMonitoringTs int64

	for _, r := range f.Of(fit.MsgMonitoring) {
		ts := monitoringTimestamp(&r, lastMonitoringTs)
		if ts == 0 {
			continue
		}
		lastMonitoringTs = ts

		b := buckets[ts]
		if b == nil {
			b = &monitorBucket{
				tsSec:          ts,
				heartRate:      KindNotMeasured,
				intensity:      KindNotMeasured,
				activityKind:   KindActivity,
				stepsPerType:   map[int]int{},
				distPerType:    map[int]int{},
				caloriePerType: map[int]int{},
			}
			buckets[ts] = b
		}

		activityType := KindNotMeasured
		if v, ok := r.Int("activity_type"); ok {
			activityType = int(v)
		}
		if v, ok := r.Int("current_activity_type_intensity"); ok {
			if activityType == KindNotMeasured {
				activityType = int(v) & 0x1F
			}
			b.intensity = (int(v) >> 5) & 0x07
		}
		if v, ok := r.Int("heart_rate"); ok {
			b.heartRate = int(v)
		}
		// Garmin reports cumulative counters per activity type; the last value
		// for a type wins and the sample is their sum.
		if v, ok := r.Int("cycles"); ok {
			b.stepsPerType[activityType] = int(v)
		}
		if v, ok := r.Int("distance"); ok {
			b.distPerType[activityType] = int(v)
		}
		if v, ok := r.Int("active_calories"); ok {
			b.caloriePerType[activityType] = int(v)
		}
		if v, ok := r.Int("moderate_activity_minutes"); ok {
			b.moderate += int(v)
		}
		if v, ok := r.Int("vigorous_activity_minutes"); ok {
			b.vigorous += int(v)
		}
	}

	keys := make([]int64, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	var samples []store.ActivitySample
	var intensity []store.IntensityMinutes
	var prevTs int64
	prevKind := KindNotMeasured

	for _, ts := range keys {
		b := buckets[ts]
		if prevTs != 0 && ts-prevTs > 60 {
			kind := prevKind
			if ts-prevTs > notWornThresholdSec {
				kind = KindNotWorn
			}
			for gap := prevTs + 60; gap < ts; gap += 60 {
				samples = append(samples, store.ActivitySample{
					TsMs:         gap * 1000,
					Steps:        0,
					HeartRate:    KindNotMeasured,
					RawIntensity: KindNotMeasured,
					RawKind:      kind,
				})
			}
		}

		s := store.ActivitySample{
			TsMs:         ts * 1000,
			HeartRate:    b.heartRate,
			RawIntensity: b.intensity,
			RawKind:      KindActivity,
		}
		for _, v := range b.stepsPerType {
			s.Steps += v
		}
		for _, v := range b.distPerType {
			s.DistanceCm += v
		}
		for _, v := range b.caloriePerType {
			s.ActiveCalories += v
		}
		if s.Steps == 0 && s.DistanceCm == 0 && s.ActiveCalories == 0 &&
			s.HeartRate <= 0 && s.RawIntensity < 0 {
			prevTs, prevKind = ts, KindActivity
			continue
		}
		samples = append(samples, s)
		if b.moderate != 0 || b.vigorous != 0 {
			intensity = append(intensity, store.IntensityMinutes{
				TsMs: ts * 1000, Moderate: b.moderate, Vigorous: b.vigorous,
			})
		}
		prevTs, prevKind = ts, KindActivity
	}

	if err := im.db.PutActivitySamples(deviceID, samples); err != nil {
		im.log.Error("importer: activity samples", "err", err)
	}
	if err := im.db.PutIntensityMinutes(deviceID, intensity); err != nil {
		im.log.Error("importer: intensity minutes", "err", err)
	}
	res.Activity = len(samples)

	res.Stress = im.putSeriesFrom(deviceID, f, fit.MsgStressLevel, "stress", func(r *fit.Record) (int64, float64, bool) {
		v, ok := r.Int("stress_level_value")
		if !ok || v < 0 {
			return 0, 0, false
		}
		ts := r.Timestamp
		if t, ok := r.Int("stress_level_time"); ok {
			ts = t
		}
		return ts, float64(v), true
	})
	res.BodyEnergy = im.putSeriesFrom(deviceID, f, fit.MsgStressLevel, "body_energy", func(r *fit.Record) (int64, float64, bool) {
		v, ok := r.Int("body_energy")
		if !ok || v < 0 {
			return 0, 0, false
		}
		ts := r.Timestamp
		if t, ok := r.Int("stress_level_time"); ok {
			ts = t
		}
		return ts, float64(v), true
	})
	res.Spo2 = im.putSeriesFrom(deviceID, f, fit.MsgSpo2, "spo2", func(r *fit.Record) (int64, float64, bool) {
		v, ok := r.Int("reading_spo2")
		if !ok || v <= 0 {
			return 0, 0, false
		}
		return r.Timestamp, float64(v), true
	})
	res.Respiration = im.putSeriesFrom(deviceID, f, fit.MsgRespirationRate, "respiration", func(r *fit.Record) (int64, float64, bool) {
		v, ok := r.Float("respiration_rate")
		if !ok || v <= 0 {
			return 0, 0, false
		}
		return r.Timestamp, v, true
	})
	im.putSeriesFrom(deviceID, f, fit.MsgMonitoringHRData, "resting_hr", func(r *fit.Record) (int64, float64, bool) {
		v, ok := r.Int("current_day_resting_heart_rate")
		if !ok {
			if v, ok = r.Int("resting_heart_rate"); !ok {
				return 0, 0, false
			}
		}
		if v <= 0 {
			return 0, 0, false
		}
		return r.Timestamp, float64(v), true
	})
	im.putSeriesFrom(deviceID, f, fit.MsgMonitoringInfo, "rmr", func(r *fit.Record) (int64, float64, bool) {
		v, ok := r.Int("resting_metabolic_rate")
		if !ok || v <= 0 {
			return 0, 0, false
		}
		return r.Timestamp, float64(v), true
	})
}

// monitoringTimestamp resolves the record time, expanding the 16 bit rolling
// timestamp against the last full one. Without this steps drift by hours.
func monitoringTimestamp(r *fit.Record, lastFull int64) int64 {
	t16, ok := r.Int("timestamp_16")
	if !ok {
		if r.Timestamp != 0 {
			return r.Timestamp
		}
		return lastFull
	}
	if lastFull == 0 {
		lastFull = r.Timestamp
	}
	if lastFull == 0 {
		return 0
	}
	refGarmin := lastFull - fit.GarminEpoch
	diff := (t16 & 0xFFFF) - (refGarmin & 0xFFFF)
	if diff < -32768 {
		diff += 65536
	}
	if diff > 32768 {
		diff -= 65536
	}
	return lastFull + diff
}

func (im *Importer) putSeriesFrom(deviceID int64, f *fit.File, msg uint16, series string,
	extract func(*fit.Record) (int64, float64, bool)) int {
	records := f.Of(msg)
	if len(records) == 0 {
		return 0
	}
	points := make([]store.Point, 0, len(records))
	for i := range records {
		ts, v, ok := extract(&records[i])
		if !ok || ts == 0 {
			continue
		}
		points = append(points, store.Point{TsMs: ts * 1000, Value: v})
	}
	if len(points) == 0 {
		return 0
	}
	if err := im.db.PutSeries(series, deviceID, points); err != nil {
		im.log.Error("importer: series", "series", series, "err", err)
		return 0
	}
	return len(points)
}

// ------------------------------------------------------------------ sleep ---

func (im *Importer) importSleep(deviceID int64, f *fit.File, res *Result) {
	var stages []store.SleepStage
	realStages := 0
	for _, r := range f.Of(fit.MsgSleepStage) {
		v, ok := r.Int("sleep_stage")
		if !ok || r.Timestamp == 0 {
			continue
		}
		if v != SleepUnmeasurable && v != SleepAwake {
			realStages++
		}
		stages = append(stages, store.SleepStage{TsMs: r.Timestamp * 1000, Stage: int(v)})
	}

	var events []store.SleepEvent
	for _, r := range f.Of(fit.MsgEvent) {
		ev, ok := r.Int("event")
		if !ok || ev != 74 {
			continue
		}
		et, _ := r.Int("event_type")
		data, _ := r.Int("data")
		events = append(events, store.SleepEvent{
			TsMs: r.Timestamp * 1000, Event: int(ev), EventType: int(et), Data: int(data),
		})
	}

	// A file with raw sleep blocks but no scored stages only tells us the
	// window; synthesise the asleep/awake pair the way upstream does.
	raw := f.Of(fit.MsgSleepDataRaw)
	if len(raw) > 0 && realStages == 0 {
		if ids := f.Of(fit.MsgFileID); len(ids) > 0 {
			if created, ok := ids[0].Int("time_created"); ok && created > 0 {
				asleep := created * 1000
				wake := asleep + int64(len(raw))*60000
				events = append(events,
					store.SleepEvent{TsMs: asleep, Event: 74, EventType: 0, Data: -1},
					store.SleepEvent{TsMs: wake, Event: 74, EventType: 1, Data: -1})
				stages = nil
			}
		}
	}

	if err := im.db.PutSleepStages(deviceID, stages); err != nil {
		im.log.Error("importer: sleep stages", "err", err)
	}
	if err := im.db.PutSleepEvents(deviceID, events); err != nil {
		im.log.Error("importer: sleep events", "err", err)
	}
	res.SleepStages = len(stages)

	im.putSeriesFrom(deviceID, f, fit.MsgSleepStats, "sleep_score", func(r *fit.Record) (int64, float64, bool) {
		v, ok := r.Int("overall_sleep_score")
		if !ok || v <= 0 || r.Timestamp == 0 {
			return 0, 0, false
		}
		return r.Timestamp, float64(v), true
	})
	im.putSeriesFrom(deviceID, f, fit.MsgSleepRestless, "restless", func(r *fit.Record) (int64, float64, bool) {
		v, ok := r.Int("restless_moments_count")
		if !ok || r.Timestamp == 0 {
			return 0, 0, false
		}
		return r.Timestamp, float64(v), true
	})

	var naps []store.Nap
	for _, r := range f.Of(fit.MsgDailySleep) {
		start, ok1 := r.Int("start_timestamp")
		end, ok2 := r.Int("end_timestamp")
		if !ok1 || !ok2 || end <= start {
			continue
		}
		if deleted, ok := r.Int("deleted"); ok && deleted == 1 {
			continue
		}
		naps = append(naps, store.Nap{StartMs: start * 1000, EndMs: end * 1000})
	}
	if err := im.db.PutNaps(deviceID, naps); err != nil {
		im.log.Error("importer: naps", "err", err)
	}
}

// -------------------------------------------------------------------- hrv ---

func (im *Importer) importHRV(deviceID int64, f *fit.File, res *Result) {
	var summaries []store.HRVSummary
	for _, r := range f.Of(fit.MsgHRVSummary) {
		if r.Timestamp == 0 {
			continue
		}
		s := store.HRVSummary{TsMs: r.Timestamp * 1000}
		s.WeeklyAverage = roundField(&r, "weekly_average")
		s.LastNightAverage = roundField(&r, "last_night_average")
		s.LastNight5MinHigh = roundField(&r, "last_night_5_min_high")
		s.BaselineLowUpper = roundField(&r, "baseline_low_upper")
		s.BaselineBalancedLower = roundField(&r, "baseline_balanced_lower")
		s.BaselineBalancedUpper = roundField(&r, "baseline_balanced_upper")
		if v, ok := r.Int("status"); ok {
			s.StatusNum = int(v)
		}
		summaries = append(summaries, s)
	}
	if err := im.db.PutHRVSummaries(deviceID, summaries); err != nil {
		im.log.Error("importer: hrv summaries", "err", err)
	}
	res.HRV = im.putSeriesFrom(deviceID, f, fit.MsgHRVValue, "hrv", func(r *fit.Record) (int64, float64, bool) {
		v, ok := r.Float("value")
		if !ok || v <= 0 || r.Timestamp == 0 {
			return 0, 0, false
		}
		return r.Timestamp, math.Round(v), true
	})
	res.HRV += len(summaries)
}

func roundField(r *fit.Record, name string) int {
	if v, ok := r.Float(name); ok {
		return int(math.Round(v))
	}
	return 0
}

// ---------------------------------------------------------------- battery ---

func (im *Importer) importBattery(deviceID int64, f *fit.File) {
	for _, r := range f.Of(fit.MsgDeviceStatus) {
		v, ok := r.Int("battery_level")
		if !ok || r.Timestamp == 0 {
			continue
		}
		if err := im.db.PutBattery(deviceID, r.Timestamp*1000, int(v)); err != nil {
			im.log.Error("importer: battery", "err", err)
		}
	}
}

// ---------------------------------------------------------------- metrics ---

// metricSpec maps a global message plus field onto a generic metric id.
type metricSpec struct {
	msg    uint16
	field  string
	metric int
}

var metricSpecs = []metricSpec{
	{378, "training_load_acute", MetricTrainingLoadAcute},
	{378, "training_load_chronic", MetricTrainingLoadChronic},
	{402, "hill_score", MetricHillScore},
	{402, "hill_strength", MetricHillStrength},
	{402, "hill_endurance", MetricHillEndurance},
	{369, "training_readiness", MetricTrainingReadiness},
	{403, "endurance_score", MetricEnduranceScore},
	{356, "functional_threshold_power", MetricFunctionalThresholdPower},
	{356, "running_lactate_threshold_power", MetricRunningLactateThreshold},
	{fit.MsgMaxMetData, "vo2_max", MetricMaxVO2},
	{fit.MsgMonitoringInfo, "resting_metabolic_rate", MetricRestingMetabolicRate},
}

func (im *Importer) importMetrics(deviceID int64, f *fit.File) int {
	var items []store.MetricSample
	for _, spec := range metricSpecs {
		for _, r := range f.Of(spec.msg) {
			v, ok := r.Float(spec.field)
			if !ok || v <= 0 {
				continue
			}
			ts := r.Timestamp
			if ts == 0 {
				if u, ok := r.Int("update_time"); ok {
					ts = u
				}
			}
			if ts == 0 {
				continue
			}
			var extra int64
			if lvl, ok := r.Int("level"); ok {
				extra = lvl
			}
			items = append(items, store.MetricSample{
				TsMs: ts * 1000, Type: spec.metric, Score: v, Extra: extra,
			})
		}
	}
	if len(items) == 0 {
		return 0
	}
	if err := im.db.PutMetrics(deviceID, items); err != nil {
		im.log.Error("importer: metrics", "err", err)
		return 0
	}
	return len(items)
}

// --------------------------------------------------------------- workouts ---

func (im *Importer) importActivity(deviceID int64, f *fit.File, res *Result) {
	sessions := f.Of(fit.MsgSession)
	if len(sessions) == 0 {
		return
	}
	s := sessions[0]

	start := s.Timestamp
	if v, ok := s.Int("start_time"); ok {
		start = v
	}
	if start == 0 {
		if ids := f.Of(fit.MsgFileID); len(ids) > 0 {
			start, _ = ids[0].Int("time_created")
		}
	}
	if start == 0 {
		return
	}

	elapsed := 0.0
	if v, ok := s.Float("total_elapsed_time"); ok {
		elapsed = v
	}
	end := start + int64(elapsed)
	if end <= start {
		end = s.Timestamp
	}

	sport, _ := s.Int("sport")
	subSport, _ := s.Int("sub_sport")
	name, _ := s.Str("sport_profile_name")
	if name == "" {
		if n, ok := fit.EnumName("Sport", int(sport)); ok {
			name = n
		} else {
			name = fmt.Sprintf("Sport %d", sport)
		}
	}

	summary := map[string]any{}
	for _, key := range []string{
		"total_distance", "total_calories", "total_timer_time", "total_elapsed_time",
		"avg_speed", "max_speed", "average_heart_rate", "max_heart_rate", "min_heart_rate",
		"avg_cadence", "max_cadence", "avg_power", "max_power", "total_ascent",
		"total_descent", "total_training_effect", "num_laps", "avg_temperature",
		"max_temperature", "total_moving_time", "avg_altitude", "max_altitude", "min_altitude",
	} {
		if v, ok := s.Fields[key]; ok {
			summary[key] = v
		}
	}
	raw, _ := json.Marshal(summary)

	w := &store.Workout{
		DeviceID:     deviceID,
		StartMs:      start * 1000,
		EndMs:        end * 1000,
		ActivityKind: KindActivity,
		Sport:        int(sport),
		SubSport:     int(subSport),
		Name:         name,
		Summary:      raw,
	}

	for _, r := range f.Of(fit.MsgRecord) {
		if r.Timestamp == 0 {
			continue
		}
		p := store.TrackPoint{TsMs: r.Timestamp * 1000}
		if v, ok := r.Float("latitude"); ok {
			p.Lat = &v
		}
		if v, ok := r.Float("longitude"); ok {
			p.Lon = &v
		}
		if v, ok := r.Float("enhanced_altitude"); ok {
			p.Altitude = &v
		} else if v, ok := r.Float("altitude"); ok {
			p.Altitude = &v
		}
		if v, ok := r.Int("heart_rate"); ok {
			hr := int(v)
			p.HeartRate = &hr
		}
		if v, ok := r.Int("cadence"); ok {
			c := int(v)
			p.Cadence = &c
		}
		if v, ok := r.Float("enhanced_speed"); ok {
			p.Speed = &v
		} else if v, ok := r.Float("speed"); ok {
			p.Speed = &v
		}
		if v, ok := r.Int("power"); ok {
			pw := int(v)
			p.Power = &pw
		}
		if v, ok := r.Float("distance"); ok {
			p.Distance = &v
		}
		w.Track = append(w.Track, p)
	}

	if err := im.db.PutWorkout(w); err != nil {
		im.log.Error("importer: workout", "err", err)
		return
	}
	res.Workouts = 1
}
