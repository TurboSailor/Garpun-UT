// Package analytics turns stored samples into the aggregates the UI shows.
//
// Garmin activity samples are cumulative: the value at a timestamp is the
// running total up to that minute, and the counter resets at the start of a
// day. Everything in this package therefore reads samples one minute ahead,
// shifts them back and differences them, exactly like the upstream sample
// provider does.
package analytics

import (
	"math"
	"sort"
	"time"

	"pulse/backend/internal/store"
)

// Settings are the user-tunable numbers the dashboard depends on.
type Settings struct {
	StepsGoal          int    `json:"stepsGoal"`
	SleepGoalMinutes   int    `json:"sleepGoalMinutes"`
	ActiveCaloriesGoal int    `json:"activeCaloriesGoal"`
	DistanceGoalM      int    `json:"distanceGoalM"`
	ActiveMinutesGoal  int    `json:"activeMinutesGoal"`
	IntensityGoal      int    `json:"intensityGoal"`
	Units              string `json:"units"`
	AnyGoalStreak      bool   `json:"anyGoalStreak"`
}

// DefaultSettings mirrors the Pulse presets.
func DefaultSettings() Settings {
	return Settings{
		StepsGoal:          10000,
		SleepGoalMinutes:   480,
		ActiveCaloriesGoal: 350,
		DistanceGoalM:      5000,
		ActiveMinutesGoal:  60,
		IntensityGoal:      30,
		Units:              "metric",
	}
}

// Sleep stage codes as stored (FIT sleep_stage).
const (
	stageUnmeasurable = 0
	stageAwake        = 1
	stageLight        = 2
	stageDeep         = 3
	stageREM          = 4
)

// Engine answers dashboard queries for one device.
type Engine struct {
	db       *store.DB
	deviceID int64
	loc      *time.Location
	settings Settings
}

func New(db *store.DB, deviceID int64, settings Settings, loc *time.Location) *Engine {
	if loc == nil {
		loc = time.Local
	}
	return &Engine{db: db, deviceID: deviceID, loc: loc, settings: settings}
}

// DayWindow returns [start, end) in Unix milliseconds for a local calendar day.
func (e *Engine) DayWindow(day time.Time) (int64, int64) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, e.loc)
	return start.UnixMilli(), start.AddDate(0, 0, 1).UnixMilli()
}

// deltaSamples reads cumulative samples for a window and returns per-minute
// deltas with timestamps shifted back by one minute.
func (e *Engine) deltaSamples(fromMs, toMs int64) []store.ActivitySample {
	const minute = int64(60_000)
	raw, err := e.db.ActivitySamples(e.deviceID, fromMs+minute, toMs+minute)
	if err != nil || len(raw) == 0 {
		return nil
	}

	// Upstream (AbstractSampleProvider.convertCumulativeSteps) keeps the first
	// sample of the range at its full value and only rebases it when an
	// earlier sample exists right before the window. The counter resets at
	// midnight, so the day's first reading already is the total so far and a
	// baseline from yesterday must never be borrowed.
	var prev *store.ActivitySample
	if before, err := e.db.ActivitySamples(e.deviceID, fromMs, fromMs+minute); err == nil {
		for i := len(before) - 1; i >= 0; i-- {
			if before[i].Steps > 0 || before[i].DistanceCm > 0 || before[i].ActiveCalories > 0 {
				s := before[i]
				prev = &s
				break
			}
		}
	}

	out := make([]store.ActivitySample, 0, len(raw))
	for _, s := range raw {
		d := s
		d.TsMs = s.TsMs - minute
		if prev != nil {
			d.Steps = counterDelta(prev.Steps, s.Steps)
			d.DistanceCm = counterDelta(prev.DistanceCm, s.DistanceCm)
			d.ActiveCalories = counterDelta(prev.ActiveCalories, s.ActiveCalories)
		}
		cur := s
		if s.Steps == 0 && prev != nil && prev.Steps > 0 {
			cur.Steps = prev.Steps
		}
		if s.DistanceCm == 0 && prev != nil && prev.DistanceCm > 0 {
			cur.DistanceCm = prev.DistanceCm
		}
		if s.ActiveCalories == 0 && prev != nil && prev.ActiveCalories > 0 {
			cur.ActiveCalories = prev.ActiveCalories
		}
		prev = &cur
		out = append(out, d)
	}
	return out
}

