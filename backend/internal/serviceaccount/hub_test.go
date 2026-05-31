package serviceaccount

import (
	"context"
	"testing"
	"time"
)

func TestEventHubSubscribeBroadcastAndUnsubscribe(t *testing.T) {
	hub := NewEventHub()
	ch, unsub := hub.Subscribe()

	hub.Broadcast()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("expected broadcast signal")
	}

	unsub()
	if len(hub.clients) != 0 {
		t.Fatalf("expected no clients after unsubscribe, got %d", len(hub.clients))
	}
}

func TestEventHubListenForChangesReturnsOnCanceledContext(t *testing.T) {
	hub := NewEventHub()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		hub.ListenForChanges(ctx, "postgres://invalid")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ListenForChanges should exit when context is canceled")
	}
}

func TestEventHubBroadcastSkipsFullClientBuffer(t *testing.T) {
	hub := NewEventHub()
	slow, unsubSlow := hub.Subscribe()
	defer unsubSlow()
	fast, unsubFast := hub.Subscribe()
	defer unsubFast()

	// Fill slow client's single-slot buffer so Broadcast must take the default case.
	slow <- struct{}{}

	hub.Broadcast()

	select {
	case <-fast:
	case <-time.After(time.Second):
		t.Fatal("expected fast client to receive broadcast")
	}
}

func TestEventHubListenOnceReturnsConnectError(t *testing.T) {
	hub := NewEventHub()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := hub.listenOnce(ctx, "postgres://127.0.0.1:1/doesnotexist?connect_timeout=1")
	if err == nil {
		t.Fatal("expected listenOnce to return a connection error")
	}
}
