// Package api serves the local HTTP interface the QML front end talks to.
package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pulse/backend/internal/analytics"
	"pulse/backend/internal/daemon"
	"pulse/backend/internal/store"
)

// Version is reported by /api/status.
const Version = "0.1.0"

// NotificationFeed exposes recent notifications for the UI.
type NotificationFeed interface {
	RecentJSON(limit int) any
}

// NotificationSink accepts a notification observed by another process. The
// push service path is invisible to this daemon, so pulse-wdnotify hands over
// what it relayed instead.
type NotificationSink interface {
	InjectJSON(key, source, appID, appName, title, body string, removed bool) error
}

// Server wires the daemon and the database into HTTP handlers.
type Server struct {
	db    *store.DB
	mgr   *daemon.Manager
	log   *slog.Logger
	notes NotificationFeed
	sink  NotificationSink
}

func New(db *store.DB, mgr *daemon.Manager, log *slog.Logger, notes NotificationFeed, sink NotificationSink) *Server {
	return &Server{db: db, mgr: mgr, log: log, notes: notes, sink: sink}
}

// Handler builds the router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/devices", s.devices)
	mux.HandleFunc("POST /api/scan", s.startScan)
	mux.HandleFunc("GET /api/scan/results", s.scanResults)
	mux.HandleFunc("POST /api/pair", s.pair)
	mux.HandleFunc("GET /api/pairing", s.pairingState)
	mux.HandleFunc("POST /api/pairing", s.pairingSubmit)
	mux.HandleFunc("POST /api/connect", s.connect)
	mux.HandleFunc("POST /api/disconnect", s.disconnect)
	mux.HandleFunc("POST /api/forget", s.forget)
	mux.HandleFunc("POST /api/sync", s.sync)
	mux.HandleFunc("GET /api/today", s.today)
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/sleep", s.sleep)
	mux.HandleFunc("GET /api/workouts", s.workouts)
	mux.HandleFunc("GET /api/workouts/{id}", s.workout)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PUT /api/settings", s.putSettings)
	mux.HandleFunc("POST /api/findwatch", s.findWatch)
	mux.HandleFunc("POST /api/findwatch/cancel", s.cancelFindWatch)
	mux.HandleFunc("GET /api/notifications", s.notifications)
	mux.HandleFunc("POST /api/notifications", s.injectNotification)
	mux.HandleFunc("GET /api/events", s.events)
	return logging(s.log, mux)
}

func logging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Debug("api", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start))
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

// ----------------------------------------------------------------- status ---

type statusResponse struct {
	DaemonVersion  string               `json:"daemonVersion"`
	AdapterPowered bool                 `json:"adapterPowered"`
	Device         *daemon.DeviceStatus `json:"device"`
	Syncing        bool                 `json:"syncing"`
	Progress       daemon.Progress      `json:"progress"`
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	dev, syncing, progress := s.mgr.Status()
	writeJSON(w, http.StatusOK, statusResponse{
		DaemonVersion:  Version,
		AdapterPowered: s.mgr.AdapterPowered(),
		Device:         dev,
		Syncing:        syncing,
		Progress:       progress,
	})
}

type deviceEntry struct {
	Address    string `json:"address"`
	Name       string `json:"name"`
	Model      string `json:"model"`
	Paired     bool   `json:"paired"`
	Connected  bool   `json:"connected"`
	LastSyncMs int64  `json:"lastSyncMs"`
}

func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Devices()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	active, _, _ := s.mgr.Status()
	out := make([]deviceEntry, 0, len(rows))
	for _, d := range rows {
		e := deviceEntry{
			Address:    d.Address,
			Name:       d.Name,
			Model:      d.Model,
			Paired:     true,
			LastSyncMs: d.LastSyncMs,
		}
		if active != nil && active.Address == d.Address {
			e.Connected = active.Connected
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, out)
}

// -------------------------------------------------------------- discovery ---

