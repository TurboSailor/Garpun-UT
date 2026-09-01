package garmin

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"pulse/backend/internal/fit"
	"pulse/backend/internal/gfdi"
)

// Event is emitted by the session for the layers above (storage, UI, HTTP API).
type Event struct {
	Kind string `json:"kind"`
	Data any    `json:"data,omitempty"`
}

// Event kinds.
const (
	EventDeviceInfo     = "device_info"
	EventCapabilities   = "capabilities"
	EventInitialized    = "initialized"
	EventSyncStarted    = "sync_started"
	EventSyncProgress   = "sync_progress"
	EventSyncFinished   = "sync_finished"
	EventFileDownloaded = "file_downloaded"
	EventBattery        = "battery"
	EventFindPhone      = "find_phone"
	EventMusicControl   = "music_control"
	EventNotifAction    = "notification_action"
	EventWeatherWanted  = "weather_request"
	EventDisconnected   = "disconnected"
	EventLog            = "log"
)

// Options configures a session.
type Options struct {
	// PhoneName is advertised to the watch as the Bluetooth friendly name.
	PhoneName string
	// Manufacturer and Model are reported in the device information reply.
	Manufacturer string
	Model        string
	// SyncTime enables answering CURRENT_TIME_REQUEST and sending TIME_UPDATED.
	SyncTime bool
	// FirstConnect sends the pairing completion events the watch expects the
	// first time a new host shows up.
	FirstConnect bool
	// KeepFilesOnWatch skips the ARCHIVE flag after a successful download.
	KeepFilesOnWatch bool
	// FetchUnknownFiles downloads file types upstream does not mark as pull.
	FetchUnknownFiles bool
}

// Session drives one connected watch: it owns the GFDI dispatch loop, the
// initialisation handshake and the file sync state machine.
type Session struct {
	opts   Options
	log    *slog.Logger
	tr     Transport
	events chan Event

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu            sync.Mutex
	deviceInfo    *gfdi.DeviceInformation
	capabilities  map[int]bool
	maxPacketSize int
	initialized   bool
	supportedFile []gfdi.SupportedFileType

	download *downloadState
	queue    []DirectoryEntry
	archived map[uint16]bool
	syncing  bool

	upload *uploadState

	protoMu   sync.Mutex
	protoNext uint16
	protoOut  map[uint16][]byte // requestId -> full outbound payload awaiting chunk acks
	protoIn   map[uint16][]byte // requestId -> reassembly buffer
	protoWait map[uint16]chan []byte

	notifyMu   sync.Mutex
	notifState *notificationTransfer

	// Hooks let the daemon supply data the protocol layer must not know about.
	Hooks Hooks
}

// Hooks are supplied by the daemon to answer watch-initiated requests.
type Hooks struct {
	// Weather returns an encoded FIT weather payload for the requested spot.
	Weather func(req *gfdi.WeatherRequest) ([]byte, error)
	// NotificationContent supplies the notification the watch is asking about.
	NotificationContent func(id int32) *NotificationContent
	// AppName resolves an application identifier to a display name.
	AppName func(appID string) string
	// Protobuf handles an inbound Smart message and optionally returns a reply.
	Protobuf func(requestID uint16, payload []byte) ([]byte, error)
	// FileDownloaded is called for every completed non-directory file.
	FileDownloaded func(entry DirectoryEntry, data []byte) error
}

// NewSession starts the dispatch loop over an open transport.
func NewSession(ctx context.Context, tr Transport, opts Options, log *slog.Logger) *Session {
	ctx, cancel := context.WithCancel(ctx)
	s := &Session{
		opts:          opts,
		log:           log,
		tr:            tr,
		events:        make(chan Event, 128),
		ctx:           ctx,
		cancel:        cancel,
		capabilities:  map[int]bool{},
		maxPacketSize: 375,
		archived:      map[uint16]bool{},
		protoOut:      map[uint16][]byte{},
		protoIn:       map[uint16][]byte{},
		protoWait:     map[uint16]chan []byte{},
	}
	s.wg.Add(1)
	go s.loop()
	return s
}

// Events yields session events. The channel is closed when the session ends.
func (s *Session) Events() <-chan Event { return s.events }

