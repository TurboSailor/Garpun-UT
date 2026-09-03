package garmin

import (
	"time"

	"pulse/backend/internal/gfdi"
)

// notifMaxBlockSize is the chunk size upstream uses for NOTIFICATION_DATA.
const notifMaxBlockSize = 300

// NotificationAction is one action button the watch may offer for a
// notification.
type NotificationAction struct {
	Code  uint8  `json:"code"`
	Icon  uint8  `json:"icon"` // 0 bottom, 1 right, 2 left
	Label string `json:"label"`
}

// NotificationContent is everything the watch can ask about a notification.
type NotificationContent struct {
	ID                  int32                `json:"id"`
	AppIdentifier       string               `json:"appIdentifier"`
	Title               string               `json:"title"`
	Subtitle            string               `json:"subtitle"`
	Message             string               `json:"message"`
	Date                time.Time            `json:"date"`
	NegativeActionLabel string               `json:"negativeActionLabel"`
	Actions             []NotificationAction `json:"actions"`
	Category            uint8                `json:"category"`
}

// notificationTransfer tracks the chunked NOTIFICATION_DATA flow. The watch
// acknowledges each chunk before the next one may be sent.
type notificationTransfer struct {
	block  []byte
	offset int
	crc    uint16
}

// SendNotification pushes a notification to the watch. The watch then asks for
// the attributes it wants via NOTIFICATION_CONTROL.
//
// An id the watch already holds is announced as MODIFY, so a card that keeps
// updating (message counters, download progress) replaces the entry instead of
// stacking a new one the user has to clear by hand.
//
// ACTION_DECLINE is set unconditionally, as upstream getCategoryFlags does:
// without it the watch offers no dismiss entry, so the card cannot be cleared
// from the wrist at all.
func (s *Session) SendNotification(n NotificationContent) error {
	flags := uint8(0x02 | 0x10) // FOREGROUND | ACTION_DECLINE
	phoneFlags := uint8(0x02)   // NEW_ACTIONS

	s.notifyMu.Lock()
	if s.notifCounts == nil {
		s.notifCounts = map[uint8]map[int32]bool{}
	}
	byID := s.notifCounts[n.Category]
	if byID == nil {
		byID = map[int32]bool{}
		s.notifCounts[n.Category] = byID
	}
	update := gfdi.NotifUpdateAdd
	if byID[n.ID] {
		update = gfdi.NotifUpdateModify
	}
	byID[n.ID] = true
	count := clampCount(len(byID))
	s.notifyMu.Unlock()

	s.log.Debug("garmin: notification update out",
		"id", n.ID, "category", n.Category, "count", count, "update", update)
	return s.send(gfdi.NotificationUpdate(update, flags, n.Category, count, n.ID, phoneFlags))
}

// RemoveNotification tells the watch a notification is gone. Flags mirror
// upstream NotificationUpdateMessage: FOREGROUND|ACTION_DECLINE in the
// category byte, no phone flags, and the count of what is left in the
// category.
func (s *Session) RemoveNotification(id int32, category uint8) error {
	s.notifyMu.Lock()
	if byID := s.notifCounts[category]; byID != nil {
		delete(byID, id)
	}
	count := clampCount(len(s.notifCounts[category]))
	s.notifyMu.Unlock()

	return s.send(gfdi.NotificationUpdate(gfdi.NotifUpdateRemove, 0x02|0x10, category, count, id, 0x00))
}

// DropNotification removes a notification when only its id is known, which is
// the case for a dismissal that came from the watch itself. The category is
// resolved from what the watch is still holding, falling back to OTHER as
// upstream does for an id it no longer knows.
func (s *Session) DropNotification(id int32) error {
	category := gfdi.CategoryOther
	s.notifyMu.Lock()
	for cat, byID := range s.notifCounts {
		if byID[id] {
			category = cat
			break
		}
	}
	s.notifyMu.Unlock()
	return s.RemoveNotification(id, category)
}

// clampCount keeps the outstanding count inside the single byte the wire
// format allows.
func clampCount(n int) uint8 {
	if n > 0xFF {
		return 0xFF
	}
	if n < 0 {
		return 0
	}
	return uint8(n)
}

func (s *Session) onNotificationControl(f *gfdi.Frame) {
	ctrl, err := gfdi.ParseNotificationControl(f.Payload)
	if err != nil {
		s.send(gfdi.NotificationControlStatus(false))
		return
	}

	s.log.Debug("garmin: notification control in", "command", ctrl.Command, "id", ctrl.NotificationID)

	switch ctrl.Command {
	case gfdi.NotifCmdGetNotificationAttributes:
		s.send(gfdi.NotificationControlStatus(true))
		var content *NotificationContent
		if s.Hooks.NotificationContent != nil {
			content = s.Hooks.NotificationContent(ctrl.NotificationID)
		}
		block := buildNotificationBlock(ctrl.NotificationID, ctrl.Attributes, content)
		s.startNotificationTransfer(block)

	case gfdi.NotifCmdGetAppAttributes:
		s.send(gfdi.NotificationControlStatus(true))
		name := ctrl.AppIdentifier
		if s.Hooks.AppName != nil {
			if v := s.Hooks.AppName(ctrl.AppIdentifier); v != "" {
				name = v
			}
		}
		block := buildAppAttributeBlock(ctrl.AppIdentifier, ctrl.Attributes, name)
		s.startNotificationTransfer(block)

	case gfdi.NotifCmdPerformLegacyAction, gfdi.NotifCmdPerformAction:
		s.send(gfdi.NotificationControlStatus(true))
		s.emit(EventNotifAction, map[string]any{
			"id":     ctrl.NotificationID,
			"action": ctrl.Action,
			"text":   ctrl.ActionText,
		})

	default:
		s.send(gfdi.NotificationControlStatus(false))
	}
}

