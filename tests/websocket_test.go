//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type wsEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// dial opens a real WebSocket against the running test server.
func dial(t *testing.T, wsURL, gameID, playerID string) *websocket.Conn {
	t.Helper()
	url := wsURL + "/ws?game_id=" + gameID + "&player_id=" + playerID
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("dial %s: %v (status %d)", playerID, err, status)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// nextEvent reads until it sees the wanted type, or fails on timeout. Other
// events are skipped so an unrelated broadcast cannot make the test flaky.
func nextEvent(t *testing.T, conn *websocket.Conn, want string) wsEvent {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn.SetReadDeadline(deadline)
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for %s: %v", want, err)
		}
		var ev wsEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			continue
		}
		if ev.Type == want {
			return ev
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", want)
		}
	}
}

func sendMove(t *testing.T, conn *websocket.Conn, move string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"move": move})
	msg, _ := json.Marshal(wsEvent{Type: "MOVE", Payload: payload})
	if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		t.Fatalf("sending %s: %v", move, err)
	}
}

// A connecting client is sent the current game state straight away.
func TestConnectingClientReceivesGameState(t *testing.T) {
	s := newStack(t)
	_, wsURL := s.server(t)

	game, err := s.Service.CreateGame(context.Background(), "it-white", "it-black", fastControl())
	if err != nil {
		t.Fatalf("CreateGame: %v", err)
	}

	conn := dial(t, wsURL, game.ID.String(), "it-white")
	ev := nextEvent(t, conn, "GAME_UPDATE")

	var state struct {
		ID         string `json:"id"`
		CurrentFEN string `json:"current_fen"`
	}
	if err := json.Unmarshal(ev.Payload, &state); err != nil {
		t.Fatalf("decoding state: %v", err)
	}
	if state.ID != game.ID.String() {
		t.Errorf("received state for game %s, want %s", state.ID, game.ID)
	}
	if state.CurrentFEN == "" {
		t.Error("initial sync carried no position")
	}
}

// A move by one player must reach the other player's socket.
func TestMoveIsBroadcastToOpponent(t *testing.T) {
	s := newStack(t)
	_, wsURL := s.server(t)

	game, _ := s.Service.CreateGame(context.Background(), "it-white", "it-black", fastControl())

	white := dial(t, wsURL, game.ID.String(), "it-white")
	black := dial(t, wsURL, game.ID.String(), "it-black")
	nextEvent(t, white, "GAME_UPDATE")
	nextEvent(t, black, "GAME_UPDATE")

	sendMove(t, white, "e4")

	ev := nextEvent(t, black, "GAME_UPDATE")
	var state struct {
		History []struct {
			Notation string `json:"notation"`
			PlayerID string `json:"player_id"`
		} `json:"history"`
	}
	json.Unmarshal(ev.Payload, &state)

	if len(state.History) != 1 {
		t.Fatalf("opponent saw %d moves, want 1", len(state.History))
	}
	if state.History[0].Notation != "e4" {
		t.Errorf("opponent saw %q, want e4", state.History[0].Notation)
	}
	if state.History[0].PlayerID != "it-white" {
		t.Errorf("move attributed to %q, want it-white", state.History[0].PlayerID)
	}
}

// Moving out of turn returns an error to the sender and nothing to the room.
func TestOutOfTurnMoveIsRejected(t *testing.T) {
	s := newStack(t)
	_, wsURL := s.server(t)

	game, _ := s.Service.CreateGame(context.Background(), "it-white", "it-black", fastControl())

	black := dial(t, wsURL, game.ID.String(), "it-black")
	nextEvent(t, black, "GAME_UPDATE")

	sendMove(t, black, "e5") // black cannot open

	ev := nextEvent(t, black, "ERROR")
	var msg string
	json.Unmarshal(ev.Payload, &msg)
	if msg == "" {
		t.Error("ERROR event carried no message")
	}

	loaded, _ := s.Service.GetGame(context.Background(), game.ID)
	if len(loaded.History) != 0 {
		t.Errorf("rejected move was still recorded: %d moves in history", len(loaded.History))
	}
}

// A full game over the socket ends with GAME_OVER and a Mongo archive.
func TestFullGameOverWebSocketIsArchived(t *testing.T) {
	s := newStack(t)
	_, wsURL := s.server(t)
	ctx := context.Background()

	game, _ := s.Service.CreateGame(ctx, "it-white", "it-black", fastControl())

	conns := map[string]*websocket.Conn{
		"white": dial(t, wsURL, game.ID.String(), "it-white"),
		"black": dial(t, wsURL, game.ID.String(), "it-black"),
	}
	for _, c := range conns {
		nextEvent(t, c, "GAME_UPDATE")
	}

	for _, m := range foolsMate {
		sendMove(t, conns[m.Player], m.Move)
		// Wait for the move to land before sending the next one.
		nextEvent(t, conns[m.Player], "GAME_UPDATE")
	}

	ev := nextEvent(t, conns["white"], "GAME_OVER")
	var over struct {
		Winner string `json:"winner"`
		Reason string `json:"reason"`
	}
	json.Unmarshal(ev.Payload, &over)

	if over.Winner != "it-black" {
		t.Errorf("winner = %q, want it-black", over.Winner)
	}
	if over.Reason != "Checkmate" {
		t.Errorf("reason = %q, want Checkmate", over.Reason)
	}

	if _, err := s.archived(ctx, game.ID.String()); err != nil {
		t.Errorf("game played over the socket was not archived: %v", err)
	}
}

// A disconnect used to deadlock the handler, taking every game with it. The
// room must be told, and the server must keep serving.
func TestDisconnectNotifiesRoomAndServerSurvives(t *testing.T) {
	s := newStack(t)
	_, wsURL := s.server(t)

	game, _ := s.Service.CreateGame(context.Background(), "it-white", "it-black", fastControl())

	white := dial(t, wsURL, game.ID.String(), "it-white")
	black := dial(t, wsURL, game.ID.String(), "it-black")
	nextEvent(t, white, "GAME_UPDATE")
	nextEvent(t, black, "GAME_UPDATE")

	white.Close()

	ev := nextEvent(t, black, "PLAYER_DISCONNECTED")
	var payload map[string]string
	json.Unmarshal(ev.Payload, &payload)
	if payload["player_id"] != "it-white" {
		t.Errorf("disconnect reported %q, want it-white", payload["player_id"])
	}

	// The handler must still work afterwards: a new game, a new socket, a move.
	second, _ := s.Service.CreateGame(context.Background(), "it-white", "it-black", fastControl())
	fresh := dial(t, wsURL, second.ID.String(), "it-white")
	nextEvent(t, fresh, "GAME_UPDATE")
	sendMove(t, fresh, "e4")
	nextEvent(t, fresh, "GAME_UPDATE")
}

func TestWebSocketRejectsMissingParameters(t *testing.T) {
	s := newStack(t)
	srv, _ := s.server(t)

	for _, url := range []string{
		srv.URL + "/ws",
		srv.URL + "/ws?game_id=not-a-uuid&player_id=it-white",
		srv.URL + "/ws?game_id=" + "11111111-1111-1111-1111-111111111111",
	} {
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("GET %s → %d, want 400", url, resp.StatusCode)
		}
	}
}
