package daemon

import "sync"

// Event is one server-sent event.
type Event struct {
	Kind string `json:"kind"`
	Data any    `json:"data,omitempty"`
}

// EventHub fans daemon events out to every connected SSE client. Slow clients
// are dropped rather than allowed to stall the session loop.
type EventHub struct {
	mu      sync.Mutex
	next    int
	clients map[int]chan Event
}

func NewEventHub() *EventHub {
	return &EventHub{clients: map[int]chan Event{}}
}

// Subscribe returns a channel of events and a cancel function.
func (h *EventHub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	h.mu.Lock()
	id := h.next
	h.next++
	h.clients[id] = ch
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if c, ok := h.clients[id]; ok {
			delete(h.clients, id)
			close(c)
		}
		h.mu.Unlock()
	}
}

// Publish delivers an event to every subscriber.
func (h *EventHub) Publish(kind string, data any) {
	ev := Event{Kind: kind, Data: data}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.clients {
		select {
		case ch <- ev:
		default:
		}
	}
}