// counterDelta handles the daily reset: a drop means the counter restarted.
func counterDelta(prev, cur int) int {
	if cur >= prev {
		return cur - prev
	}
	// If the counter dropped to 0 (unrecorded/unworn), the delta is 0, not a reset.
	if cur == 0 {
		return 0
	}
	return cur
}

// Totals is the movement summary of one day.
type Totals struct {
	Steps          int
	DistanceCm     int
	ActiveCalories int // raw units, 1/1000 kcal after the upstream conversion
	ActiveMinutes  int
	HeartRateMin   int
	HeartRateMax   int
	HeartRateLast  int
}

// DayTotals aggregates a local day.
func (e *Engine) DayTotals(day time.Time) Totals {
	from, to := e.DayWindow(day)
	samples := e.deltaSamples(from, to)

	t := Totals{HeartRateMin: -1, HeartRateMax: -1, HeartRateLast: -1}
	var activeMinutes int
	for _, s := range samples {
		t.Steps += s.Steps
		t.DistanceCm += s.DistanceCm
		t.ActiveCalories += s.ActiveCalories
		if s.HeartRate > 0 {
			if t.HeartRateMin < 0 || s.HeartRate < t.HeartRateMin {
				t.HeartRateMin = s.HeartRate
			}
			if s.HeartRate > t.HeartRateMax {
				t.HeartRateMax = s.HeartRate
			}
			t.HeartRateLast = s.HeartRate
		}
		// A minute counts as active when it carries movement, which matches
		// the step-session heuristic closely enough at minute resolution.
		if s.Steps > 0 {
			activeMinutes++
		}
	}
	t.ActiveMinutes = activeMinutes
	return t
}

// Today is the payload of GET /api/today.
type Today struct {
	Date             string       `json:"date"`
	Steps            int          `json:"steps"`
	StepsGoal        int          `json:"stepsGoal"`
	DistanceM        float64      `json:"distanceM"`
	ActiveCalories   int          `json:"activeCalories"`
	RestingCalories  int          `json:"restingCalories"`
	TotalCalories    int          `json:"totalCalories"`
	ActiveMinutes    int          `json:"activeMinutes"`
	HeartRate        MinMaxLatest `json:"heartRate"`
	BodyEnergy       MinMaxLatest `json:"bodyEnergy"`
	Stress           AvgLatest    `json:"stress"`
	Spo2             Latest       `json:"spo2"`
	Respiration      LatestFloat  `json:"respiration"`
	SleepMinutes     int          `json:"sleepMinutes"`
	SleepScore       int          `json:"sleepScore"`
	IntensityMinutes Intensity    `json:"intensityMinutes"`
	Streak           Streak       `json:"streak"`
	Goals            Goals        `json:"goals"`
}

type MinMaxLatest struct {
	Latest  int `json:"latest"`
	Resting int `json:"resting,omitempty"`
	Min     int `json:"min"`
	Max     int `json:"max"`
}

type AvgLatest struct {
	Latest int `json:"latest"`
	Avg    int `json:"avg"`
}

type Latest struct {
	Latest int `json:"latest"`
}

type LatestFloat struct {
	Latest float64 `json:"latest"`
}

type Intensity struct {
	Today int `json:"today"`
	Week  int `json:"week"`
	Goal  int `json:"goal"`
}

type Streak struct {
	Current int `json:"current"`
	Best    int `json:"best"`
}

type Goals struct {
	Steps            int `json:"steps"`
	SleepMinutes     int `json:"sleepMinutes"`
	ActiveCalories   int `json:"activeCalories"`
	DistanceM        int `json:"distanceM"`
	ActiveMinutes    int `json:"activeMinutes"`
	IntensityMinutes int `json:"intensityMinutes"`
}