// Close tears down the session and its transport.
func (s *Session) Close() error {
	s.cancel()
	err := s.tr.Close()
	s.wg.Wait()
	return err
}

// Initialized reports whether the handshake completed.
func (s *Session) Initialized() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.initialized
}

// DeviceInfo returns the watch identity once known.
func (s *Session) DeviceInfo() *gfdi.DeviceInformation {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deviceInfo
}

// Capabilities returns the capability ordinals the watch reported.
func (s *Session) Capabilities() map[int]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[int]bool, len(s.capabilities))
	for k, v := range s.capabilities {
		out[k] = v
	}
	return out
}

func (s *Session) emit(kind string, data any) {
	select {
	case s.events <- Event{Kind: kind, Data: data}:
	default:
		s.log.Warn("garmin: event queue full", "kind", kind)
	}
}

func (s *Session) send(frame []byte) {
	if err := s.tr.Send(frame); err != nil {
		s.log.Error("garmin: send failed", "err", err)
	}
}

func (s *Session) loop() {
	defer s.wg.Done()
	defer close(s.events)
	frames := s.tr.Frames()
	for {
		select {
		case <-s.ctx.Done():
			return
		case raw, ok := <-frames:
			if !ok {
				s.emit(EventDisconnected, nil)
				return
			}
			f, err := gfdi.ParseFrame(raw)
			if err != nil {
				s.log.Warn("garmin: bad frame", "err", err, "len", len(raw))
				continue
			}
			s.handle(f)
		}
	}
}

