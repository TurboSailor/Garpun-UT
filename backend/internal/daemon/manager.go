// Package daemon owns the runtime: the Bluetooth adapter, the watch session,
// the sync loop and everything the HTTP API exposes.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"pulse/backend/internal/analytics"
	"pulse/backend/internal/ble"
	"pulse/backend/internal/fit"
	"pulse/backend/internal/garmin"
	"pulse/backend/internal/gfdi"
	"pulse/backend/internal/importer"
	"pulse/backend/internal/store"
)

// Deps are the optional subsystems the daemon drives. Every field may be nil;
// the daemon degrades to whatever is available.
type Deps struct {
	// Weather returns an encoded FIT payload for the coordinates the watch
	// asked about.
	Weather func(ctx context.Context, lat, lon float64, hours int) ([]byte, error)
	// Protobuf handles inbound Smart messages.
	Protobuf func(requestID uint16, payload []byte) ([]byte, error)
	// SetProtobufSender hands the handler a way to answer asynchronously.
	SetProtobufSender func(func(requestID uint16, payload []byte))
	// NotificationContent resolves a notification the watch asks about.
	NotificationContent func(id int32) *garmin.NotificationContent
	// AppName resolves an application id to a display name.
	AppName func(appID string) string
	// Notifications streams phone notifications to forward to the watch.
	Notifications <-chan NotifyEvent
	// MusicMetadata supplies now-playing information.
	MusicMetadata func() []gfdi.MusicEntityValue
	// MusicCommand executes a watch media command.
	MusicCommand func(cmd uint8) error
	// AcceptCall and RejectCall answer watch call actions.
	AcceptCall func() error
	RejectCall func() error
}

// NotifyEvent is one phone notification, or its removal.
type NotifyEvent struct {
	Content garmin.NotificationContent
	Removed bool
	// Source is where the notification originated: "freedesktop" for the
	// phone's own, "waydroid" for an Android one relayed into the shade.
	// It is what the notifyWaydroid setting filters on.
	Source string
}

// Progress describes an in-flight sync.
type Progress struct {
	FileIndex int `json:"fileIndex"`
	Received  int `json:"received"`
	Total     int `json:"total"`
	Remaining int `json:"remaining"`
}

// DeviceStatus is the connected watch as the API reports it.
type DeviceStatus struct {
	Address      string `json:"address"`
	Name         string `json:"name"`
	Model        string `json:"model"`
	Firmware     string `json:"firmware"`
	Connected    bool   `json:"connected"`
	Initialized  bool   `json:"initialized"`
	BatteryLevel int    `json:"batteryLevel"`
	LastSyncMs   int64  `json:"lastSyncMs"`
}

// ScanResult is one discovered device.
type ScanResult struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	RSSI    int16  `json:"rssi"`
	Paired  bool   `json:"paired"`
	Garmin  bool   `json:"garmin"`
}

