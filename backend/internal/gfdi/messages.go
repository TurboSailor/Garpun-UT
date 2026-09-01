package gfdi

// GFDI message ids.
const (
	MsgResponse                  uint16 = 5000
	MsgDownloadRequest           uint16 = 5002
	MsgUploadRequest             uint16 = 5003
	MsgFileTransferData          uint16 = 5004
	MsgCreateFile                uint16 = 5005
	MsgFilter                    uint16 = 5007
	MsgSetFileFlag               uint16 = 5008
	MsgFileAvailable             uint16 = 5009
	MsgFitDefinition             uint16 = 5011
	MsgFitData                   uint16 = 5012
	MsgWeatherRequest            uint16 = 5014
	MsgBatteryStatus             uint16 = 5023
	MsgDeviceInformation         uint16 = 5024
	MsgDeviceSettings            uint16 = 5026
	MsgSystemEvent               uint16 = 5030
	MsgSupportedFileTypesRequest uint16 = 5031
	MsgNotificationUpdate        uint16 = 5033
	MsgNotificationControl       uint16 = 5034
	MsgNotificationData          uint16 = 5035
	MsgNotificationSubscription  uint16 = 5036
	MsgSynchronization           uint16 = 5037
	MsgFindMyPhoneRequest        uint16 = 5039
	MsgFindMyPhoneCancel         uint16 = 5040
	MsgMusicControl              uint16 = 5041
	MsgMusicControlCapabilities  uint16 = 5042
	MsgProtobufRequest           uint16 = 5043
	MsgProtobufResponse          uint16 = 5044
	MsgMusicControlEntityUpdate  uint16 = 5049
	MsgConfiguration             uint16 = 5050
	MsgCurrentTimeRequest        uint16 = 5052
	MsgAuthNegotiation           uint16 = 5101
)

// Status codes carried by MsgResponse.
const (
	StatusACK         uint8 = 0
	StatusNAK         uint8 = 1
	StatusUnsupported uint8 = 2
	StatusDecodeError uint8 = 3
	StatusCRCError    uint8 = 4
	StatusLengthError uint8 = 5
)

// System event types (SYSTEM_EVENT payload byte 0).
const (
	SysSyncComplete        uint8 = 0
	SysSyncFail            uint8 = 1
	SysFactoryReset        uint8 = 2
	SysPairStart           uint8 = 3
	SysPairComplete        uint8 = 4
	SysPairFail            uint8 = 5
	SysHostEnterForeground uint8 = 6
	SysHostEnterBackground uint8 = 7
	SysSyncReady           uint8 = 8
	SysNewDownload         uint8 = 9
	SysSoftwareUpdate      uint8 = 10
	SysDeviceDisconnect    uint8 = 11
	SysTutorialComplete    uint8 = 12
	SysSetupWizardStart    uint8 = 13
	SysSetupWizardComplete uint8 = 14
	SysSetupWizardSkipped  uint8 = 15
	SysTimeUpdated         uint8 = 16
)

// Device settings ids (DEVICE_SETTINGS entries).
const (
	SettingDeviceName              uint8 = 0
	SettingCurrentTime             uint8 = 1
	SettingDaylightSavingsOffset   uint8 = 2
	SettingTimeZoneOffset          uint8 = 3
	SettingNextDaylightSavingStart uint8 = 4
	SettingNextDaylightSavingEnd   uint8 = 5
	SettingAutoUploadEnabled       uint8 = 6
	SettingWeatherConditions       uint8 = 7
	SettingWeatherAlerts           uint8 = 8
)

// File flags used by SET_FILE_FLAG.
const (
	FileFlagArchive uint8 = 0x10
	FileFlagDelete  uint8 = 0x20
)

// GarminEpoch is the Garmin/FIT epoch: 1989-12-31T00:00:00Z in Unix seconds.
const GarminEpoch int64 = 631065600