// buildNotificationBlock assembles the attribute payload the watch asked for.
// MESSAGE_SIZE always goes last, matching upstream ordering.
func buildNotificationBlock(id int32, attrs []gfdi.NotificationAttrRequest, c *NotificationContent) []byte {
	w := gfdi.NewWriter()
	w.U8(gfdi.NotifCmdGetNotificationAttributes)
	w.I32(id)
	if c == nil {
		c = &NotificationContent{ID: id}
	}

	var messageSize *gfdi.NotificationAttrRequest
	for i := range attrs {
		if attrs[i].ID == gfdi.NotifAttrMessageSize {
			messageSize = &attrs[i]
			continue
		}
		writeNotificationAttr(w, attrs[i], c)
	}
	if messageSize != nil {
		writeNotificationAttr(w, *messageSize, c)
	}
	return w.Bytes()
}

func writeNotificationAttr(w *gfdi.Writer, attr gfdi.NotificationAttrRequest, c *NotificationContent) {
	var value []byte
	switch attr.ID {
	case gfdi.NotifAttrAppIdentifier:
		value = []byte(c.AppIdentifier)
	case gfdi.NotifAttrTitle:
		value = []byte(c.Title)
	case gfdi.NotifAttrSubtitle:
		value = []byte(c.Subtitle)
	case gfdi.NotifAttrMessage:
		value = []byte(c.Message)
	case gfdi.NotifAttrMessageSize:
		value = []byte(itoa(len(c.Message)))
	case gfdi.NotifAttrDate:
		t := c.Date
		if t.IsZero() {
			t = time.Now()
		}
		value = []byte(t.Format("20060102T150405"))
	case gfdi.NotifAttrNegativeActionLbl:
		value = []byte(c.NegativeActionLabel)
	case gfdi.NotifAttrActions:
		value = encodeActions(c.Actions)
	case gfdi.NotifAttrAttachments:
		value = nil
	default:
		value = nil
	}
	if attr.MaxLength > 0 && len(value) > int(attr.MaxLength) {
		value = value[:attr.MaxLength]
	}
	w.U8(attr.ID)
	w.U16(uint16(len(value)))
	w.Raw(value)
}

func encodeActions(actions []NotificationAction) []byte {
	if len(actions) == 0 {
		return []byte{0, 0, 0, 0}
	}
	w := gfdi.NewWriter()
	w.U8(uint8(len(actions)))
	for _, a := range actions {
		label := []byte(a.Label)
		if len(label) > 255 {
			label = label[:255]
		}
		w.U8(a.Code)
		w.U8(a.Icon)
		w.U8(uint8(len(label)))
		w.Raw(label)
	}
	return w.Bytes()
}

func buildAppAttributeBlock(appID string, attrs []gfdi.NotificationAttrRequest, name string) []byte {
	w := gfdi.NewWriter()
	w.U8(gfdi.NotifCmdGetAppAttributes)
	w.CStr(appID)
	for _, a := range attrs {
		var value []byte
		if a.ID == 0 { // APP_NAME
			value = []byte(name)
		}
		w.U8(a.ID)
		w.U16(uint16(len(value)))
		w.Raw(value)
	}
	return w.Bytes()
}

func (s *Session) startNotificationTransfer(block []byte) {
	s.notifyMu.Lock()
	s.notifState = &notificationTransfer{block: block}
	s.notifyMu.Unlock()
	s.sendNotificationChunk()
}

func (s *Session) sendNotificationChunk() {
	s.notifyMu.Lock()
	t := s.notifState
	if t == nil || t.offset >= len(t.block) {
		s.notifState = nil
		s.notifyMu.Unlock()
		return
	}
	end := t.offset + notifMaxBlockSize
	if end > len(t.block) {
		end = len(t.block)
	}
	chunk := t.block[t.offset:end]
	t.crc = gfdi.CRC16(t.crc, chunk)
	frame := gfdi.NotificationData(uint16(len(t.block)), t.crc, uint16(t.offset), chunk)
	t.offset = end
	s.notifyMu.Unlock()
	s.send(frame)
}

func (s *Session) onNotificationDataStatus(st *gfdi.StatusMessage) {
	if len(st.Tail) < 1 {
		return
	}
	transferStatus := st.Tail[0]
	if !st.OK() || transferStatus != 0 {
		s.log.Warn("garmin: notification chunk rejected", "status", st.Status, "transfer", transferStatus)
		s.notifyMu.Lock()
		s.notifState = nil
		s.notifyMu.Unlock()
		return
	}
	s.sendNotificationChunk()
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