func (s *Server) startScan(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DurationMs int `json:"durationMs"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	d := time.Duration(body.DurationMs) * time.Millisecond
	if d <= 0 {
		d = 20 * time.Second
	}
	s.mgr.StartScan(d)
	writeJSON(w, http.StatusAccepted, map[string]any{"scanning": true, "durationMs": d.Milliseconds()})
}

func (s *Server) scanResults(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.mgr.ScanResults())
}

type addressBody struct {
	Address string `json:"address"`
}

func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	var body addressBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Address == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("address required"))
		return
	}
	go func() {
		if err := s.mgr.Pair(body.Address); err != nil {
			s.log.Warn("api: pair failed", "err", err)
			s.mgr.Events().Publish("pair_failed", map[string]string{"error": err.Error()})
			return
		}
		s.mgr.Events().Publish("pair_complete", map[string]string{"address": body.Address})
		if err := s.mgr.Connect(body.Address); err != nil {
			s.log.Warn("api: connect after pair failed", "err", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"address": body.Address})
}

func (s *Server) pairingState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.mgr.Pairing())
}

func (s *Server) pairingSubmit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Passkey *uint32 `json:"passkey"`
		Confirm *bool   `json:"confirm"`
		Cancel  *bool   `json:"cancel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var err error
	switch {
	case body.Cancel != nil && *body.Cancel:
		err = s.mgr.CancelPairing()
	case body.Passkey != nil:
		err = s.mgr.SubmitPasskey(*body.Passkey)
	case body.Confirm != nil:
		err = s.mgr.ConfirmPairing(*body.Confirm)
	default:
		err = fmt.Errorf("passkey, confirm or cancel required")
	}
	if err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) connect(w http.ResponseWriter, r *http.Request) {
	var body addressBody
	json.NewDecoder(r.Body).Decode(&body)
	if body.Address == "" {
		rows, err := s.db.Devices()
		if err != nil || len(rows) == 0 {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("no known device"))
			return
		}
		body.Address = rows[0].Address
	}
	go func() {
		if err := s.mgr.Connect(body.Address); err != nil {
			s.log.Warn("api: connect failed", "err", err)
			s.mgr.Events().Publish("connect_failed", map[string]string{"error": err.Error()})
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"address": body.Address})
}

func (s *Server) disconnect(w http.ResponseWriter, r *http.Request) {
	s.mgr.Disconnect()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) forget(w http.ResponseWriter, r *http.Request) {
	var body addressBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Address == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("address required"))
		return
	}
	if err := s.mgr.Forget(body.Address); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) sync(w http.ResponseWriter, r *http.Request) {
	if err := s.mgr.Sync(); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

// ------------------------------------------------------------- dashboards ---

func (s *Server) engine() (*analytics.Engine, bool) {
	id, ok := s.mgr.ActiveDeviceID()
	if !ok {
		return nil, false
	}
	return analytics.New(s.db, id, s.mgr.AnalyticsSettings(), time.Local), true
}

func parseDate(r *http.Request) time.Time {
	v := r.URL.Query().Get("date")
	if v == "" {
		return time.Now()
	}
	t, err := time.ParseInLocation("2006-01-02", v, time.Local)
	if err != nil {
		return time.Now()
	}
	return t
}

func (s *Server) today(w http.ResponseWriter, r *http.Request) {
	e, ok := s.engine()
	if !ok {
		writeJSON(w, http.StatusOK, emptyToday(s.mgr))
		return
	}
	writeJSON(w, http.StatusOK, e.Today(parseDate(r)))
}

func emptyToday(mgr *daemon.Manager) analytics.Today {
	set := mgr.AnalyticsSettings()
	return analytics.Today{
		Date:      time.Now().Format("2006-01-02"),
		StepsGoal: set.StepsGoal,
		Goals: analytics.Goals{
			Steps:            set.StepsGoal,
			SleepMinutes:     set.SleepGoalMinutes,
			ActiveCalories:   set.ActiveCaloriesGoal,
			DistanceM:        set.DistanceGoalM,
			ActiveMinutes:    set.ActiveMinutesGoal,
			IntensityMinutes: set.IntensityGoal,
		},
		IntensityMinutes: analytics.Intensity{Goal: set.IntensityGoal},
		HeartRate:        analytics.MinMaxLatest{Latest: -1, Min: -1, Max: -1},
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	days := 7
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 90 {
			days = n
		}
	}
	e, ok := s.engine()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"metrics": []analytics.Metric{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"metrics": e.Health(days, time.Now())})
}