// OurCapabilities returns the 15 byte capability bitfield the phone advertises:
// every capability except UNK_104..UNK_111 and UNK_114..UNK_119, matching what
// Garmin Connect dumps actually contain.
func OurCapabilities() []byte {
	const total = 120
	out := make([]byte, (total+7)/8)
	for i := range total {
		switch {
		case i >= 104 && i <= 111, i >= 114 && i <= 119:
			continue
		}
		out[i/8] |= 1 << (i % 8)
	}
	return out
}

// Capability ordinals worth naming; the watch reports which ones it supports.
const (
	CapSync                  = 3
	CapDeviceInitiatesSync   = 4
	CapHostInitiatedSync     = 5
	CapGNCS                  = 6
	CapAdvancedMusicControls = 7
	CapFindMyPhone           = 8
	CapFindMyWatch           = 9
	CapConnectIQHTTP         = 10
	CapWeatherConditions     = 26
	CapWeatherAlerts         = 27
	CapGPSEphemerisDownload  = 28
	CapExplicitArchive       = 29
	CapTrueUp                = 33
	CapCalendar              = 49
	CapSMSNotifications      = 51
	CapBasicMusicControls    = 52
	CapDeviceInfoFileType    = 55
	CapExploreSync           = 68
	CapCurrentTimeRequest    = 70
	CapContacts              = 71
	CapMultiLinkService      = 76
	CapOAuthCredentials      = 77
	CapRealtimeSettings      = 92
)