// Today builds the dashboard payload for a local day.
func (e *Engine) Today(day time.Time) Today {
	from, to := e.DayWindow(day)
	totals := e.DayTotals(day)

	t := Today{
		Date:      day.In(e.loc).Format("2006-01-02"),
		Steps:     totals.Steps,
		StepsGoal: e.settings.StepsGoal,
		DistanceM: float64(totals.DistanceCm) / 100.0,
		// Stored active calories are kcal; upstream multiplies by 1000 on read
		// and divides again for display, so keep kcal here.
		ActiveCalories: totals.ActiveCalories,
		ActiveMinutes:  totals.ActiveMinutes,
		HeartRate: MinMaxLatest{
			Latest: totals.HeartRateLast,
			Min:    totals.HeartRateMin,
			Max:    totals.HeartRateMax,
		},
		Goals: Goals{
			Steps:            e.settings.StepsGoal,
			SleepMinutes:     e.settings.SleepGoalMinutes,
			ActiveCalories:   e.settings.ActiveCaloriesGoal,
			DistanceM:        e.settings.DistanceGoalM,
			ActiveMinutes:    e.settings.ActiveMinutesGoal,
			IntensityMinutes: e.settings.IntensityGoal,
		},
	}

	// Steps-based distance fallback keeps the tile useful on watches that do
	// not report distance in monitoring files.
	if t.DistanceM == 0 && totals.Steps > 0 {
		t.DistanceM = float64(totals.Steps) * 0.75
	}

	if p, ok := e.db.LatestSeries("resting_hr", e.deviceID, to); ok {
		t.HeartRate.Resting = int(p.Value)
	}

	if pts, err := e.db.Series("body_energy", e.deviceID, from, to); err == nil && len(pts) > 0 {
		t.BodyEnergy.Latest = int(pts[len(pts)-1].Value)
		t.BodyEnergy.Min, t.BodyEnergy.Max = int(pts[0].Value), int(pts[0].Value)
		for _, p := range pts {
			if int(p.Value) < t.BodyEnergy.Min {
				t.BodyEnergy.Min = int(p.Value)
			}
			if int(p.Value) > t.BodyEnergy.Max {
				t.BodyEnergy.Max = int(p.Value)
			}
		}
	}
	if pts, err := e.db.Series("stress", e.deviceID, from, to); err == nil && len(pts) > 0 {
		sum := 0.0
		for _, p := range pts {
			sum += p.Value
		}
		t.Stress.Latest = int(pts[len(pts)-1].Value)
		t.Stress.Avg = int(math.Round(sum / float64(len(pts))))
	}
	if pts, err := e.db.Series("spo2", e.deviceID, from, to); err == nil && len(pts) > 0 {
		t.Spo2.Latest = int(pts[len(pts)-1].Value)
	}
	if pts, err := e.db.Series("respiration", e.deviceID, from, to); err == nil && len(pts) > 0 {
		t.Respiration.Latest = pts[len(pts)-1].Value
	}

	// Resting calories: RMR scaled by the fraction of the day elapsed.
	if p, ok := e.db.LatestSeries("rmr", e.deviceID, to); ok && p.Value > 0 {
		fraction := 1.0
		nowMs := time.Now().UnixMilli()
		if nowMs < to {
			fraction = float64(nowMs-from) / float64(to-from)
			if fraction < 0 {
				fraction = 0
			}
		}
		t.RestingCalories = int(math.Round(p.Value * fraction))
	}
	t.TotalCalories = t.ActiveCalories + t.RestingCalories

	sleep := e.Sleep(day)
	t.SleepMinutes = sleep.AsleepMinutes
	t.SleepScore = sleep.Score

	t.IntensityMinutes = e.intensity(day)
	t.Streak = e.streak(day)
	return t
}

