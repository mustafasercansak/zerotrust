package serviceaccount

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// EventHub fans out a broadcast signal to all connected SSE subscribers.
type EventHub struct {
	mu      sync.Mutex
	clients map[chan struct{}]struct{}
}

func NewEventHub() *EventHub {
	return &EventHub{clients: make(map[chan struct{}]struct{})}
}

// Subscribe registers a new SSE client and returns its channel plus an
// unsubscribe function that must be deferred by the caller.
func (h *EventHub) Subscribe() (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
		close(ch)
	}
}

// Broadcast sends a signal to every connected subscriber; slow clients are skipped.
func (h *EventHub) Broadcast() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// ListenForChanges connects to PostgreSQL and listens on the
// "service_accounts_changed" channel, broadcasting on every notification.
// It reconnects automatically on failure until ctx is cancelled.
func (h *EventHub) ListenForChanges(ctx context.Context, connStr string) {
	for {
		if ctx.Err() != nil {
			return
		}
		if err := h.listenOnce(ctx, connStr); err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("pg listener disconnected, retrying", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
		}
	}
}

func (h *EventHub) listenOnce(ctx context.Context, connStr string) error {
	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := conn.Close(closeCtx); err != nil {
			slog.Warn("pg listener close failed", "error", err)
		}
	}()

	if _, err := conn.Exec(ctx, "LISTEN service_accounts_changed"); err != nil {
		return err
	}

	for {
		if _, err := conn.WaitForNotification(ctx); err != nil {
			return err
		}
		h.Broadcast()
	}
}
