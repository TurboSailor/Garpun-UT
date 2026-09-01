package garminhttp

import (
	"math"
	"time"

	"google.golang.org/protobuf/proto"

	pb "pulse/backend/internal/gproto/garmin"
)

// garminEpoch is the FIT/Garmin epoch in Unix seconds (1989-12-31T00:00:00Z).
const garminEpoch = 631065600

// semicircleScale converts degrees to the int32 semicircles the watch uses.
const semicircleScale = float64(1<<31) / 180.0

// --- CoreService -----------------------------------------------------------

func (h *Handler) handleCore(svc *pb.CoreService) *pb.Smart {
	if resp := svc.GetSyncResponse(); resp != nil {
		h.log.Info("garminhttp: core sync status", "status", resp.GetStatus().String())
		return nil
	}

	if svc.GetGetLocationRequest() != nil {
		return h.locationResponse()
	}

	if req := svc.GetLocationUpdatedSetEnabledRequest(); req != nil {
		return h.locationUpdatedResponse(req)
	}

	h.log.Warn("garminhttp: unknown core service request")
	return nil
}

func (h *Handler) locationResponse() *pb.Smart {
	status := pb.CoreService_GetLocationResponse_NO_VALID_LOCATION
	out := &pb.CoreService_GetLocationResponse{}

	var (
		lat, lon float64
		ok       bool
	)
	if h.opts.Location != nil {
		lat, lon, ok = h.opts.Location.Location()
	}
	if ok && (lat != 0 || lon != 0) {
		status = pb.CoreService_GetLocationResponse_OK
		out.LocationData = locationData(lat, lon, pb.CoreService_GENERAL_LOCATION)
	} else {
		h.log.Warn("garminhttp: location requested but none available")
	}
	out.Status = &status

	return &pb.Smart{CoreService: &pb.CoreService{GetLocationResponse: out}}
}

// locationUpdatedResponse declines every realtime stream: the daemon has no GPS
// feed to push, so the watch has to use its own receiver.
func (h *Handler) locationUpdatedResponse(req *pb.CoreService_LocationUpdatedSetEnabledRequest) *pb.Smart {
	status := pb.CoreService_LocationUpdatedSetEnabledResponse_OK
	out := &pb.CoreService_LocationUpdatedSetEnabledResponse{Status: &status}

	if req.GetEnabled() {
		for _, r := range req.GetRequests() {
			requested := r.GetRequested()
			requestedStatus := pb.CoreService_LocationUpdatedSetEnabledResponse_Requested_KO
			out.Requests = append(out.Requests, &pb.CoreService_LocationUpdatedSetEnabledResponse_Requested{
				Requested: &requested,
				Status:    &requestedStatus,
			})
		}
	}
	h.log.Info("garminhttp: location updates declined", "enabled", req.GetEnabled(), "requests", len(req.GetRequests()))

	return &pb.Smart{CoreService: &pb.CoreService{LocationUpdatedSetEnabledResponse: out}}
}

func locationData(lat, lon float64, kind pb.CoreService_DataType) *pb.CoreService_LocationData {
	latSemi := int32(math.Round(lat * semicircleScale))
	lonSemi := int32(math.Round(lon * semicircleScale))
	timestamp := uint32(time.Now().Unix() - garminEpoch)
	altitude := float32(0)
	accuracy := float32(0)
	bearing := float32(0)
	speed := float32(0)
	return &pb.CoreService_LocationData{
		Position:     &pb.CoreService_LatLon{Lat: &latSemi, Lon: &lonSemi},
		Altitude:     &altitude,
		Timestamp:    &timestamp,
		HAccuracy:    &accuracy,
		VAccuracy:    &accuracy,
		PositionType: &kind,
		Bearing:      &bearing,
		Speed:        &speed,
	}
}

// --- DeviceStatusService ---------------------------------------------------

func (h *Handler) handleDeviceStatus(svc *pb.DeviceStatusService) {
	if resp := svc.GetRemoteDeviceBatteryStatusResponse(); resp != nil {
		level := resp.GetCurrentBatteryLevel()
		h.log.Info("garminhttp: watch battery", "status", resp.GetStatus().String(), "level", level)
		if h.opts.OnBattery != nil {
			h.opts.OnBattery(level)
		}
		return
	}
	if resp := svc.GetActivityStatusResponse(); resp != nil {
		h.log.Info("garminhttp: activity status", "status", resp.GetStatus().String())
		return
	}
	if svc.GetRemoteDeviceBatteryStatusChangedNotification() != nil {
		// The watch only signals the change; the level arrives in the next
		// unsolicited response.
		h.log.Debug("garminhttp: battery status changed notification")
		return
	}
	h.log.Warn("garminhttp: unknown device status message")
}

// --- FindMyWatchService ----------------------------------------------------

func (h *Handler) handleFindMyWatch(svc *pb.FindMyWatchService) {
	switch {
	case svc.GetCancelRequest() != nil:
		h.log.Info("garminhttp: watch found, alert cancelled from the watch")
	case svc.GetFindResponse() != nil:
		h.log.Debug("garminhttp: find my watch response", "status", svc.GetFindResponse().GetStatus().String())
	case svc.GetCancelResponse() != nil:
		h.log.Debug("garminhttp: find my watch cancel response", "status", svc.GetCancelResponse().GetStatus().String())
	default:
		h.log.Warn("garminhttp: unknown find my watch message")
	}
}