func (e *Engine) intensity(day time.Time) Intensity {
	out := Intensity{Goal: e.settings.IntensityGoal}
	from, to := e.DayWindow(day)
	if items, err := e.db.IntensityMinutes(e.deviceID, from, to); err == nil {
		for _, i := range items {
			out.Today += i.Moderate + 2*i.Vigorous
		}
	}

	local := day.In(e.loc)
	weekday := (int(local.Weekday()) + 6) % 7 // Monday = 0
	weekStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, e.loc).
		AddDate(0, 0, -weekday)
	if items, err := e.db.IntensityMinutes(e.deviceID, weekStart.UnixMilli(), to); err == nil {
		for _, i := range items {
			out.Week += i.Moderate + 2*i.Vigorous
		}
	}
	return out
}

// streak counts consecutive days meeting the step goal, walking backwards from
// the given day. With AnyGoalStreak any of the tracked goals counts.
func (e *Engine) streak(day time.Time) Streak {
	const maxLookback = 372
	var s Streak
	current := 0
	best := 0
	counting := true
	for i := range maxLookback {
		d := day.AddDate(0, 0, -i)
		t := e.DayTotals(d)
		met := t.Steps >= e.settings.StepsGoal
		if e.settings.AnyGoalStreak && !met {
			met = t.ActiveCalories >= e.settings.ActiveCaloriesGoal ||
				t.DistanceCm/100 >= e.settings.DistanceGoalM ||
				t.ActiveMinutes >= e.settings.ActiveMinutesGoal
		}
		if met {
			current++
			if current > best {
				best = current
			}
			continue
		}
		// Today not being finished yet must not break the streak.
		if i == 0 {
			continue
		}
		if counting {
			s.Current = current
			counting = false
		}
		current = 0
	}
	if counting {
		s.Current = current
	}
	s.Best = best
	return s
}

// ------------------------------------------------------------------ sleep ---

// SleepStageSpan is one contiguous stage interval.
type SleepStageSpan struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Stage   string `json:"stage"`
}

// SleepTotals holds per-stage minutes.
type SleepTotals struct {
	Deep  int `json:"deep"`
	Light int `json:"light"`
	REM   int `json:"rem"`
	Awake int `json:"awake"`
}

// NapSpan is a daytime sleep session.
type NapSpan struct {
	StartMs int64 `json:"startMs"`
	EndMs   int64 `json:"endMs"`
	Minutes int   `json:"minutes"`
}

// TrendPoint is one night of the seven night trend.
type TrendPoint struct {
	Date    string `json:"date"`
	Minutes int    `json:"minutes"`
	Score   int    `json:"score"`
}

// SleepReport is the payload of GET /api/sleep.
type SleepReport struct {
	Score   int         `json:"score"`
	Quality string      `json:"quality"`
	StartMs int64       `json:"startMs"`
	EndMs   int64       `json:"endMs"`
	Totals  SleepTotals `json:"totals"`
	// AsleepMinutes is the night length excluding awake time. With stage data
	// it equals deep+light+rem; without it, it comes from the watch's own
	// nightly summary, which is all some devices provide.
	AsleepMinutes int `json:"asleepMinutes"`
	// HasStages tells the UI whether a hypnogram and a stage breakdown exist.
	HasStages        bool             `json:"hasStages"`
	StartBodyBattery int              `json:"startBodyBattery"`
	EndBodyBattery   int              `json:"endBodyBattery"`
	Stages           []SleepStageSpan `json:"stages"`
	Naps             []NapSpan        `json:"naps"`
	Trend            []TrendPoint     `json:"trend"`
	RestlessMoments  int              `json:"restlessMoments"`
}

// sleepWindow is the 24 hours ending at noon of the selected day, which is how
// Pulse attributes a night to the morning you woke up.
func (e *Engine) sleepWindow(day time.Time) (int64, int64) {
	local := day.In(e.loc)
	noon := time.Date(local.Year(), local.Month(), local.Day(), 12, 0, 0, 0, e.loc)
	return noon.AddDate(0, 0, -1).UnixMilli(), noon.UnixMilli()
}