// handle implements the upstream ordering: run handlers, send the status for
// the inbound message, send the message's own reply, then any follow-up.
func (s *Session) handle(f *gfdi.Frame) {
	switch f.Type {
	case gfdi.MsgResponse:
		s.handleStatus(f)

	case gfdi.MsgDeviceInformation:
		info, err := gfdi.ParseDeviceInformation(f.Payload)
		if err != nil {
			s.log.Warn("garmin: bad device information", "err", err)
			s.send(gfdi.GenericStatus(f.Type, gfdi.StatusDecodeError))
			return
		}
		s.mu.Lock()
		s.deviceInfo = info
		if info.MaxPacketSize > 0 {
			s.maxPacketSize = int(info.MaxPacketSize)
		}
		s.mu.Unlock()
		s.log.Info("garmin: device information",
			"name", info.DeviceName, "model", info.DeviceModel,
			"firmware", info.FirmwareVersion(), "unit", info.UnitNumber,
			"maxPacketSize", info.MaxPacketSize)
		s.emit(EventDeviceInfo, info)
		s.send(gfdi.DeviceInformationResponse(info, s.opts.PhoneName, s.opts.Manufacturer, s.opts.Model))

	case gfdi.MsgAuthNegotiation:
		r := gfdi.NewReader(f.Payload)
		unk, _ := r.U8()
		flags, _ := r.I32()
		s.send(gfdi.AuthNegotiationResponse(unk, flags))

	case gfdi.MsgConfiguration:
		r := gfdi.NewReader(f.Payload)
		n, err := r.U8()
		if err != nil {
			s.send(gfdi.GenericStatus(f.Type, gfdi.StatusDecodeError))
			return
		}
		bits := make([]byte, 0, n)
		for range int(n) {
			b, err := r.U8()
			if err != nil {
				break
			}
			bits = append(bits, b)
		}
		caps := gfdi.DecodeCapabilities(bits)
		s.mu.Lock()
		s.capabilities = caps
		s.mu.Unlock()
		s.emit(EventCapabilities, caps)
		s.send(gfdi.GenericStatus(f.Type, gfdi.StatusACK))
		s.send(gfdi.ConfigurationResponse())
		s.completeInitialization()

	case gfdi.MsgCurrentTimeRequest:
		if !s.opts.SyncTime {
			s.send(gfdi.GenericStatus(f.Type, gfdi.StatusUnsupported))
			return
		}
		r := gfdi.NewReader(f.Payload)
		ref, _ := r.I32()
		now := time.Now()
		_, offset := now.Zone()
		s.send(gfdi.CurrentTimeResponse(ref, now.Unix(), int32(offset), 0, 0))

	case gfdi.MsgSynchronization:
		sync, err := gfdi.ParseSynchronization(f.Payload)
		s.send(gfdi.GenericStatus(f.Type, gfdi.StatusACK))
		if err != nil {
			return
		}
		s.log.Info("garmin: sync requested", "type", sync.SyncType, "mask", fmt.Sprintf("%#x", sync.Bitmask))
		if sync.ShouldProceed() {
			s.StartSync()
		}

	case gfdi.MsgFileTransferData:
		s.onFileTransferData(f)

	case gfdi.MsgWeatherRequest:
		req, err := gfdi.ParseWeatherRequest(f.Payload)
		s.send(gfdi.GenericStatus(f.Type, gfdi.StatusACK))
		if err != nil {
			return
		}
		s.emit(EventWeatherWanted, req)
		if s.Hooks.Weather != nil {
			go s.pushWeather(req)
		}

	case gfdi.MsgFindMyPhoneRequest:
		r := gfdi.NewReader(f.Payload)
		duration, _ := r.U8()
		s.send(gfdi.GenericStatus(f.Type, gfdi.StatusACK))
		s.emit(EventFindPhone, map[string]any{"active": true, "duration": duration})

	case gfdi.MsgFindMyPhoneCancel:
		s.send(gfdi.GenericStatus(f.Type, gfdi.StatusACK))
		s.emit(EventFindPhone, map[string]any{"active": false})

	case gfdi.MsgMusicControl:
		r := gfdi.NewReader(f.Payload)
		cmd, _ := r.U8()
		s.send(gfdi.GenericStatus(f.Type, gfdi.StatusACK))
		s.emit(EventMusicControl, map[string]any{"command": cmd})

	case gfdi.MsgMusicControlCapabilities:
		s.send(gfdi.MusicControlCapabilitiesResponse())

	case gfdi.MsgNotificationSubscription:
		r := gfdi.NewReader(f.Payload)
		enable, _ := r.U8()
		s.send(gfdi.NotificationSubscriptionStatus(enable == 1, enable))

	case gfdi.MsgNotificationControl:
		s.onNotificationControl(f)

	case gfdi.MsgProtobufRequest, gfdi.MsgProtobufResponse:
		s.onProtobuf(f)

	case gfdi.MsgBatteryStatus:
		r := gfdi.NewReader(f.Payload)
		wire, _ := r.U8()
		raw, _ := r.U8()
		s.send(gfdi.GenericStatus(f.Type, gfdi.StatusACK))
		s.emit(EventBattery, map[string]any{
			"wireStatus": wire & 0x70,
			"voltage":    float64(raw) / 100.0,
		})

	case gfdi.MsgFileAvailable:
		// Upstream deliberately refuses these; the watch then offers the file
		// through the regular directory listing instead.
		s.send(gfdi.GenericStatus(f.Type, gfdi.StatusUnsupported))

	case gfdi.MsgFitDefinition, gfdi.MsgFitData:
		// Inbound realtime FIT records; acknowledge so the watch keeps going.
		w := gfdi.NewWriter()
		w.U16(f.Type)
		w.U8(gfdi.StatusACK)
		w.U8(0) // APPLIED
		s.send(gfdi.BuildFrame(gfdi.MsgResponse, w.Bytes()))
		s.emit(EventLog, map[string]any{"fit": f.Type, "len": len(f.Payload)})

	default:
		s.log.Debug("garmin: unhandled message", "type", f.Type, "len", len(f.Payload))
		s.send(gfdi.GenericStatus(f.Type, gfdi.StatusUnsupported))
	}
}

