package session

import (
	"testing"
	"time"
)

func TestEventHubSubscribeAndUnsubscribe(t *testing.T) {
	hub := NewEventHub()
	ch, unsub := hub.Subscribe("u1")

	hub.mu.Lock()
	if len(hub.clients["u1"]) != 1 {
		hub.mu.Unlock()
		t.Fatalf("subscriber count=%d want=1", len(hub.clients["u1"]))
	}
	hub.mu.Unlock()

	unsub()

	hub.mu.Lock()
	if _, ok := hub.clients["u1"]; ok {
		hub.mu.Unlock()
		t.Fatal("expected user subscriptions to be removed after unsubscribe")
	}
	hub.mu.Unlock()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed after unsubscribe")
		}
	default:
		t.Fatal("expected closed channel to be readable immediately")
	}
}

func TestEventHubBroadcastHelpers(t *testing.T) {
	hub := NewEventHub()
	ch, unsub := hub.Subscribe("u1")
	defer unsub()

	tests := []struct {
		name string
		send func()
		want Event
	}{
		{name: "change", send: func() { hub.Broadcast("u1") }, want: Event{Kind: "change"}},
		{name: "revoked", send: func() { hub.BroadcastRevoked("u1", "h1") }, want: Event{Kind: "revoked", SessionHash: "h1"}},
		{name: "revoked all", send: func() { hub.BroadcastRevokedAll("u1") }, want: Event{Kind: "revoked_all"}},
		{name: "revoked others", send: func() { hub.BroadcastRevokedOthers("u1", "keep") }, want: Event{Kind: "revoked_others", SessionHash: "keep"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.send()
			select {
			case got := <-ch:
				if got != tt.want {
					t.Fatalf("event=%+v want=%+v", got, tt.want)
				}
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for event")
			}
		})
	}
}

func TestEventHubBroadcastEventDefaultsAndGuards(t *testing.T) {
	var nilHub *EventHub
	nilHub.BroadcastEvent("u1", Event{})

	hub := NewEventHub()
	ch, unsub := hub.Subscribe("u1")
	defer unsub()

	hub.BroadcastEvent("", Event{Kind: "revoked"})
	select {
	case got := <-ch:
		t.Fatalf("unexpected event for empty user broadcast: %+v", got)
	default:
	}

	hub.BroadcastEvent("u1", Event{})
	select {
	case got := <-ch:
		if got.Kind != "change" {
			t.Fatalf("kind=%q want=change", got.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for default change event")
	}
}