// Sleep builds the night report for a day.
func (e *Engine) Sleep(day time.Time) SleepReport {
	from, to := e.sleepWindow(day)
	rep := SleepReport{Totals: SleepTotals{}}

	samples, err := e.db.SleepStages(e.deviceID, from, to)
	if err != nil || len(samples) == 0 {
		return e.sleepFromSummary(day, from, to)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].TsMs < samples[j].TsMs })

	// Garmin stage samples are upper-bound timestamps: a sample marks the end
	// of the interval that started at the previous sample.
	var spans []SleepStageSpan
	prevTs := samples[0].TsMs
	for _, s := range samples[1:] {
		if s.TsMs <= prevTs {
			continue
		}
		name := stageName(s.Stage)
		if name != "" {
			spans = append(spans, SleepStageSpan{StartMs: prevTs, EndMs: s.TsMs, Stage: name})
		}
		prevTs = s.TsMs
	}

	// Split into sessions: a gap over an hour starts a new night, and only the
	// longest session is reported as the main sleep.
	sessions := splitSessions(spans, 60*60*1000)
	if len(sessions) == 0 {
		return e.sleepFromSummary(day, from, to)
	}
	main := 0
	bestAsleep := -1
	for i, s := range sessions {
		if a := asleepMinutes(s); a > bestAsleep {
			bestAsleep, main = a, i
		}
	}

	rep.Stages = sessions[main]
	rep.StartMs = rep.Stages[0].StartMs
	rep.EndMs = rep.Stages[len(rep.Stages)-1].EndMs
	rep.Totals = stageTotals(rep.Stages)
	rep.HasStages = true
	rep.AsleepMinutes = rep.Totals.Deep + rep.Totals.Light + rep.Totals.REM
	if sess, err := e.db.SleepSessions(e.deviceID, from, to); err == nil && len(sess) > 0 {
		s := sess[len(sess)-1]
		rep.StartBodyBattery, rep.EndBodyBattery = s.StartBodyBattery, s.EndBodyBattery
	}

	for i, s := range sessions {
		if i == main {
			continue
		}
		if m := asleepMinutes(s); m >= 10 {
			rep.Naps = append(rep.Naps, NapSpan{
				StartMs: s[0].StartMs, EndMs: s[len(s)-1].EndMs, Minutes: m,
			})
		}
	}
	if naps, err := e.db.Naps(e.deviceID, from, to); err == nil {
		for _, n := range naps {
			rep.Naps = append(rep.Naps, NapSpan{
				StartMs: n.StartMs, EndMs: n.EndMs, Minutes: int((n.EndMs - n.StartMs) / 60000),
			})
		}
	}

	if p, ok := e.db.LatestSeries("sleep_score", e.deviceID, to); ok && p.TsMs >= from {
		rep.Score = int(p.Value)
	} else {
		rep.Score = fallbackSleepScore(rep.Totals, e.settings.SleepGoalMinutes)
	}
	rep.Quality = qualityLabel(rep.Score)

	if pts, err := e.db.Series("restless", e.deviceID, from, to); err == nil {
		for _, p := range pts {
			rep.RestlessMoments += int(p.Value)
		}
	}
	rep.Trend = e.sleepTrend(day)
	return rep
}

