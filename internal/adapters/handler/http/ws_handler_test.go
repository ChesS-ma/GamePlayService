package http

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newTestClient(gameID uuid.UUID, playerID string) *Client {
	return &Client{
		GameID:   gameID,
		PlayerID: playerID,
		Send:     make(chan []byte, 256),
		Done:     make(chan struct{}),
	}
}

// runWithin fails the test if fn has not returned before d elapses. A plain
// call would hang the whole suite rather than report the deadlock.
func runWithin(t *testing.T, d time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s did not return within %v (deadlock)", what, d)
	}
}

// unregisterClient used to broadcast while holding the write lock. RWMutex is
// not reentrant, so the read lock inside broadcastToRoom never succeeded.
func TestUnregisterClientDoesNotDeadlock(t *testing.T) {
	h := NewWsHandler(nil)
	gameID := uuid.New()

	leaving := newTestClient(gameID, "white-1")
	staying := newTestClient(gameID, "black-1")
	h.registerClient(leaving)
	h.registerClient(staying)

	runWithin(t, 2*time.Second, "unregisterClient", func() {
		h.unregisterClient(leaving)
	})

	// The handler must still be usable afterwards.
	runWithin(t, 2*time.Second, "broadcastToRoom after unregister", func() {
		h.broadcastToRoom(gameID, "GAME_UPDATE", map[string]string{"ok": "yes"})
	})
}

func TestUnregisterClientNotifiesTheRoom(t *testing.T) {
	h := NewWsHandler(nil)
	gameID := uuid.New()

	leaving := newTestClient(gameID, "white-1")
	staying := newTestClient(gameID, "black-1")
	h.registerClient(leaving)
	h.registerClient(staying)

	runWithin(t, 2*time.Second, "unregisterClient", func() {
		h.unregisterClient(leaving)
	})

	select {
	case msg := <-staying.Send:
		var event WsEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if event.Type != "PLAYER_DISCONNECTED" {
			t.Fatalf("event type = %q, want PLAYER_DISCONNECTED", event.Type)
		}
		var payload map[string]string
		json.Unmarshal(event.Payload, &payload)
		if payload["player_id"] != "white-1" {
			t.Errorf("player_id = %q, want white-1", payload["player_id"])
		}
	default:
		t.Fatal("remaining player was not notified of the disconnect")
	}

	select {
	case <-leaving.Done:
	default:
		t.Error("leaving client's Done channel was not closed")
	}
}

// A client whose buffer is full must not stall the broadcaster, and a client
// that has gone away must not panic it.
func TestBroadcastDoesNotBlockOnStuckOrGoneClients(t *testing.T) {
	h := NewWsHandler(nil)
	gameID := uuid.New()

	stuck := newTestClient(gameID, "stuck")
	stuck.Send = make(chan []byte) // unbuffered, nothing reading
	gone := newTestClient(gameID, "gone")
	healthy := newTestClient(gameID, "healthy")

	h.registerClient(stuck)
	h.registerClient(gone)
	h.registerClient(healthy)
	gone.close()

	runWithin(t, 2*time.Second, "broadcastToRoom", func() {
		h.broadcastToRoom(gameID, "GAME_UPDATE", map[string]string{"move": "e4"})
	})

	if len(healthy.Send) != 1 {
		t.Errorf("healthy client got %d messages, want 1", len(healthy.Send))
	}
}

func TestClientCloseIsIdempotent(t *testing.T) {
	c := newTestClient(uuid.New(), "p1")
	runWithin(t, time.Second, "repeated close", func() {
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); c.close() }()
		}
		wg.Wait()
	})
}

func TestConcurrentRegisterUnregister(t *testing.T) {
	h := NewWsHandler(nil)
	gameID := uuid.New()

	runWithin(t, 5*time.Second, "concurrent register/unregister", func() {
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				c := newTestClient(gameID, "p")
				h.registerClient(c)
				h.broadcastToRoom(gameID, "GAME_UPDATE", map[string]int{"n": n})
				h.unregisterClient(c)
			}(i)
		}
		wg.Wait()
	})

	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.rooms) != 0 {
		t.Errorf("rooms map not cleaned up: %d entries remain", len(h.rooms))
	}
}