// Manager is the daemon core.
type Manager struct {
	db   *store.DB
	log  *slog.Logger
	deps Deps
	imp  *importer.Importer

	bus     *ble.Bus
	adapter *ble.Adapter
	agent   *ble.Agent
	pairing *pairingUI

	events *EventHub

	mu            sync.Mutex
	session       *garmin.Session
	dev           *ble.Device
	deviceRow     *store.Device
	battery       int
	syncing       bool
	progress      Progress
	scanResults   map[string]ScanResult
	scanning      bool
	autoReconnect bool
	lastWeather   time.Time

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New brings up the Bluetooth stack and the pairing agent.
func New(ctx context.Context, db *store.DB, log *slog.Logger, deps Deps) (*Manager, error) {
	bus, err := ble.Dial()
	if err != nil {
		return nil, err
	}
	adapter, err := bus.DefaultAdapter()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	m := &Manager{
		db:          db,
		log:         log,
		deps:        deps,
		imp:         importer.New(db, log),
		bus:         bus,
		adapter:     adapter,
		events:      NewEventHub(),
		scanResults: map[string]ScanResult{},
		battery:     -1,
		ctx:         ctx,
		cancel:      cancel,
	}

	m.pairing = &pairingUI{onState: func(s PairingState) {
		m.events.Publish("pairing_request", s)
	}}
	agent, err := ble.NewAgent(bus, m.pairing)
	if err != nil {
		cancel()
		return nil, err
	}
	// KeyboardDisplay covers both the passkey-entry and numeric-comparison
	// flows Garmin watches use.
	if err := agent.Register("KeyboardDisplay"); err != nil {
		log.Warn("daemon: pairing agent not registered", "err", err)
	}
	m.agent = agent

	powerCtx, powerCancel := context.WithTimeout(ctx, 10*time.Second)
	if err := adapter.SetPowered(powerCtx, true); err != nil {
		log.Warn("daemon: could not power on the adapter", "err", err)
	}
	powerCancel()

	m.wg.Add(1)
	go m.notificationLoop()
	m.wg.Add(1)
	go m.autoSyncLoop()
	m.wg.Add(1)
	go m.reconnectLoop()

	return m, nil
}

// Events exposes the SSE hub.
func (m *Manager) Events() *EventHub { return m.events }

// Close disconnects and releases the adapter.
func (m *Manager) Close() {
	m.cancel()
	m.mu.Lock()
	sess := m.session
	m.session = nil
	m.mu.Unlock()
	if sess != nil {
		sess.Close()
	}
	if m.agent != nil {
		m.agent.Unregister()
	}
	m.wg.Wait()
}

// ---------------------------------------------------------------- scanning ---

var garminHints = []string{
	"garmin", "forerunner", "fenix", "instinct", "venu", "vivo", "vívo", "epix",
	"enduro", "tactix", "quatix", "descent", "lily", "swim", "marq", "approach",
	"edge", "hrm", "index", "cirqa",
}

func looksGarmin(d ble.DeviceInfo) bool {
	name := strings.ToLower(d.Name + " " + d.Alias)
	for _, h := range garminHints {
		if strings.Contains(name, h) {
			return true
		}
	}
	for _, u := range d.UUIDs {
		u = strings.ToLower(u)
		if strings.HasPrefix(u, "6a4e") || strings.HasPrefix(u, "9b012401") {
			return true
		}
	}
	return false
}

// StartScan runs discovery in the background.
func (m *Manager) StartScan(d time.Duration) {
	m.mu.Lock()
	if m.scanning {
		m.mu.Unlock()
		return
	}
	m.scanning = true
	m.scanResults = map[string]ScanResult{}
	m.mu.Unlock()

	go func() {
		defer func() {
			m.mu.Lock()
			m.scanning = false
			m.mu.Unlock()
		}()
		err := m.adapter.Discover(m.ctx, d, func(info ble.DeviceInfo) {
			r := ScanResult{
				Address: info.Address,
				Name:    info.Name,
				RSSI:    info.RSSI,
				Paired:  info.Paired,
				Garmin:  looksGarmin(info),
			}
			if r.Name == "" {
				r.Name = info.Alias
			}
			m.mu.Lock()
			prev, seen := m.scanResults[r.Address]
			m.scanResults[r.Address] = r
			m.mu.Unlock()
			if !seen || prev.Name != r.Name {
				m.events.Publish("scan_result", r)
			}
		})
		if err != nil {
			m.log.Warn("daemon: scan failed", "err", err)
		}
	}()
}

// ScanResults returns the current discovery snapshot, Garmin devices first.
func (m *Manager) ScanResults() []ScanResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ScanResult, 0, len(m.scanResults))
	for _, r := range m.scanResults {
		out = append(out, r)
	}
	sortScanResults(out)
	return out
}