// sleepFromSummary builds the report when no per-stage data exists. The watch
// still reports the night itself (FIT DAILY_SLEEP): window, awake time and its
// own score. Without stages there is no hypnogram, and HasStages stays false so
// the UI can say so instead of drawing an empty graph.
func (e *Engine) sleepFromSummary(day time.Time, from, to int64) SleepReport {
	rep := SleepReport{Trend: e.sleepTrend(day)}

	sessions, err := e.db.SleepSessions(e.deviceID, from, to)
	if err != nil || len(sessions) == 0 {
		rep.Quality = qualityLabel(0)
		return rep
	}
	// Several summaries can land in one window when the watch revises a night;
	// the last one wins, and shorter ones are daytime naps.
	main := 0
	for i, s := range sessions {
		if s.EndMs-s.StartMs > sessions[main].EndMs-sessions[main].StartMs {
			main = i
		}
	}
	s := sessions[main]
	rep.StartMs, rep.EndMs = s.StartMs, s.EndMs
	rep.StartBodyBattery, rep.EndBodyBattery = s.StartBodyBattery, s.EndBodyBattery
	awake := int(s.AwakeMs / 60000)
	total := int((s.EndMs - s.StartMs) / 60000)
	if awake > total {
		awake = total
	}
	rep.AsleepMinutes = total - awake
	rep.Totals.Awake = awake

	for i, n := range sessions {
		if i == main {
			continue
		}
		if m := int((n.EndMs - n.StartMs) / 60000); m >= 10 {
			rep.Naps = append(rep.Naps, NapSpan{StartMs: n.StartMs, EndMs: n.EndMs, Minutes: m})
		}
	}

	rep.Score = s.Score
	if rep.Score == 0 {
		if p, ok := e.db.LatestSeries("sleep_score", e.deviceID, to); ok && p.TsMs >= from {
			rep.Score = int(p.Value)
		}
	}
	rep.Quality = qualityLabel(rep.Score)

	if pts, err := e.db.Series("restless", e.deviceID, from, to); err == nil {
		for _, p := range pts {
			rep.RestlessMoments += int(p.Value)
		}
	}
	return rep
}

