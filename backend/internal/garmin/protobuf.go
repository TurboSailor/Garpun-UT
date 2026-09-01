package garmin

import (
	"fmt"
	"time"

	"pulse/backend/internal/gfdi"
)

// The protobuf layer carries GdiSmartProto.Smart messages inside GFDI 5043
// (request) and 5044 (response) frames, fragmented at 375 bytes per frame and
// reassembled per request id.

// SendProtobuf transmits a serialised Smart message as a request and returns
// the assigned request id.
func (s *Session) SendProtobuf(payload []byte) uint16 {
	return s.sendProtobufAs(gfdi.MsgProtobufRequest, s.nextRequestID(), payload)
}

// SendProtobufResponse answers an inbound request with the same request id.
func (s *Session) SendProtobufResponse(requestID uint16, payload []byte) {
	s.sendProtobufAs(gfdi.MsgProtobufResponse, requestID, payload)
}

// RequestProtobuf sends a request and waits for the matching response payload.
func (s *Session) RequestProtobuf(payload []byte, timeout time.Duration) ([]byte, error) {
	id := s.nextRequestID()
	ch := make(chan []byte, 1)
	s.protoMu.Lock()
	s.protoWait[id] = ch
	s.protoMu.Unlock()
	defer func() {
		s.protoMu.Lock()
		delete(s.protoWait, id)
		s.protoMu.Unlock()
	}()

	s.sendProtobufAs(gfdi.MsgProtobufRequest, id, payload)
	select {
	case reply := <-ch:
		return reply, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("garmin: protobuf request %d timed out", id)
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

func (s *Session) nextRequestID() uint16 {
	s.protoMu.Lock()
	defer s.protoMu.Unlock()
	s.protoNext++ // natural uint16 wraparound matches (last + 1) % 65536
	return s.protoNext
}

func (s *Session) sendProtobufAs(messageType, requestID uint16, payload []byte) uint16 {
	frames := gfdi.ProtobufFrames(messageType, requestID, payload)
	if len(frames) > 1 {
		// Only the first fragment goes out now; the watch paces the rest with
		// per-chunk acknowledgements.
		s.protoMu.Lock()
		s.protoOut[requestID] = payload
		s.protoMu.Unlock()
		s.send(frames[0])
		return requestID
	}
	s.send(frames[0])
	return requestID
}

func (s *Session) onProtobuf(f *gfdi.Frame) {
	pf, err := gfdi.ParseProtobufFrame(f.Payload)
	if err != nil {
		s.send(gfdi.GenericStatus(f.Type, gfdi.StatusDecodeError))
		return
	}

	if pf.Complete() {
		s.send(gfdi.GenericStatus(f.Type, gfdi.StatusACK))
		s.dispatchProtobuf(f.Type, pf.RequestID, pf.Chunk)
		return
	}

	s.send(gfdi.ProtobufStatusResponse(f.Type, pf.RequestID, pf.DataOffset))

	s.protoMu.Lock()
	buf := s.protoIn[pf.RequestID]
	switch {
	case pf.DataOffset == 0:
		buf = append([]byte(nil), pf.Chunk...)
	case int(pf.DataOffset) == len(buf):
		buf = append(buf, pf.Chunk...)
	default:
		s.protoMu.Unlock()
		s.log.Warn("garmin: protobuf fragment out of order",
			"requestId", pf.RequestID, "offset", pf.DataOffset, "have", len(buf))
		return
	}
	s.protoIn[pf.RequestID] = buf
	done := int32(len(buf)) >= pf.TotalLen
	if done {
		delete(s.protoIn, pf.RequestID)
	}
	s.protoMu.Unlock()

	if done {
		s.dispatchProtobuf(f.Type, pf.RequestID, buf)
	}
}

func (s *Session) dispatchProtobuf(messageType, requestID uint16, payload []byte) {
	if messageType == gfdi.MsgProtobufResponse {
		s.protoMu.Lock()
		ch, ok := s.protoWait[requestID]
		s.protoMu.Unlock()
		if ok {
			select {
			case ch <- payload:
			default:
			}
			return
		}
	}
	if s.Hooks.Protobuf == nil {
		return
	}
	reply, err := s.Hooks.Protobuf(requestID, payload)
	if err != nil {
		s.log.Warn("garmin: protobuf handler failed", "err", err, "requestId", requestID)
		return
	}
	if len(reply) > 0 {
		s.SendProtobufResponse(requestID, reply)
	}
}

func (s *Session) onProtobufStatus(st *gfdi.StatusMessage) {
	ps, err := st.ProtobufStatus()
	if err != nil || !st.OK() {
		return
	}
	s.protoMu.Lock()
	payload, ok := s.protoOut[ps.RequestID]
	s.protoMu.Unlock()
	if !ok {
		return
	}
	if !ps.OK() {
		s.log.Warn("garmin: protobuf chunk rejected", "requestId", ps.RequestID, "code", ps.Code)
		s.protoMu.Lock()
		delete(s.protoOut, ps.RequestID)
		s.protoMu.Unlock()
		return
	}

	start := int(ps.DataOffset) + gfdi.ProtobufMaxChunk
	if start >= len(payload) {
		s.protoMu.Lock()
		delete(s.protoOut, ps.RequestID)
		s.protoMu.Unlock()
		return
	}
	end := start + gfdi.ProtobufMaxChunk
	if end > len(payload) {
		end = len(payload)
	}
	w := gfdi.NewWriter()
	w.U16(ps.RequestID)
	w.I32(int32(start))
	w.I32(int32(len(payload)))
	w.I32(int32(end - start))
	w.Raw(payload[start:end])
	s.send(gfdi.BuildFrame(gfdi.MsgProtobufRequest, w.Bytes()))
}
