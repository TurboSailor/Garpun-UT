package garmin

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	pb "pulse/backend/internal/gproto/garmin"
)

// Helpers around the GdiSmartProto.Smart envelope.

func marshalSmart(m *pb.Smart) ([]byte, error) {
	b, err := proto.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("garmin: marshal smart: %w", err)
	}
	return b, nil
}

// ParseSmart decodes an inbound Smart message.
func ParseSmart(payload []byte) (*pb.Smart, error) {
	var m pb.Smart
	if err := proto.Unmarshal(payload, &m); err != nil {
		return nil, fmt.Errorf("garmin: parse smart: %w", err)
	}
	return &m, nil
}

// enableBatteryUpdates subscribes to watch battery reports. The percentage
// never arrives through BATTERY_STATUS (5023), only through this service.
func (s *Session) enableBatteryUpdates() {
	msg := &pb.Smart{
		DeviceStatusService: &pb.DeviceStatusService{
			RemoteDeviceBatteryStatusRequest: &pb.DeviceStatusService_RemoteDeviceBatteryStatusRequest{},
		},
	}
	b, err := marshalSmart(msg)
	if err != nil {
		s.log.Error("garmin: battery subscribe", "err", err)
		return
	}
	s.SendProtobuf(b)
}

// FindMyWatch makes the watch beep and vibrate for the given duration.
func (s *Session) FindMyWatch(timeout time.Duration) error {
	secs := int32(timeout.Seconds())
	if secs <= 0 {
		secs = 30
	}
	msg := &pb.Smart{
		FindMyWatchService: &pb.FindMyWatchService{
			FindRequest: &pb.FindMyWatchService_FindMyWatchRequest{Timeout: &secs},
		},
	}
	b, err := marshalSmart(msg)
	if err != nil {
		return err
	}
	s.SendProtobuf(b)
	return nil
}

// CancelFindMyWatch stops an active find alert.
func (s *Session) CancelFindMyWatch() error {
	msg := &pb.Smart{
		FindMyWatchService: &pb.FindMyWatchService{
			CancelRequest: &pb.FindMyWatchService_FindMyWatchCancelRequest{},
		},
	}
	b, err := marshalSmart(msg)
	if err != nil {
		return err
	}
	s.SendProtobuf(b)
	return nil
}

// BatteryLevelFromSmart extracts a battery percentage from an inbound message.
func BatteryLevelFromSmart(m *pb.Smart) (int32, bool) {
	ds := m.GetDeviceStatusService()
	if ds == nil {
		return 0, false
	}
	resp := ds.GetRemoteDeviceBatteryStatusResponse()
	if resp == nil {
		return 0, false
	}
	return resp.GetCurrentBatteryLevel(), true
}