// DecodeCapabilities expands a capability bitfield into a set of ordinals.
func DecodeCapabilities(b []byte) map[int]bool {
	out := make(map[int]bool, len(b)*8)
	for i, v := range b {
		for bit := range 8 {
			if v&(1<<bit) != 0 {
				out[i*8+bit] = true
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- inbound ---

// DeviceInformation is message 5024, sent by the watch right after the GFDI
// channel opens.
type DeviceInformation struct {
	ProtocolVersion uint16
	ProductNumber   uint16
	UnitNumber      uint32
	SoftwareVersion uint16
	MaxPacketSize   uint16
	BluetoothName   string
	DeviceName      string
	DeviceModel     string
}

func ParseDeviceInformation(p []byte) (*DeviceInformation, error) {
	r := NewReader(p)
	d := &DeviceInformation{}
	var err error
	if d.ProtocolVersion, err = r.U16(); err != nil {
		return nil, err
	}
	if d.ProductNumber, err = r.U16(); err != nil {
		return nil, err
	}
	if d.UnitNumber, err = r.U32(); err != nil {
		return nil, err
	}
	if d.SoftwareVersion, err = r.U16(); err != nil {
		return nil, err
	}
	if d.MaxPacketSize, err = r.U16(); err != nil {
		return nil, err
	}
	if d.BluetoothName, err = r.Str(); err != nil {
		return nil, err
	}
	if d.DeviceName, err = r.Str(); err != nil {
		return nil, err
	}
	if d.DeviceModel, err = r.Str(); err != nil {
		return nil, err
	}
	return d, nil
}

// FirmwareVersion renders softwareVersion as major.minor.
func (d *DeviceInformation) FirmwareVersion() string {
	return itoa(int(d.SoftwareVersion)/100) + "." + pad2(int(d.SoftwareVersion)%100)
}

// Synchronization is message 5037.
type Synchronization struct {
	SyncType uint8
	Bitmask  uint64
}

// Sync bitmask bits.
const (
	SyncSettings        = 1
	SyncGoals           = 2
	SyncWorkouts        = 3
	SyncCourses         = 4
	SyncActivities      = 5
	SyncRecords         = 6
	SyncSoftwareUpdate  = 8
	SyncDeviceConfig    = 9
	SyncUser            = 11
	SyncSports          = 12
	SyncSegments        = 13
	SyncInstall         = 17
	SyncTrueUp          = 19
	SyncActivitySummary = 21
	SyncMetrics         = 22
	SyncPaceBand        = 23
	SyncSleep           = 26
)

func ParseSynchronization(p []byte) (*Synchronization, error) {
	r := NewReader(p)
	s := &Synchronization{}
	var err error
	if s.SyncType, err = r.U8(); err != nil {
		return nil, err
	}
	size, err := r.U8()
	if err != nil {
		return nil, err
	}
	switch size {
	case 8:
		v, err := r.U64()
		if err != nil {
			return nil, err
		}
		s.Bitmask = v
	case 4:
		v, err := r.U32()
		if err != nil {
			return nil, err
		}
		s.Bitmask = uint64(v)
	}
	return s, nil
}

// ShouldProceed mirrors SynchronizationMessage.shouldProceed: a sync is only
// worth acting on when it involves data we actually download.
func (s *Synchronization) ShouldProceed() bool {
	for _, bit := range []int{SyncWorkouts, SyncActivities, SyncActivitySummary, SyncSleep} {
		if s.Bitmask&(1<<uint(bit)) != 0 {
			return true
		}
	}
	return false
}

// WeatherRequest is message 5014.
type WeatherRequest struct {
	Format          uint8
	LatSemicircles  int32
	LonSemicircles  int32
	HoursOfForecast uint8
}

func ParseWeatherRequest(p []byte) (*WeatherRequest, error) {
	r := NewReader(p)
	w := &WeatherRequest{}
	var err error
	if w.Format, err = r.U8(); err != nil {
		return nil, err
	}
	if w.LatSemicircles, err = r.I32(); err != nil {
		return nil, err
	}
	if w.LonSemicircles, err = r.I32(); err != nil {
		return nil, err
	}
	if w.HoursOfForecast, err = r.U8(); err != nil {
		return nil, err
	}
	return w, nil
}

// FileTransferData is message 5004 arriving from the watch.
type FileTransferData struct {
	Flags      uint8
	CRC        uint16
	DataOffset int32
	Data       []byte
}

func ParseFileTransferData(p []byte) (*FileTransferData, error) {
	r := NewReader(p)
	f := &FileTransferData{}
	var err error
	if f.Flags, err = r.U8(); err != nil {
		return nil, err
	}
	if f.CRC, err = r.U16(); err != nil {
		return nil, err
	}
	if f.DataOffset, err = r.I32(); err != nil {
		return nil, err
	}
	f.Data = r.Rest()
	return f, nil
}

// StatusMessage is the common head of message 5000.
type StatusMessage struct {
	OriginalType uint16
	Status       uint8
	Tail         []byte
}

func ParseStatus(p []byte) (*StatusMessage, error) {
	r := NewReader(p)
	s := &StatusMessage{}
	var err error
	if s.OriginalType, err = r.U16(); err != nil {
		return nil, err
	}
	if s.Status, err = r.U8(); err != nil {
		return nil, err
	}
	s.Tail = r.Rest()
	return s, nil
}

func (s *StatusMessage) OK() bool { return s.Status == StatusACK }

// DownloadStatus is the tail of a RESPONSE to DOWNLOAD_REQUEST.
type DownloadStatus struct {
	Code        uint8
	MaxFileSize int32
}

func (s *StatusMessage) DownloadStatus() (*DownloadStatus, error) {
	r := NewReader(s.Tail)
	d := &DownloadStatus{}
	var err error
	if d.Code, err = r.U8(); err != nil {
		return nil, err
	}
	if d.MaxFileSize, err = r.I32(); err != nil {
		return nil, err
	}
	return d, nil
}

// UploadStatus is the tail of a RESPONSE to UPLOAD_REQUEST.
type UploadStatus struct {
	Code        uint8
	DataOffset  int32
	MaxFileSize int32
	CRCSeed     uint16
}

func (s *StatusMessage) UploadStatus() (*UploadStatus, error) {
	r := NewReader(s.Tail)
	u := &UploadStatus{}
	var err error
	if u.Code, err = r.U8(); err != nil {
		return nil, err
	}
	if u.DataOffset, err = r.I32(); err != nil {
		return nil, err
	}
	if u.MaxFileSize, err = r.I32(); err != nil {
		return nil, err
	}
	if u.CRCSeed, err = r.U16(); err != nil {
		return nil, err
	}
	return u, nil
}

// TransferStatus is the tail of a RESPONSE to FILE_TRANSFER_DATA.
type TransferStatus struct {
	Code       uint8
	DataOffset int32
}

const (
	TransferOK             uint8 = 0
	TransferResend         uint8 = 1
	TransferAbort          uint8 = 2
	TransferCRCMismatch    uint8 = 3
	TransferOffsetMismatch uint8 = 4
	TransferSyncPaused     uint8 = 5
)

func (s *StatusMessage) TransferStatus() (*TransferStatus, error) {
	r := NewReader(s.Tail)
	t := &TransferStatus{}
	var err error
	if t.Code, err = r.U8(); err != nil {
		return nil, err
	}
	if t.DataOffset, err = r.I32(); err != nil {
		return nil, err
	}
	return t, nil
}

// CreateFileStatus is the tail of a RESPONSE to CREATE_FILE.
type CreateFileStatus struct {
	Code       uint8
	FileIndex  uint16
	DataType   uint8
	SubType    uint8
	FileNumber uint16
}

func (s *StatusMessage) CreateFileStatus() (*CreateFileStatus, error) {
	r := NewReader(s.Tail)
	c := &CreateFileStatus{}
	var err error
	if c.Code, err = r.U8(); err != nil {
		return nil, err
	}
	if c.FileIndex, err = r.U16(); err != nil {
		return nil, err
	}
	if c.DataType, err = r.U8(); err != nil {
		return nil, err
	}
	if c.SubType, err = r.U8(); err != nil {
		return nil, err
	}
	if c.FileNumber, err = r.U16(); err != nil {
		return nil, err
	}
	return c, nil
}

// SupportedFileType is one entry of the RESPONSE to SUPPORTED_FILE_TYPES.
type SupportedFileType struct {
	DataType uint8
	SubType  uint8
	Name     string
}

func (s *StatusMessage) SupportedFileTypes() ([]SupportedFileType, error) {
	r := NewReader(s.Tail)
	n, err := r.U8()
	if err != nil {
		return nil, err
	}
	out := make([]SupportedFileType, 0, n)
	for range int(n) {
		var t SupportedFileType
		if t.DataType, err = r.U8(); err != nil {
			return out, nil
		}
		if t.SubType, err = r.U8(); err != nil {
			return out, nil
		}
		if t.Name, err = r.Str(); err != nil {
			return out, nil
		}
		out = append(out, t)
	}
	return out, nil
}

// ProtobufStatus is the tail of a RESPONSE to PROTOBUF_REQUEST/RESPONSE.
type ProtobufStatus struct {
	RequestID   uint16
	DataOffset  int32
	ChunkStatus uint8
	Code        uint8
}

func (s *StatusMessage) ProtobufStatus() (*ProtobufStatus, error) {
	r := NewReader(s.Tail)
	p := &ProtobufStatus{}
	var err error
	if p.RequestID, err = r.U16(); err != nil {
		return nil, err
	}
	if p.DataOffset, err = r.I32(); err != nil {
		return nil, err
	}
	if p.ChunkStatus, err = r.U8(); err != nil {
		return nil, err
	}
	if p.Code, err = r.U8(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *ProtobufStatus) OK() bool { return p.ChunkStatus == 0 && p.Code == 0 }

// ProtobufFrame is message 5043/5044.
type ProtobufFrame struct {
	RequestID  uint16
	DataOffset int32
	TotalLen   int32
	ChunkLen   int32
	Chunk      []byte
}

func ParseProtobufFrame(p []byte) (*ProtobufFrame, error) {
	r := NewReader(p)
	f := &ProtobufFrame{}
	var err error
	if f.RequestID, err = r.U16(); err != nil {
		return nil, err
	}
	if f.DataOffset, err = r.I32(); err != nil {
		return nil, err
	}
	if f.TotalLen, err = r.I32(); err != nil {
		return nil, err
	}
	if f.ChunkLen, err = r.I32(); err != nil {
		return nil, err
	}
	f.Chunk = r.Rest()
	return f, nil
}

func (f *ProtobufFrame) Chunked() bool  { return f.TotalLen != f.ChunkLen }
func (f *ProtobufFrame) Complete() bool { return f.DataOffset == 0 && !f.Chunked() }

// NotificationControl is message 5034.
type NotificationControl struct {
	Command        uint8
	NotificationID int32
	Action         uint8
	ActionText     string
	Attributes     []NotificationAttrRequest
	AppIdentifier  string
}

// NotificationAttrRequest is one requested attribute inside command 0.
type NotificationAttrRequest struct {
	ID        uint8
	MaxLength uint16
}

// Notification attribute ids.
const (
	NotifAttrAppIdentifier     uint8 = 0
	NotifAttrTitle             uint8 = 1
	NotifAttrSubtitle          uint8 = 2
	NotifAttrMessage           uint8 = 3
	NotifAttrMessageSize       uint8 = 4
	NotifAttrDate              uint8 = 5
	NotifAttrNegativeActionLbl uint8 = 7
	NotifAttrActions           uint8 = 127
	NotifAttrAttachments       uint8 = 128
)

// Notification control commands.
const (
	NotifCmdGetNotificationAttributes uint8 = 0
	NotifCmdGetAppAttributes          uint8 = 1
	NotifCmdPerformLegacyAction       uint8 = 2
	NotifCmdPerformAction             uint8 = 128
)

// Notification action codes.
const (
	ActionCustom1             uint8 = 1
	ActionReplyIncomingCall   uint8 = 94
	ActionReplyMessages       uint8 = 95
	ActionAcceptIncomingCall  uint8 = 96
	ActionRejectIncomingCall  uint8 = 97
	ActionDismissNotification uint8 = 98
	ActionBlockApplication    uint8 = 99
)

func ParseNotificationControl(p []byte) (*NotificationControl, error) {
	r := NewReader(p)
	n := &NotificationControl{}
	var err error
	if n.Command, err = r.U8(); err != nil {
		return nil, err
	}
	switch n.Command {
	case NotifCmdGetNotificationAttributes:
		if n.NotificationID, err = r.I32(); err != nil {
			return nil, err
		}
		for r.Remaining() > 0 {
			id, err := r.U8()
			if err != nil {
				break
			}
			attr := NotificationAttrRequest{ID: id}
			switch id {
			case NotifAttrTitle, NotifAttrSubtitle, NotifAttrMessage:
				if attr.MaxLength, err = r.U16(); err != nil {
					return n, nil
				}
			case NotifAttrActions:
				if attr.MaxLength, err = r.U16(); err != nil {
					return n, nil
				}
				if _, err = r.U8(); err != nil {
					return n, nil
				}
			}
			n.Attributes = append(n.Attributes, attr)
		}
	case NotifCmdPerformLegacyAction:
		if n.NotificationID, err = r.I32(); err != nil {
			return nil, err
		}
		if n.Action, err = r.U8(); err != nil {
			return nil, err
		}
	case NotifCmdPerformAction:
		if n.NotificationID, err = r.I32(); err != nil {
			return nil, err
		}
		if n.Action, err = r.U8(); err != nil {
			return nil, err
		}
		if r.Remaining() > 0 {
			n.ActionText, _ = r.CStr()
		}
	case NotifCmdGetAppAttributes:
		if n.AppIdentifier, err = r.CStr(); err != nil {
			return nil, err
		}
		for r.Remaining() > 0 {
			id, err := r.U8()
			if err != nil {
				break
			}
			n.Attributes = append(n.Attributes, NotificationAttrRequest{ID: id})
		}
	}
	return n, nil
}

// --------------------------------------------------------------- outbound ---

// GenericStatus builds a plain RESPONSE frame.
func GenericStatus(originalType uint16, status uint8) []byte {
	w := NewWriter()
	w.U16(originalType)
	w.U8(status)
	return BuildFrame(MsgResponse, w.Bytes())
}

// DeviceInformationResponse answers 5024. The watch refuses to consider the
// phone connected until it receives this.
func DeviceInformationResponse(in *DeviceInformation, btName, manufacturer, model string) []byte {
	if btName == "" {
		btName = "Pulse"
	}
	w := NewWriter()
	w.U16(MsgDeviceInformation)
	w.U8(StatusACK)
	w.U16(150)    // our protocol version
	w.U16(0xFFFF) // our product number: unknown
	w.I32(-1)     // our unit number: unknown
	w.U16(7791)   // our software version
	w.U16(0xFFFF) // our max packet size: unlimited
	w.Str(btName)
	w.Str(manufacturer)
	w.Str(model)
	var flags uint8
	if in != nil && in.ProtocolVersion/100 == 1 {
		flags = 1
	}
	w.U8(flags)
	return BuildFrame(MsgResponse, w.Bytes())
}

// ConfigurationResponse answers 5050 with our own capability bitfield. Note it
// reuses message id 5050 rather than RESPONSE.
func ConfigurationResponse() []byte {
	caps := OurCapabilities()
	w := NewWriter()
	w.U8(uint8(len(caps)))
	w.Raw(caps)
	return BuildFrame(MsgConfiguration, w.Bytes())
}

// SystemEvent builds 5030 with a single byte value.
func SystemEvent(eventType uint8, value uint8) []byte {
	w := NewWriter()
	w.U8(eventType)
	w.U8(value)
	return BuildFrame(MsgSystemEvent, w.Bytes())
}

// DeviceSetting is one entry of DEVICE_SETTINGS.
type DeviceSetting struct {
	ID   uint8
	Bool *bool
	Int  *int32
	Str  *string
}

func BoolSetting(id uint8, v bool) DeviceSetting  { return DeviceSetting{ID: id, Bool: &v} }
func IntSetting(id uint8, v int32) DeviceSetting  { return DeviceSetting{ID: id, Int: &v} }
func StrSetting(id uint8, v string) DeviceSetting { return DeviceSetting{ID: id, Str: &v} }

// DeviceSettings builds 5026.
func DeviceSettings(settings []DeviceSetting) []byte {
	w := NewWriter()
	w.U8(uint8(len(settings)))
	for _, s := range settings {
		w.U8(s.ID)
		switch {
		case s.Str != nil:
			w.Str(*s.Str)
		case s.Int != nil:
			w.U8(4)
			w.I32(*s.Int)
		case s.Bool != nil:
			w.U8(1)
			if *s.Bool {
				w.U8(1)
			} else {
				w.U8(0)
			}
		}
	}
	return BuildFrame(MsgDeviceSettings, w.Bytes())
}

// SupportedFileTypesRequest builds 5031 (empty payload).
func SupportedFileTypesRequest() []byte { return BuildFrame(MsgSupportedFileTypesRequest, nil) }

// Filter builds 5007. Upstream always uses filter type 3.
func Filter() []byte {
	w := NewWriter()
	w.U8(3)
	return BuildFrame(MsgFilter, w.Bytes())
}

// DownloadRequest builds 5002.
func DownloadRequest(fileIndex uint16, dataOffset int32, isNew bool, crcSeed uint16, dataSize int32) []byte {
	w := NewWriter()
	w.U16(fileIndex)
	w.I32(dataOffset)
	if isNew {
		w.U8(1)
	} else {
		w.U8(0)
	}
	w.U16(crcSeed)
	w.I32(dataSize)
	return BuildFrame(MsgDownloadRequest, w.Bytes())
}

// SetFileFlags builds 5008.
func SetFileFlags(fileIndex uint16, flags uint8) []byte {
	w := NewWriter()
	w.U16(fileIndex)
	w.U8(flags)
	return BuildFrame(MsgSetFileFlag, w.Bytes())
}

// FileTransferDataAck acknowledges a received chunk; this is the download flow
// control, with a window of exactly one chunk.
func FileTransferDataAck(nextOffset int32) []byte {
	w := NewWriter()
	w.U16(MsgFileTransferData)
	w.U8(StatusACK)
	w.U8(TransferOK)
	w.I32(nextOffset)
	return BuildFrame(MsgResponse, w.Bytes())
}

// CurrentTimeResponse answers 5052.
func CurrentTimeResponse(referenceID int32, unixSec int64, tzOffsetSec int32, nextTransitionEnd, nextTransitionStart int32) []byte {
	w := NewWriter()
	w.U16(MsgCurrentTimeRequest)
	w.U8(StatusACK)
	w.I32(referenceID)
	w.I32(int32(unixSec - GarminEpoch))
	w.I32(tzOffsetSec)
	w.I32(nextTransitionEnd)
	w.I32(nextTransitionStart)
	return BuildFrame(MsgResponse, w.Bytes())
}

// AuthNegotiationResponse answers 5101. There is no cryptographic
// authentication in this protocol: BLE bonding is the only pairing step.
func AuthNegotiationResponse(unk uint8, authFlags int32) []byte {
	w := NewWriter()
	w.U16(MsgAuthNegotiation)
	w.U8(StatusACK)
	w.U8(0) // GUESS_OK
	w.U8(unk)
	w.I32(authFlags)
	return BuildFrame(MsgResponse, w.Bytes())
}

// MusicControlCapabilitiesResponse answers 5042 advertising commands 0..8.
func MusicControlCapabilitiesResponse() []byte {
	w := NewWriter()
	w.U16(MsgMusicControlCapabilities)
	w.U8(StatusACK)
	w.U8(9)
	for i := range uint8(9) {
		w.U8(i)
	}
	return BuildFrame(MsgResponse, w.Bytes())
}

// NotificationSubscriptionStatus answers 5036.
func NotificationSubscriptionStatus(enabled bool, enableRaw uint8) []byte {
	w := NewWriter()
	w.U16(MsgNotificationSubscription)
	w.U8(StatusACK)
	if enabled {
		w.U8(0) // ENABLED
	} else {
		w.U8(1) // DISABLED
	}
	w.U8(enableRaw)
	w.U8(0)
	return BuildFrame(MsgResponse, w.Bytes())
}

// NotificationControlStatus answers 5034.
func NotificationControlStatus(ok bool) []byte {
	w := NewWriter()
	w.U16(MsgNotificationControl)
	w.U8(StatusACK)
	if ok {
		w.U8(0)
		w.U8(0) // NO_ERROR
	} else {
		w.U8(1)
		w.U8(160) // UNKNOWN_COMMAND
	}
	return BuildFrame(MsgResponse, w.Bytes())
}

// ProtobufStatusResponse acknowledges one protobuf fragment.
func ProtobufStatusResponse(originalType uint16, requestID uint16, dataOffset int32) []byte {
	w := NewWriter()
	w.U16(originalType)
	w.U8(StatusACK)
	w.U16(requestID)
	w.I32(dataOffset)
	w.U8(0) // KEPT
	w.U8(0) // NO_ERROR
	return BuildFrame(MsgResponse, w.Bytes())
}

// ProtobufFrames splits a serialised Smart message into 5043/5044 frames.
const ProtobufMaxChunk = 375

func ProtobufFrames(messageType uint16, requestID uint16, payload []byte) [][]byte {
	total := int32(len(payload))
	if total <= ProtobufMaxChunk {
		w := NewWriter()
		w.U16(requestID)
		w.I32(0)
		w.I32(total)
		w.I32(total)
		w.Raw(payload)
		return [][]byte{BuildFrame(messageType, w.Bytes())}
	}
	var out [][]byte
	for off := int32(0); off < total; off += ProtobufMaxChunk {
		end := off + ProtobufMaxChunk
		if end > total {
			end = total
		}
		w := NewWriter()
		w.U16(requestID)
		w.I32(off)
		w.I32(total)
		w.I32(end - off)
		w.Raw(payload[off:end])
		out = append(out, BuildFrame(messageType, w.Bytes()))
	}
	return out
}

// NotificationUpdate builds 5033.
const (
	NotifUpdateAdd    uint8 = 0
	NotifUpdateModify uint8 = 1
	NotifUpdateRemove uint8 = 2
)

// Notification categories.
const (
	CategoryOther              uint8 = 0
	CategoryIncomingCall       uint8 = 1
	CategoryMissedCall         uint8 = 2
	CategoryVoicemail          uint8 = 3
	CategorySocial             uint8 = 4
	CategorySchedule           uint8 = 5
	CategoryEmail              uint8 = 6
	CategoryNews               uint8 = 7
	CategoryHealthAndFitness   uint8 = 8
	CategoryBusinessAndFinance uint8 = 9
	CategoryLocation           uint8 = 10
	CategoryEntertainment      uint8 = 11
	CategorySMS                uint8 = 12
)

// NotificationUpdate builds a 5033 NOTIFICATION_UPDATE.
//
// Field order matters and is easy to get subtly wrong: count sits between the
// category and the id. Omitting it shifts the id and the phone flags by a
// byte, and the watch then quietly ignores the notification instead of asking
// for its attributes.
//
// count is how many notifications of this category are outstanding, matching
// upstream NotificationUpdateMessage.
func NotificationUpdate(updateType, categoryFlags, category, count uint8, id int32, phoneFlags uint8) []byte {
	w := NewWriter()
	w.U8(updateType)
	w.U8(categoryFlags)
	w.U8(category)
	w.U8(count)
	w.I32(id)
	w.U8(phoneFlags)
	return BuildFrame(MsgNotificationUpdate, w.Bytes())
}

// NotificationData builds one 5035 chunk. crc is the running checksum of every
// byte sent so far including this chunk.
func NotificationData(messageSize, crc, dataOffset uint16, chunk []byte) []byte {
	w := NewWriter()
	w.U16(messageSize)
	w.U16(crc)
	w.U16(dataOffset)
	w.Raw(chunk)
	return BuildFrame(MsgNotificationData, w.Bytes())
}

// MusicEntity ids and attributes for 5049.
const (
	EntityPlayer uint8 = 0
	EntityQueue  uint8 = 1
	EntityTrack  uint8 = 2

	PlayerName         uint8 = 0
	PlayerPlaybackInfo uint8 = 1
	PlayerVolume       uint8 = 2

	QueueIndex   uint8 = 0
	QueueCount   uint8 = 1
	QueueShuffle uint8 = 2
	QueueRepeat  uint8 = 3

	TrackArtist   uint8 = 0
	TrackAlbum    uint8 = 1
	TrackTitle    uint8 = 2
	TrackDuration uint8 = 3
)

// MusicEntityValue is one TLV of 5049.
type MusicEntityValue struct {
	Entity    uint8
	Attribute uint8
	Value     string
}

func MusicControlEntityUpdate(values []MusicEntityValue) []byte {
	w := NewWriter()
	for _, v := range values {
		b := []byte(v.Value)
		if len(b) > 252 {
			b = b[:252]
		}
		w.U8(uint8(3 + len(b)))
		w.U8(v.Entity)
		w.U8(v.Attribute)
		w.U8(0)
		w.Raw(b)
	}
	return BuildFrame(MsgMusicControlEntityUpdate, w.Bytes())
}

// CreateFile builds 5005.
func CreateFile(fileSize int32, dataType, subType uint8, nonce int64) []byte {
	w := NewWriter()
	w.I32(fileSize)
	w.U8(dataType)
	w.U8(subType)
	w.U16(0) // file index
	w.U8(0)  // reserved
	w.U8(0)  // sub type mask
	w.U16(65535)
	w.U16(0)
	w.I64(nonce)
	return BuildFrame(MsgCreateFile, w.Bytes())
}

// UploadRequest builds 5003.
func UploadRequest(fileIndex uint16, size, dataOffset int32, crcSeed uint16) []byte {
	w := NewWriter()
	w.U16(fileIndex)
	w.I32(size)
	w.I32(dataOffset)
	w.U16(crcSeed)
	return BuildFrame(MsgUploadRequest, w.Bytes())
}

// FileTransferDataChunk builds an outbound 5004 chunk.
func FileTransferDataChunk(crc uint16, dataOffset int32, chunk []byte) []byte {
	w := NewWriter()
	w.U8(0)
	w.U16(crc)
	w.I32(dataOffset)
	w.Raw(chunk)
	return BuildFrame(MsgFileTransferData, w.Bytes())
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func pad2(v int) string {
	if v < 10 {
		return "0" + itoa(v)
	}
	return itoa(v)
}