func sortScanResults(rs []ScanResult) {
	// Garmin watches first, then by signal strength; a stable order keeps the
	// list from jumping around while scanning.
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0; j-- {
			a, b := rs[j-1], rs[j]
			better := (b.Garmin && !a.Garmin) || (b.Garmin == a.Garmin && b.RSSI > a.RSSI)
			if !better {
				break
			}
			rs[j-1], rs[j] = b, a
		}
	}
}

// Pairing returns the pending agent request, if any.
func (m *Manager) Pairing() PairingState { return m.pairing.State() }

// SubmitPasskey answers a passkey request.
func (m *Manager) SubmitPasskey(passkey uint32) error {
	return m.pairing.Submit(pairingReply{passkey: passkey, confirm: true})
}

// ConfirmPairing answers a numeric comparison request.
func (m *Manager) ConfirmPairing(ok bool) error {
	return m.pairing.Submit(pairingReply{confirm: ok, cancel: !ok})
}

// CancelPairing aborts the pending request.
func (m *Manager) CancelPairing() error {
	return m.pairing.Submit(pairingReply{cancel: true})
}

// Pair bonds with a watch that is in pairing mode.
func (m *Manager) Pair(addr string) error {
	dev, err := m.adapter.Device(addr)
	if err != nil {
		// The watch may not be in the object tree yet; a short scan fixes that.
		scanCtx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
		m.adapter.Discover(scanCtx, 20*time.Second, nil)
		cancel()
		dev, err = m.adapter.Device(addr)
		if err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(m.ctx, 3*time.Minute)
	defer cancel()
	if err := dev.Pair(ctx); err != nil {
		return err
	}
	if err := dev.SetTrusted(true); err != nil {
		m.log.Warn("daemon: could not trust device", "err", err)
	}
	return nil
}

// Forget removes the bond and all stored data for a watch.
func (m *Manager) Forget(addr string) error {
	m.Disconnect()
	if err := m.adapter.RemoveDevice(addr); err != nil {
		m.log.Warn("daemon: remove device", "err", err)
	}
	return m.db.ForgetDevice(addr)
}

// ------------------------------------------------------------- connection ---

// Connect opens a GFDI session with a watch and keeps it until Disconnect.
func (m *Manager) Connect(addr string) error {
	m.mu.Lock()
	if m.session != nil {
		m.mu.Unlock()
		return fmt.Errorf("daemon: already connected")
	}
	m.mu.Unlock()

	dev, err := m.adapter.Device(addr)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
	defer cancel()
	if err := dev.Connect(ctx); err != nil {
		return fmt.Errorf("daemon: connect %s: %w", addr, err)
	}

	trCtx, trCancel := context.WithTimeout(m.ctx, 30*time.Second)
	tr, err := garmin.OpenTransport(trCtx, dev, m.log)
	trCancel()
	if err != nil {
		dev.Disconnect()
		return err
	}

	row, err := m.db.DeviceByAddress(addr)
	if err != nil {
		m.log.Warn("daemon: device lookup", "err", err)
	}
	firstConnect := row == nil
	if row == nil {
		row = &store.Device{Address: addr, Name: dev.Name(), CreatedMs: time.Now().UnixMilli()}
		if err := m.db.UpsertDevice(row); err != nil {
			m.log.Error("daemon: create device", "err", err)
		}
	}

	sess := garmin.NewSession(m.ctx, tr, garmin.Options{
		PhoneName:        m.settingString("phoneName", "Pulse"),
		Manufacturer:     "Ubuntu Touch",
		Model:            "Pulse",
		SyncTime:         m.settingBool("syncTime", true),
		FirstConnect:     firstConnect,
		KeepFilesOnWatch: m.settingBool("keepFilesOnWatch", false),
		NotificationsAllowed: func() bool {
			return m.settingBool("notificationsEnabled", true)
		},
	}, m.log)

	m.wireHooks(sess, row)

	m.mu.Lock()
	m.session = sess
	m.dev = dev
	m.deviceRow = row
	m.autoReconnect = true
	m.battery = -1
	m.mu.Unlock()

	m.wg.Add(1)
	go m.sessionLoop(sess, dev, row)
	return nil
}

func (m *Manager) wireHooks(sess *garmin.Session, row *store.Device) {
	deviceID := row.ID

	if m.deps.Weather != nil {
		sess.Hooks.Weather = func(req *gfdi.WeatherRequest) ([]byte, error) {
			if !m.settingBool("weatherEnabled", true) {
				return nil, nil
			}
			lat := semicirclesToDegrees(req.LatSemicircles)
			lon := semicirclesToDegrees(req.LonSemicircles)
			hours := int(req.HoursOfForecast)
			if hours <= 0 {
				hours = 12
			}
			ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
			defer cancel()
			return m.deps.Weather(ctx, lat, lon, hours)
		}
	}
	if m.deps.NotificationContent != nil {
		sess.Hooks.NotificationContent = m.deps.NotificationContent
	}
	if m.deps.AppName != nil {
		sess.Hooks.AppName = m.deps.AppName
	}
	if m.deps.Protobuf != nil {
		sess.Hooks.Protobuf = m.deps.Protobuf
	}
	if m.deps.SetProtobufSender != nil {
		m.deps.SetProtobufSender(func(requestID uint16, payload []byte) {
			sess.SendProtobufResponse(requestID, payload)
		})
	}

	sess.Hooks.HaveFile = func(entry garmin.DirectoryEntry) bool {
		return m.db.HasFitFile(deviceID, int(entry.FileNumber))
	}

	sess.Hooks.FileDownloaded = func(entry garmin.DirectoryEntry, data []byte) error {
		f := &store.FitFile{
			DeviceID:     deviceID,
			FileNumber:   int(entry.FileNumber),
			DataType:     int(entry.FileDataType),
			SubType:      int(entry.FileSubType),
			FileTs:       entry.Timestamp * 1000,
			Flags:        int(entry.FileFlags),
			Size:         len(data),
			DownloadedMs: time.Now().UnixMilli(),
			Data:         data,
		}
		if err := m.db.PutFitFile(f); err != nil {
			m.log.Error("daemon: store fit file", "err", err)
		}
		if !entry.IsFit() {
			return nil
		}
		res, err := m.imp.Import(deviceID, entry.FileSubType, data)
		if err != nil {
			m.log.Warn("daemon: import failed", "err", err, "index", entry.FileIndex)
			return nil
		}
		f.Imported = true
		if err := m.db.PutFitFile(f); err != nil {
			m.log.Warn("daemon: mark imported", "err", err)
		}
		m.log.Info("daemon: imported", "type", res.FileType, "records", res.Records,
			"activity", res.Activity, "stress", res.Stress, "sleep", res.SleepStages)
		m.events.Publish("file_imported", res)
		return nil
	}
}

func semicirclesToDegrees(v int32) float64 {
	return float64(v) * (180.0 / 2147483648.0)
}

func (m *Manager) sessionLoop(sess *garmin.Session, dev *ble.Device, row *store.Device) {
	defer m.wg.Done()
	defer func() {
		m.mu.Lock()
		if m.session == sess {
			m.session = nil
			m.dev = nil
			m.syncing = false
		}
		m.mu.Unlock()
		sess.Close()
		dev.Disconnect()
		m.events.Publish("disconnected", nil)
	}()

	for ev := range sess.Events() {
		m.events.Publish(ev.Kind, ev.Data)
		switch ev.Kind {
		case garmin.EventDeviceInfo:
			if info, ok := ev.Data.(*gfdi.DeviceInformation); ok {
				row.Name = strings.TrimSpace(info.DeviceName + " " + info.DeviceModel)
				row.Model = info.DeviceModel
				row.Firmware = info.FirmwareVersion()
				row.UnitID = int64(info.UnitNumber)
				row.ProductNumber = int64(info.ProductNumber)
				if err := m.db.UpsertDevice(row); err != nil {
					m.log.Warn("daemon: update device", "err", err)
				}
			}

		case garmin.EventInitialized:
			m.pushMusicMetadata(sess)
			if m.settingBool("autoSyncOnConnect", true) {
				sess.StartSync()
			}

		case garmin.EventBattery:
			if data, ok := ev.Data.(map[string]any); ok {
				if v, ok := data["level"].(int32); ok {
					m.setBattery(row.ID, int(v))
				}
			}

		case garmin.EventSyncStarted:
			m.mu.Lock()
			m.syncing = true
			m.progress = Progress{}
			m.mu.Unlock()

		case garmin.EventSyncProgress:
			if data, ok := ev.Data.(map[string]any); ok {
				m.mu.Lock()
				if v, ok := data["fileIndex"].(uint16); ok {
					m.progress.FileIndex = int(v)
				}
				if v, ok := data["received"].(int); ok {
					m.progress.Received = v
				}
				if v, ok := data["total"].(int); ok {
					m.progress.Total = v
				}
				if v, ok := data["remaining"].(int); ok {
					m.progress.Remaining = v
				}
				m.mu.Unlock()
			}

		case garmin.EventSyncFinished:
			m.mu.Lock()
			m.syncing = false
			m.progress = Progress{}
			m.mu.Unlock()
			now := time.Now().UnixMilli()
			row.LastSyncMs = now
			if err := m.db.TouchSync(row.ID, now); err != nil {
				m.log.Warn("daemon: touch sync", "err", err)
			}

		case garmin.EventMusicControl:
			if data, ok := ev.Data.(map[string]any); ok && m.deps.MusicCommand != nil {
				if cmd, ok := data["command"].(uint8); ok {
					if err := m.deps.MusicCommand(cmd); err != nil {
						m.log.Warn("daemon: music command", "err", err)
					}
					m.pushMusicMetadata(sess)
				}
			}

		case garmin.EventNotifAction:
			m.handleNotificationAction(ev.Data)

		case garmin.EventDisconnected:
			return
		}
	}
}

func (m *Manager) handleNotificationAction(data any) {
	d, ok := data.(map[string]any)
	if !ok {
		return
	}
	action, _ := d["action"].(uint8)
	switch action {
	case gfdi.ActionAcceptIncomingCall:
		if m.deps.AcceptCall != nil {
			if err := m.deps.AcceptCall(); err != nil {
				m.log.Warn("daemon: accept call", "err", err)
			}
		}
	case gfdi.ActionRejectIncomingCall:
		if m.deps.RejectCall != nil {
			if err := m.deps.RejectCall(); err != nil {
				m.log.Warn("daemon: reject call", "err", err)
			}
		}
	}
}

func (m *Manager) pushMusicMetadata(sess *garmin.Session) {
	if m.deps.MusicMetadata == nil {
		return
	}
	values := m.deps.MusicMetadata()
	if len(values) == 0 {
		return
	}
	sess.SendMusicEntityUpdate(values)
}

// OnBatteryLevel records a watch battery percentage reported over protobuf,
// which is the only place the real percentage arrives.
func (m *Manager) OnBatteryLevel(level int32) {
	m.mu.Lock()
	row := m.deviceRow
	m.mu.Unlock()
	if row == nil {
		return
	}
	m.setBattery(row.ID, int(level))
	m.events.Publish("battery", map[string]any{"level": level})
}

func (m *Manager) setBattery(deviceID int64, level int) {
	m.mu.Lock()
	m.battery = level
	m.mu.Unlock()
	if err := m.db.PutBattery(deviceID, time.Now().UnixMilli(), level); err != nil {
		m.log.Warn("daemon: store battery", "err", err)
	}
}

// Disconnect drops the current session.
func (m *Manager) Disconnect() {
	m.mu.Lock()
	sess := m.session
	m.autoReconnect = false
	m.mu.Unlock()
	if sess != nil {
		sess.Close()
	}
}

// Sync asks the watch for new files.
func (m *Manager) Sync() error {
	m.mu.Lock()
	sess := m.session
	m.mu.Unlock()
	if sess == nil {
		return fmt.Errorf("daemon: not connected")
	}
	sess.StartSync()
	return nil
}

// FindWatch makes the watch alert.
func (m *Manager) FindWatch(seconds int) error {
	m.mu.Lock()
	sess := m.session
	m.mu.Unlock()
	if sess == nil {
		return fmt.Errorf("daemon: not connected")
	}
	return sess.FindMyWatch(time.Duration(seconds) * time.Second)
}

// CancelFindWatch stops the alert.
func (m *Manager) CancelFindWatch() error {
	m.mu.Lock()
	sess := m.session
	m.mu.Unlock()
	if sess == nil {
		return fmt.Errorf("daemon: not connected")
	}
	return sess.CancelFindMyWatch()
}

// Status reports the connected watch.
func (m *Manager) Status() (*DeviceStatus, bool, Progress) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deviceRow == nil {
		return nil, m.syncing, m.progress
	}
	st := &DeviceStatus{
		Address:      m.deviceRow.Address,
		Name:         m.deviceRow.Name,
		Model:        m.deviceRow.Model,
		Firmware:     m.deviceRow.Firmware,
		Connected:    m.session != nil,
		BatteryLevel: m.battery,
		LastSyncMs:   m.deviceRow.LastSyncMs,
	}
	if m.session != nil {
		st.Initialized = m.session.Initialized()
	}
	if st.BatteryLevel < 0 {
		if level, ok := m.db.LatestBattery(m.deviceRow.ID); ok {
			st.BatteryLevel = level
		} else {
			st.BatteryLevel = 0
		}
	}
	return st, m.syncing, m.progress
}

