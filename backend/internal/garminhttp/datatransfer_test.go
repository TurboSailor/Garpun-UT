package garminhttp

import (
	"bytes"
	"testing"

	pb "pulse/backend/internal/gproto/garmin"
)

func downloadRequest(id, offset, maxChunk uint32) *pb.DataTransferService {
	req := &pb.DataTransferService_DataDownloadRequest{Id: &id, Offset: &offset}
	if maxChunk > 0 {
		req.MaxChunkSize = &maxChunk
	}
	return &pb.DataTransferService{DataDownloadRequest: req}
}

func TestDataTransferChunking(t *testing.T) {
	h := testHandler(t, Options{})
	payload := bytes.Repeat([]byte("0123456789"), 10) // 100 bytes

	sent := 0
	id := h.transfers.register(payload, func() { sent++ })

	var assembled []byte
	for offset := uint32(0); offset < uint32(len(payload)); offset += 30 {
		smart := h.handleDataTransfer(downloadRequest(id, offset, 30))
		resp := smart.GetDataTransferService().GetDataDownloadResponse()
		if resp.GetStatus() != pb.DataTransferService_SUCCESS {
			t.Fatalf("offset %d: status = %v", offset, resp.GetStatus())
		}
		if resp.GetId() != id || resp.GetOffset() != offset {
			t.Fatalf("echoed id/offset wrong: %d/%d", resp.GetId(), resp.GetOffset())
		}
		want := 30
		if remaining := len(payload) - int(offset); remaining < want {
			want = remaining
		}
		if len(resp.GetPayload()) != want {
			t.Fatalf("offset %d: chunk is %d bytes, want %d", offset, len(resp.GetPayload()), want)
		}
		assembled = append(assembled, resp.GetPayload()...)
	}
	if !bytes.Equal(assembled, payload) {
		t.Fatal("reassembled payload differs from the original")
	}
	if sent != 1 {
		t.Fatalf("completion hook fired %d times, want 1", sent)
	}
}

func TestDataTransferWholePayloadWithoutMaxChunkSize(t *testing.T) {
	h := testHandler(t, Options{})
	payload := []byte("short body")
	id := h.transfers.register(payload, nil)

	smart := h.handleDataTransfer(downloadRequest(id, 0, 0))
	resp := smart.GetDataTransferService().GetDataDownloadResponse()
	if resp.GetStatus() != pb.DataTransferService_SUCCESS {
		t.Fatalf("status = %v", resp.GetStatus())
	}
	if !bytes.Equal(resp.GetPayload(), payload) {
		t.Fatalf("payload = %q", resp.GetPayload())
	}
}

func TestDataTransferInvalidID(t *testing.T) {
	h := testHandler(t, Options{})
	id := h.transfers.register([]byte("body"), nil)

	smart := h.handleDataTransfer(downloadRequest(id+1000, 0, 10))
	resp := smart.GetDataTransferService().GetDataDownloadResponse()
	if resp.GetStatus() != pb.DataTransferService_INVALID_ID {
		t.Fatalf("status = %v, want INVALID_ID", resp.GetStatus())
	}
	if len(resp.GetPayload()) != 0 {
		t.Fatal("error response must not carry a payload")
	}
}

func TestDataTransferInvalidOffset(t *testing.T) {
	h := testHandler(t, Options{})
	payload := []byte("0123456789")
	id := h.transfers.register(payload, nil)

	for _, offset := range []uint32{uint32(len(payload)), uint32(len(payload)) + 1, 1 << 20} {
		smart := h.handleDataTransfer(downloadRequest(id, offset, 4))
		resp := smart.GetDataTransferService().GetDataDownloadResponse()
		if resp.GetStatus() != pb.DataTransferService_INVALID_OFFSET {
			t.Fatalf("offset %d: status = %v, want INVALID_OFFSET", offset, resp.GetStatus())
		}
		if resp.GetOffset() != offset {
			t.Fatalf("offset echo = %d, want %d", resp.GetOffset(), offset)
		}
	}
}

func TestDataTransferCompletionNeedsFullCoverage(t *testing.T) {
	h := testHandler(t, Options{})
	sent := 0
	id := h.transfers.register(bytes.Repeat([]byte{0xaa}, 20), func() { sent++ })

	// Skipping the head leaves a gap, so the hook must stay silent.
	h.handleDataTransfer(downloadRequest(id, 10, 10))
	if sent != 0 {
		t.Fatalf("hook fired with a gap in the served ranges")
	}
	h.handleDataTransfer(downloadRequest(id, 0, 10))
	if sent != 1 {
		t.Fatalf("hook fired %d times after full coverage, want 1", sent)
	}
	// Retransmissions must not fire it again.
	h.handleDataTransfer(downloadRequest(id, 0, 10))
	if sent != 1 {
		t.Fatalf("hook fired again on retransmission")
	}
}

func TestDataTransferEvictsOldestPayloads(t *testing.T) {
	h := testHandler(t, Options{})
	ids := make([]uint32, 0, maxRetainedTransfers+2)
	for i := range maxRetainedTransfers + 2 {
		ids = append(ids, h.transfers.register([]byte{byte(i)}, nil))
	}

	smart := h.handleDataTransfer(downloadRequest(ids[0], 0, 1))
	if got := smart.GetDataTransferService().GetDataDownloadResponse().GetStatus(); got != pb.DataTransferService_INVALID_ID {
		t.Fatalf("oldest payload status = %v, want INVALID_ID", got)
	}
	smart = h.handleDataTransfer(downloadRequest(ids[len(ids)-1], 0, 1))
	if got := smart.GetDataTransferService().GetDataDownloadResponse().GetStatus(); got != pb.DataTransferService_SUCCESS {
		t.Fatalf("newest payload status = %v, want SUCCESS", got)
	}
}

func TestDataTransferUnsupportedRequest(t *testing.T) {
	h := testHandler(t, Options{})
	if smart := h.handleDataTransfer(&pb.DataTransferService{}); smart != nil {
		t.Fatalf("expected no reply, got %v", smart)
	}
}