// --- SmsNotificationService ------------------------------------------------

func (h *Handler) handleSms(svc *pb.SmsNotificationService) *pb.Smart {
	if req := svc.GetSmsCannedListRequest(); req != nil {
		return h.cannedListResponse(req.GetRequestedTypes())
	}
	if req := svc.GetSmsSendMessageRequest(); req != nil {
		// Sending SMS is not wired on Ubuntu Touch; tell the watch it failed
		// instead of silently dropping the message.
		h.log.Warn("garminhttp: sms send requested but unsupported", "to", req.GetReceiverNumber())
		status := pb.SmsNotificationService_GENERIC_ERROR
		return &pb.Smart{SmsNotificationService: &pb.SmsNotificationService{
			SmsSendMessageResponse: &pb.SmsNotificationService_SmsSendMessageResponse{Status: &status},
		}}
	}
	h.log.Warn("garminhttp: unknown sms notification message")
	return nil
}

func (h *Handler) cannedListResponse(types []pb.SmsNotificationService_CannedListType) *pb.Smart {
	status := pb.SmsNotificationService_SUCCESS
	out := &pb.SmsNotificationService_SmsCannedListResponse{}

	if len(h.opts.CannedReplies) == 0 {
		h.log.Warn("garminhttp: canned list requested but none configured")
		status = pb.SmsNotificationService_GENERIC_ERROR
	} else {
		for _, t := range types {
			listType := t
			out.Lists = append(out.Lists, &pb.SmsNotificationService_SmsCannedList{
				Type:     &listType,
				Response: h.opts.CannedReplies,
			})
		}
		if len(out.Lists) == 0 {
			status = pb.SmsNotificationService_GENERIC_ERROR
		}
	}
	out.Status = &status

	return &pb.Smart{SmsNotificationService: &pb.SmsNotificationService{SmsCannedListResponse: out}}
}

// --- SettingsService -------------------------------------------------------

// handleSettings forwards realtime settings payloads to the daemon. These are
// always replies to requests we sent, so no answer goes back to the watch.
func (h *Handler) handleSettings(svc *pb.SettingsService) {
	report := func(kind string, payload []byte) {
		h.log.Debug("garminhttp: settings payload", "kind", kind, "bytes", len(payload))
		if h.opts.OnSettings != nil {
			h.opts.OnSettings(kind, payload)
		}
	}

	if resp := svc.GetDefinitionResponse(); resp != nil {
		report("definition", marshalOrNil(resp.GetDefinition()))
	}
	if resp := svc.GetStateResponse(); resp != nil {
		report("state", marshalOrNil(resp.GetState()))
	}
	if resp := svc.GetChangeResponse(); resp != nil {
		report("change", marshalOrNil(resp))
	}
	if svc.GetInitResponse() != nil {
		h.log.Debug("garminhttp: realtime settings ready")
	}
}

// marshalOrNil serialises a settings sub-message for the daemon, which parses
// it again against the realtime settings schema.
func marshalOrNil(m proto.Message) []byte {
	if m == nil {
		return nil
	}
	b, err := proto.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}

// --- NotificationsService --------------------------------------------------

// handleNotifications refuses notification pictures: the daemon does not keep
// notification attachments.
func (h *Handler) handleNotifications(svc *pb.NotificationsService) *pb.Smart {
	if req := svc.GetPictureRequest(); req != nil {
		h.log.Debug("garminhttp: notification picture request declined", "notificationId", req.GetNotificationId())
		return nil
	}
	h.log.Warn("garminhttp: unknown notifications service message")
	return nil
}

// --- CalendarService -------------------------------------------------------

// handleCalendar replies with an empty, successful event list: Pulse does not
// read the Ubuntu Touch calendar.
func (h *Handler) handleCalendar(svc *pb.CalendarService) *pb.Smart {
	status := pb.CalendarService_CalendarServiceResponse_OK
	if svc.GetCalendarRequest() == nil {
		h.log.Warn("garminhttp: unknown calendar service message")
		status = pb.CalendarService_CalendarServiceResponse_UNKNOWN_RESPONSE_STATUS
	}
	return &pb.Smart{CalendarService: &pb.CalendarService{
		CalendarResponse: &pb.CalendarService_CalendarServiceResponse{Status: &status},
	}}
}

// --- AuthenticationService -------------------------------------------------

// handleAuthentication hands out fake OAuth keys, the protobuf counterpart of
// the fake HTTP OAuth endpoints.
func (h *Handler) handleAuthentication(svc *pb.AuthenticationService) *pb.Smart {
	if svc.GetOauthRequest() == nil {
		h.log.Warn("garminhttp: unknown authentication service message")
		return nil
	}
	consumerKey := randomUUID()
	consumerSecret := randomAlnum(35)
	oauthToken := randomUUID()
	oauthSecret := randomAlnum(35)
	unk2 := uint32(0)

	h.log.Debug("garminhttp: answering oauth key request")
	return &pb.Smart{AuthenticationService: &pb.AuthenticationService{
		OauthResponse: &pb.OAuthResponse{
			Keys: &pb.OAuthKeys{
				ConsumerKey:    &consumerKey,
				ConsumerSecret: &consumerSecret,
				OauthToken:     &oauthToken,
				OauthSecret:    &oauthSecret,
			},
			Unk2: &unk2,
		},
	}}
}