func splitSessions(spans []SleepStageSpan, maxGapMs int64) [][]SleepStageSpan {
	var out [][]SleepStageSpan
	var cur []SleepStageSpan
	var lastEnd int64
	for _, s := range spans {
		if len(cur) > 0 && s.StartMs-lastEnd > maxGapMs {
			out = append(out, cur)
			cur = nil
		}
		cur = append(cur, s)
		lastEnd = s.EndMs
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func stageTotals(spans []SleepStageSpan) SleepTotals {
	var t SleepTotals
	for _, s := range spans {
		m := int((s.EndMs - s.StartMs) / 60000)
		switch s.Stage {
		case "deep":
			t.Deep += m
		case "light":
			t.Light += m
		case "rem":
			t.REM += m
		case "awake":
			t.Awake += m
		}
	}
	return t
}

func asleepMinutes(spans []SleepStageSpan) int {
	t := stageTotals(spans)
	return t.Deep + t.Light + t.REM
}

func stageName(stage int) string {
	switch stage {
	case stageAwake:
		return "awake"
	case stageLight:
		return "light"
	case stageDeep:
		return "deep"
	case stageREM:
		return "rem"
	}
	return ""
}

// fallbackSleepScore reproduces the Pulse formula used when the watch does not
// supply a score: 70% duration against goal plus 30% deep+rem share.
func fallbackSleepScore(t SleepTotals, goalMinutes int) int {
	asleep := t.Deep + t.Light + t.REM
	if asleep == 0 || goalMinutes <= 0 {
		return 0
	}
	total := asleep + t.Awake
	s := 70.0 * float64(asleep) / float64(goalMinutes)
	if total > 0 {
		s += 30.0 * float64(t.Deep+t.REM) / float64(total)
	}
	score := int(math.Round(s))
	if score < 1 {
		score = 1
	}
	if score > 100 {
		score = 100
	}
	return score
}

func qualityLabel(score int) string {
	switch {
	case score >= 85:
		return "Excellent"
	case score >= 70:
		return "Good"
	case score >= 50:
		return "Fair"
	case score > 0:
		return "Poor"
	}
	return "No data"
}

func (e *Engine) sleepTrend(day time.Time) []TrendPoint {
	out := make([]TrendPoint, 0, 7)
	for i := 6; i >= 0; i-- {
		d := day.AddDate(0, 0, -i)
		from, to := e.sleepWindow(d)
		samples, err := e.db.SleepStages(e.deviceID, from, to)
		p := TrendPoint{Date: d.In(e.loc).Format("2006-01-02")}
		if err == nil && len(samples) > 1 {
			var spans []SleepStageSpan
			prev := samples[0].TsMs
			for _, s := range samples[1:] {
				if s.TsMs > prev {
					if name := stageName(s.Stage); name != "" {
						spans = append(spans, SleepStageSpan{StartMs: prev, EndMs: s.TsMs, Stage: name})
					}
					prev = s.TsMs
				}
			}
			p.Minutes = asleepMinutes(spans)
		}
		if p.Minutes == 0 {
			// No stages for that night: fall back to the watch's own summary,
			// counted exactly as sleepFromSummary does.
			if sess, err := e.db.SleepSessions(e.deviceID, from, to); err == nil {
				for _, s := range sess {
					m := int((s.EndMs-s.StartMs)/60000) - int(s.AwakeMs/60000)
					if m > p.Minutes {
						p.Minutes = m
					}
				}
			}
		}
		if sp, ok := e.db.LatestSeries("sleep_score", e.deviceID, to); ok && sp.TsMs >= from {
			p.Score = int(sp.Value)
		}
		out = append(out, p)
	}
	return out
}

// ----------------------------------------------------------------- health ---

// Metric is one card of the health screen.
type Metric struct {
	Key    string        `json:"key"`
	Label  string        `json:"label"`
	Unit   string        `json:"unit"`
	Latest float64       `json:"latest"`
	Delta  float64       `json:"delta"`
	Series []store.Point `json:"series"`
}

type metricSpec struct {
	key    string
	series string
	label  string
	unit   string
}

var healthMetrics = []metricSpec{
	{"heart_rate", "", "Heart rate", "bpm"},
	{"body_energy", "body_energy", "Body battery", "%"},
	{"stress", "stress", "Stress", ""},
	{"spo2", "spo2", "Blood oxygen", "%"},
	{"hrv", "hrv", "HRV", "ms"},
	{"respiration", "respiration", "Respiration", "br/min"},
	{"resting_hr", "resting_hr", "Resting HR", "bpm"},
	{"steps", "", "Steps", ""},
	{"intensity", "", "Intensity minutes", "min"},
	{"sleep", "", "Sleep", "min"},
}

// Health returns daily aggregates for the last n days.
func (e *Engine) Health(days int, ref time.Time) []Metric {
	if days <= 0 {
		days = 7
	}
	out := make([]Metric, 0, len(healthMetrics))
	for _, spec := range healthMetrics {
		m := Metric{Key: spec.key, Label: spec.label, Unit: spec.unit}
		for i := days - 1; i >= 0; i-- {
			d := ref.AddDate(0, 0, -i)
			from, to := e.DayWindow(d)
			var value float64
			switch spec.key {
			case "heart_rate":
				t := e.DayTotals(d)
				if t.HeartRateMax > 0 {
					value = float64(t.HeartRateMin+t.HeartRateMax) / 2
				}
			case "steps":
				value = float64(e.DayTotals(d).Steps)
			case "intensity":
				if items, err := e.db.IntensityMinutes(e.deviceID, from, to); err == nil {
					for _, it := range items {
						value += float64(it.Moderate + 2*it.Vigorous)
					}
				}
			case "sleep":
				s := e.Sleep(d)
				value = float64(s.Totals.Deep + s.Totals.Light + s.Totals.REM)
			default:
				if pts, err := e.db.Series(spec.series, e.deviceID, from, to); err == nil && len(pts) > 0 {
					sum := 0.0
					for _, p := range pts {
						sum += p.Value
					}
					value = sum / float64(len(pts))
				}
			}
			m.Series = append(m.Series, store.Point{TsMs: from, Value: math.Round(value*10) / 10})
		}
		if n := len(m.Series); n > 0 {
			m.Latest = m.Series[n-1].Value
			if n > 1 {
				m.Delta = m.Latest - m.Series[n-2].Value
			}
		}
		out = append(out, m)
	}
	return out
}