func (s *Session) handleStatus(f *gfdi.Frame) {
	st, err := gfdi.ParseStatus(f.Payload)
	if err != nil {
		return
	}
	switch st.OriginalType {
	case gfdi.MsgFilter:
		if st.OK() {
			s.initiateDirectoryDownload()
		}
	case gfdi.MsgDownloadRequest:
		s.onDownloadStatus(st)
	case gfdi.MsgFileTransferData:
		s.onTransferStatus(st)
	case gfdi.MsgCreateFile:
		s.onCreateFileStatus(st)
	case gfdi.MsgUploadRequest:
		s.onUploadStatus(st)
	case gfdi.MsgSupportedFileTypesRequest:
		if types, err := st.SupportedFileTypes(); err == nil {
			s.mu.Lock()
			s.supportedFile = types
			s.mu.Unlock()
		}
	case gfdi.MsgNotificationData:
		s.onNotificationDataStatus(st)
	case gfdi.MsgProtobufRequest, gfdi.MsgProtobufResponse:
		s.onProtobufStatus(st)
	case gfdi.MsgSetFileFlag:
		// nothing to do, the flag is best effort
	default:
		if !st.OK() {
			s.log.Warn("garmin: negative status", "for", st.OriginalType, "status", st.Status)
		}
	}
}

// completeInitialization mirrors GarminSupport.completeInitialization.
func (s *Session) completeInitialization() {
	s.mu.Lock()
	if s.initialized {
		s.mu.Unlock()
		return
	}
	s.initialized = true
	first := s.opts.FirstConnect
	s.mu.Unlock()

	s.send(gfdi.SupportedFileTypesRequest())
	s.send(gfdi.DeviceSettings([]gfdi.DeviceSetting{
		gfdi.BoolSetting(gfdi.SettingAutoUploadEnabled, true),
		gfdi.BoolSetting(gfdi.SettingWeatherConditions, true),
		gfdi.BoolSetting(gfdi.SettingWeatherAlerts, false),
	}))
	if s.opts.SyncTime {
		s.send(gfdi.SystemEvent(gfdi.SysTimeUpdated, 0))
	}
	s.send(gfdi.SystemEvent(gfdi.SysSyncReady, 0))
	s.enableBatteryUpdates()

	if first {
		s.send(gfdi.SystemEvent(gfdi.SysPairComplete, 0))
		s.send(gfdi.SystemEvent(gfdi.SysSyncComplete, 0))
		s.send(gfdi.SystemEvent(gfdi.SysSetupWizardComplete, 0))
	}
	s.log.Info("garmin: session initialized")
	s.emit(EventInitialized, nil)
}

// SendSystemEvent exposes SYSTEM_EVENT to the daemon (foreground/background).
func (s *Session) SendSystemEvent(eventType uint8) { s.send(gfdi.SystemEvent(eventType, 0)) }

func (s *Session) pushWeather(req *gfdi.WeatherRequest) {
	payload, err := s.Hooks.Weather(req)
	if err != nil {
		s.log.Warn("garmin: weather hook failed", "err", err)
		return
	}
	if len(payload) == 0 {
		return
	}
	s.SendFitRecords(payload)
}

// SendFitRecords pushes an encoded FIT record stream to the watch. Definition
// records must travel in FIT_DEFINITION (5011) messages; the watch rejects a
// FIT_DATA payload that contains a definition header, so the two are split.
func (s *Session) SendFitRecords(blob []byte) {
	if len(blob) == 0 {
		return
	}
	s.mu.Lock()
	limit := s.maxPacketSize - 6 // 2 size + 2 id + 2 crc
	s.mu.Unlock()
	if limit < 32 {
		limit = 32
	}

	definitions, data := fit.SplitRecords(blob)
	for _, chunk := range chunkBytes(definitions, limit) {
		s.send(gfdi.BuildFrame(gfdi.MsgFitDefinition, chunk))
	}
	for _, chunk := range chunkBytes(data, limit) {
		s.send(gfdi.BuildFrame(gfdi.MsgFitData, chunk))
	}
}

func chunkBytes(b []byte, limit int) [][]byte {
	if len(b) == 0 {
		return nil
	}
	out := make([][]byte, 0, (len(b)+limit-1)/limit)
	for off := 0; off < len(b); off += limit {
		end := off + limit
		if end > len(b) {
			end = len(b)
		}
		out = append(out, b[off:end])
	}
	return out
}

// SendMusicEntityUpdate pushes now-playing metadata to the watch.
func (s *Session) SendMusicEntityUpdate(values []gfdi.MusicEntityValue) {
	if len(values) == 0 {
		return
	}
	s.send(gfdi.MusicControlEntityUpdate(values))
}