// AdapterPowered reports the controller state.
func (m *Manager) AdapterPowered() bool { return m.adapter.Powered() }

// ActiveDeviceID returns the database id of the connected or last used watch.
func (m *Manager) ActiveDeviceID() (int64, bool) {
	m.mu.Lock()
	row := m.deviceRow
	m.mu.Unlock()
	if row != nil {
		return row.ID, true
	}
	devices, err := m.db.Devices()
	if err != nil || len(devices) == 0 {
		return 0, false
	}
	return devices[0].ID, true
}

// ------------------------------------------------------------ background ---

func (m *Manager) notificationLoop() {
	defer m.wg.Done()
	if m.deps.Notifications == nil {
		return
	}
	for {
		select {
		case <-m.ctx.Done():
			return
		case n, ok := <-m.deps.Notifications:
			if !ok {
				return
			}
			m.events.Publish("notification", n.Content)
			if !m.settingBool("notificationsEnabled", true) {
				m.log.Debug("daemon: notification dropped, forwarding disabled",
					"id", n.Content.ID, "app", n.Content.AppIdentifier)
				continue
			}
			if n.Source == "waydroid" && !m.settingBool("notifyWaydroid", true) {
				m.log.Debug("daemon: android notification dropped, waydroid forwarding off",
					"id", n.Content.ID, "app", n.Content.AppIdentifier)
				continue
			}
			m.mu.Lock()
			sess := m.session
			m.mu.Unlock()
			if sess == nil || !sess.Initialized() {
				m.log.Debug("daemon: notification dropped, no watch session",
					"id", n.Content.ID, "app", n.Content.AppIdentifier)
				continue
			}
			if n.Removed {
				sess.RemoveNotification(n.Content.ID, n.Content.Category)
				m.log.Info("daemon: notification removed on watch",
					"id", n.Content.ID, "app", n.Content.AppIdentifier)
			} else {
				sess.SendNotification(n.Content)
				if sess.NotificationsSubscribed() {
					m.log.Info("daemon: notification sent to watch",
						"id", n.Content.ID, "app", n.Content.AppIdentifier, "title", n.Content.Title)
				} else {
					// The watch acknowledges the frame and drops it. Nothing on
					// our side can change that: the switch lives on the watch.
					m.log.Warn("daemon: watch has notifications switched off, it will not show this",
						"id", n.Content.ID, "app", n.Content.AppIdentifier)
				}
			}
		}
	}
}

