package session

import "sync"

type Event struct {
	Kind        string
	SessionHash string
}

// EventHub fans out session-change signals to connected clients for each user.
type EventHub struct {
	mu      sync.Mutex
	clients map[string]map[chan Event]struct{}
}

func NewEventHub() *EventHub {
	return &EventHub{clients: make(map[string]map[chan Event]struct{})}
}

func (h *EventHub) Subscribe(userID string) (chan Event, func()) {
	ch := make(chan Event, 1)
	h.mu.Lock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[chan Event]struct{})
	}
	h.clients[userID][ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		delete(h.clients[userID], ch)
		if len(h.clients[userID]) == 0 {
			delete(h.clients, userID)
		}
		h.mu.Unlock()
		close(ch)
	}
}

func (h *EventHub) Broadcast(userID string) {
	h.BroadcastEvent(userID, Event{Kind: "change"})
}

func (h *EventHub) BroadcastRevoked(userID, sessionHash string) {
	h.BroadcastEvent(userID, Event{Kind: "revoked", SessionHash: sessionHash})
}

// BroadcastRevokedAll signals every connected client for userID that their
// session was terminated (token-reuse response, admin force-logout, etc.).
func (h *EventHub) BroadcastRevokedAll(userID string) {
	h.BroadcastEvent(userID, Event{Kind: "revoked_all"})
}

// BroadcastRevokedOthers signals that all sessions EXCEPT keepHash were
// revoked. The client whose hash matches keepHash gets a plain "change"
// (its session is still alive); every other client gets "revoked".
func (h *EventHub) BroadcastRevokedOthers(userID, keepHash string) {
	h.BroadcastEvent(userID, Event{Kind: "revoked_others", SessionHash: keepHash})
}

func (h *EventHub) BroadcastEvent(userID string, event Event) {
	if h == nil || userID == "" {
		return
	}
	if event.Kind == "" {
		event.Kind = "change"
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients[userID] {
		select {
		case ch <- event:
		default:
		}
	}
}