func (s *Server) sleep(w http.ResponseWriter, r *http.Request) {
	e, ok := s.engine()
	if !ok {
		writeJSON(w, http.StatusOK, analytics.SleepReport{Quality: "No data"})
		return
	}
	writeJSON(w, http.StatusOK, e.Sleep(parseDate(r)))
}

func (s *Server) workouts(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	rows, err := s.db.Workouts(limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if rows == nil {
		rows = []store.Workout{}
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) workout(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	row, err := s.db.Workout(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if row == nil {
		writeErr(w, http.StatusNotFound, fmt.Errorf("workout %d not found", id))
		return
	}
	writeJSON(w, http.StatusOK, row)
}

// ---------------------------------------------------------------- control ---

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.mgr.Settings())
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	current := s.mgr.Settings()
	if err := json.NewDecoder(r.Body).Decode(&current); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.mgr.SaveSettings(current); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, current)
}

func (s *Server) findWatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Seconds int `json:"seconds"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Seconds <= 0 {
		body.Seconds = 30
	}
	if err := s.mgr.FindWatch(body.Seconds); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) cancelFindWatch(w http.ResponseWriter, r *http.Request) {
	if err := s.mgr.CancelFindWatch(); err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) notifications(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if s.notes == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	writeJSON(w, http.StatusOK, s.notes.RecentJSON(limit))
}

// injectNotification accepts a notification another process already delivered
// to the phone, so the watch sees it too. pulse-wdnotify uses this for Android
// notifications it relays through the push service.
func (s *Server) injectNotification(w http.ResponseWriter, r *http.Request) {
	if s.sink == nil {
		writeErr(w, http.StatusServiceUnavailable, fmt.Errorf("no notification sink"))
		return
	}
	var body struct {
		Key     string `json:"key"`
		Source  string `json:"source"`
		AppID   string `json:"appId"`
		AppName string `json:"appName"`
		Title   string `json:"title"`
		Body    string `json:"body"`
		Removed bool   `json:"removed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if body.Title == "" && body.Body == "" && !body.Removed {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("empty notification"))
		return
	}
	if err := s.sink.InjectJSON(body.Key, body.Source, body.AppID, body.AppName,
		body.Title, body.Body, body.Removed); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

// ------------------------------------------------------------------- SSE ---

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, cancel := s.mgr.Events().Subscribe()
	defer cancel()

	// A periodic comment keeps intermediaries and the QML XHR reader awake.
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()

	enc := json.NewEncoder(&sseWriter{w: w})
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprint(w, "data: ")
			if err := enc.Encode(ev); err != nil {
				return
			}
			fmt.Fprint(w, "\n")
			flusher.Flush()
		}
	}
}

// sseWriter strips the newline json.Encoder appends so the SSE framing stays
// exactly "data: <json>\n\n".
type sseWriter struct{ w http.ResponseWriter }

func (s *sseWriter) Write(p []byte) (int, error) {
	n := len(p)
	trimmed := strings.TrimRight(string(p), "\n")
	if _, err := s.w.Write([]byte(trimmed)); err != nil {
		return 0, err
	}
	if _, err := s.w.Write([]byte("\n")); err != nil {
		return 0, err
	}
	return n, nil
}