func (m *Manager) autoSyncLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var last time.Time
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			every := m.settingInt("autoSyncMinutes", 30)
			if every <= 0 {
				continue
			}
			if time.Since(last) < time.Duration(every)*time.Minute {
				continue
			}
			m.mu.Lock()
			sess := m.session
			syncing := m.syncing
			m.mu.Unlock()
			if sess == nil || syncing || !sess.Initialized() {
				continue
			}
			last = time.Now()
			sess.StartSync()
		}
	}
}

// reconnectLoop brings the session back after the watch wanders out of range.
func (m *Manager) reconnectLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			need := m.autoReconnect && m.session == nil && m.deviceRow != nil
			addr := ""
			if m.deviceRow != nil {
				addr = m.deviceRow.Address
			}
			m.mu.Unlock()
			if !need {
				continue
			}
			if err := m.Connect(addr); err != nil {
				m.log.Debug("daemon: reconnect failed", "err", err)
			}
		}
	}
}

// ---------------------------------------------------------------- settings ---

func (m *Manager) settingString(key, def string) string { return m.db.Setting(key, def) }

func (m *Manager) settingBool(key string, def bool) bool {
	v := m.db.Setting(key, "")
	if v == "" {
		return def
	}
	return v == "true" || v == "1"
}

func (m *Manager) settingInt(key string, def int) int {
	v := m.db.Setting(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// AnalyticsSettings builds the aggregation settings from stored preferences.
func (m *Manager) AnalyticsSettings() analytics.Settings {
	s := analytics.DefaultSettings()
	s.StepsGoal = m.settingInt("stepsGoal", s.StepsGoal)
	s.SleepGoalMinutes = m.settingInt("sleepGoalMinutes", s.SleepGoalMinutes)
	s.ActiveCaloriesGoal = m.settingInt("activeCaloriesGoal", s.ActiveCaloriesGoal)
	s.DistanceGoalM = m.settingInt("distanceGoalM", s.DistanceGoalM)
	s.ActiveMinutesGoal = m.settingInt("activeMinutesGoal", s.ActiveMinutesGoal)
	s.IntensityGoal = m.settingInt("intensityGoal", s.IntensityGoal)
	s.Units = m.settingString("units", s.Units)
	s.AnyGoalStreak = m.settingBool("anyGoalStreak", false)
	return s
}

// UISettings is the full settings document the API exposes.
type UISettings struct {
	Theme                string `json:"theme"`
	Accent               string `json:"accent"`
	StepsGoal            int    `json:"stepsGoal"`
	SleepGoalMinutes     int    `json:"sleepGoalMinutes"`
	ActiveCaloriesGoal   int    `json:"activeCaloriesGoal"`
	DistanceGoalM        int    `json:"distanceGoalM"`
	ActiveMinutesGoal    int    `json:"activeMinutesGoal"`
	IntensityGoal        int    `json:"intensityGoal"`
	Units                string `json:"units"`
	SyncTime             bool   `json:"syncTime"`
	WeatherEnabled       bool   `json:"weatherEnabled"`
	NotificationsEnabled bool   `json:"notificationsEnabled"`
	NotifyWaydroid       bool   `json:"notifyWaydroid"`
	KeepFilesOnWatch     bool   `json:"keepFilesOnWatch"`
	AnyGoalStreak        bool   `json:"anyGoalStreak"`
	AutoSyncMinutes      int    `json:"autoSyncMinutes"`
}

// Settings reads the settings document.
func (m *Manager) Settings() UISettings {
	return UISettings{
		Theme:                m.settingString("theme", "dark"),
		Accent:               m.settingString("accent", "blue"),
		StepsGoal:            m.settingInt("stepsGoal", 10000),
		SleepGoalMinutes:     m.settingInt("sleepGoalMinutes", 480),
		ActiveCaloriesGoal:   m.settingInt("activeCaloriesGoal", 350),
		DistanceGoalM:        m.settingInt("distanceGoalM", 5000),
		ActiveMinutesGoal:    m.settingInt("activeMinutesGoal", 60),
		IntensityGoal:        m.settingInt("intensityGoal", 30),
		Units:                m.settingString("units", "metric"),
		SyncTime:             m.settingBool("syncTime", true),
		WeatherEnabled:       m.settingBool("weatherEnabled", true),
		NotificationsEnabled: m.settingBool("notificationsEnabled", true),
		NotifyWaydroid:       m.settingBool("notifyWaydroid", true),
		KeepFilesOnWatch:     m.settingBool("keepFilesOnWatch", false),
		AnyGoalStreak:        m.settingBool("anyGoalStreak", false),
		AutoSyncMinutes:      m.settingInt("autoSyncMinutes", 30),
	}
}

// SaveSettings persists the settings document.
func (m *Manager) SaveSettings(s UISettings) error {
	pairs := map[string]string{
		"theme":                s.Theme,
		"accent":               s.Accent,
		"stepsGoal":            strconv.Itoa(s.StepsGoal),
		"sleepGoalMinutes":     strconv.Itoa(s.SleepGoalMinutes),
		"activeCaloriesGoal":   strconv.Itoa(s.ActiveCaloriesGoal),
		"distanceGoalM":        strconv.Itoa(s.DistanceGoalM),
		"activeMinutesGoal":    strconv.Itoa(s.ActiveMinutesGoal),
		"intensityGoal":        strconv.Itoa(s.IntensityGoal),
		"units":                s.Units,
		"syncTime":             boolString(s.SyncTime),
		"weatherEnabled":       boolString(s.WeatherEnabled),
		"notificationsEnabled": boolString(s.NotificationsEnabled),
		"notifyWaydroid":       boolString(s.NotifyWaydroid),
		"keepFilesOnWatch":     boolString(s.KeepFilesOnWatch),
		"anyGoalStreak":        boolString(s.AnyGoalStreak),
		"autoSyncMinutes":      strconv.Itoa(s.AutoSyncMinutes),
	}
	for k, v := range pairs {
		if err := m.db.SetSetting(k, v); err != nil {
			return err
		}
	}
	m.events.Publish("settings_changed", s)
	return nil
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// PushWeather sends the current weather to the watch outside of a request.
func (m *Manager) PushWeather(lat, lon float64) error {
	m.mu.Lock()
	sess := m.session
	m.mu.Unlock()
	if sess == nil || m.deps.Weather == nil {
		return fmt.Errorf("daemon: weather unavailable")
	}
	ctx, cancel := context.WithTimeout(m.ctx, 20*time.Second)
	defer cancel()
	payload, err := m.deps.Weather(ctx, lat, lon, 12)
	if err != nil {
		return err
	}
	sess.SendFitRecords(payload)
	m.mu.Lock()
	m.lastWeather = time.Now()
	m.mu.Unlock()
	return nil
}

// UploadFit pushes a generated FIT file (alarms, settings) to the watch.
func (m *Manager) UploadFit(sub uint8, data []byte) error {
	m.mu.Lock()
	sess := m.session
	m.mu.Unlock()
	if sess == nil {
		return fmt.Errorf("daemon: not connected")
	}
	return sess.Upload(garmin.FileType{DataType: 128, SubType: sub}, data)
}

// DecodeFitFile is a diagnostics helper used by the API to inspect a stored
// file without another sync.
func (m *Manager) DecodeFitFile(data []byte) (json.RawMessage, error) {
	f, err := fit.Decode(data)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, r := range f.Records {
		counts[r.Name]++
	}
	return json.Marshal(counts)
}
