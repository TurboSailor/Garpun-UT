package garminhttp

import (
	"math"
	"math/rand/v2"
	"sort"
	"sync"

	pb "pulse/backend/internal/gproto/garmin"
)

// maxRetainedTransfers bounds how many completed payloads stay around for
// retransmission before the oldest ones are dropped.
const maxRetainedTransfers = 8

// dataTransfer serves large HTTP bodies to the watch in chunks: the response
// only carries an id and a size, then the watch pulls DataDownloadRequests.
type dataTransfer struct {
	mu     sync.Mutex
	nextID uint32
	order  []uint32
	items  map[uint32]*transferItem
}

type transferItem struct {
	data []byte
	// served records byte ranges already handed out, so the completion hook
	// can fire once the watch has requested every byte.
	served []span
	done   bool
	onSent func()
}

type span struct {
	start int
	end   int
}

func newDataTransfer() *dataTransfer {
	return &dataTransfer{
		// Random start like the reference implementation: ids must not repeat
		// across reconnects while the watch still holds an old one.
		nextID: uint32(rand.Int32N(math.MaxInt32 / 2)),
		items:  make(map[uint32]*transferItem),
	}
}

// register stores a payload and returns the id the watch will ask for.
func (d *dataTransfer) register(data []byte, onSent func()) uint32 {
	d.mu.Lock()
	defer d.mu.Unlock()

	id := d.nextID
	d.nextID++
	d.items[id] = &transferItem{data: data, onSent: onSent}
	d.order = append(d.order, id)

	for len(d.order) > maxRetainedTransfers {
		oldest := d.order[0]
		d.order = d.order[1:]
		delete(d.items, oldest)
	}
	return id
}

// chunk returns the slice at offset, capped to maxChunkSize. ok is false when
// the offset is out of range; found is false when the id is unknown.
func (d *dataTransfer) chunk(id, offset, maxChunkSize uint32) (data []byte, found, ok bool) {
	d.mu.Lock()
	item, found := d.items[id]
	if !found {
		d.mu.Unlock()
		return nil, false, false
	}
	if offset >= uint32(len(item.data)) {
		d.mu.Unlock()
		return nil, true, false
	}
	end := uint64(offset) + uint64(maxChunkSize)
	if maxChunkSize == 0 || end > uint64(len(item.data)) {
		end = uint64(len(item.data))
	}
	data = item.data[offset:end]
	fire := item.markServed(int(offset), int(end))
	onSent := item.onSent
	d.mu.Unlock()

	if fire && onSent != nil {
		onSent()
	}
	return data, true, true
}

// markServed merges a served range and reports whether the payload just became
// fully covered. Caller holds the registry lock.
func (t *transferItem) markServed(start, end int) bool {
	t.served = append(t.served, span{start, end})
	if t.done {
		return false
	}
	sort.Slice(t.served, func(i, j int) bool { return t.served[i].start < t.served[j].start })
	reach := 0
	for _, s := range t.served {
		if s.start > reach {
			return false
		}
		if s.end > reach {
			reach = s.end
		}
	}
	if reach < len(t.data) {
		return false
	}
	t.done = true
	return true
}

// handle answers a DataTransferService request.
func (h *Handler) handleDataTransfer(svc *pb.DataTransferService) *pb.Smart {
	req := svc.GetDataDownloadRequest()
	if req == nil {
		h.log.Warn("garminhttp: unsupported data transfer request")
		return nil
	}
	id := req.GetId()
	offset := req.GetOffset()
	maxChunk := req.GetMaxChunkSize()

	status := pb.DataTransferService_SUCCESS
	chunk, found, ok := h.transfers.chunk(id, offset, maxChunk)
	switch {
	case !found:
		h.log.Warn("garminhttp: data transfer invalid id", "id", id)
		status = pb.DataTransferService_INVALID_ID
	case !ok:
		h.log.Warn("garminhttp: data transfer invalid offset", "id", id, "offset", offset)
		status = pb.DataTransferService_INVALID_OFFSET
	}

	resp := &pb.DataTransferService_DataDownloadResponse{
		Status: &status,
		Id:     &id,
		Offset: &offset,
	}
	if status == pb.DataTransferService_SUCCESS {
		resp.Payload = chunk
	}
	return &pb.Smart{
		DataTransferService: &pb.DataTransferService{DataDownloadResponse: resp},
	}
}
