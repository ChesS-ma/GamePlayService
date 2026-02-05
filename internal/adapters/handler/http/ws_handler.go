package http

import (
	"context"
	"encoding/json"
	"github.com/ChesS-ma/gameplay_service/internal/core/ports"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"sync"
	"time"
)

// Client is a middleman between the websocket connection and the room.
type Client struct {
	GameID   uuid.UUID
	PlayerID string
	Conn     *websocket.Conn
	Send     chan []byte // Channel for messages to be sent to the browser
}

// WsEvent defines the envelope for all socket messages
type WsEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type WsHandler struct {
	service ports.GameService
	rooms   map[uuid.UUID][]*Client
	mu      sync.RWMutex
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // Allow all for dev
}

func NewWsHandler(service ports.GameService) *WsHandler {
	return &WsHandler{
		service: service,
		rooms:   make(map[uuid.UUID][]*Client),
	}
}

// HandleWS is the main entry point for /ws?game_id=...&player_id=...
func (h *WsHandler) HandleWS(w http.ResponseWriter, r *http.Request) {
	gameID, err := uuid.Parse(r.URL.Query().Get("game_id"))
	playerID := r.URL.Query().Get("player_id")
	if err != nil || playerID == "" {
		http.Error(w, "Missing game_id or player_id", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	client := &Client{
		GameID:   gameID,
		PlayerID: playerID,
		Conn:     conn,
		Send:     make(chan []byte, 256),
	}

	h.registerClient(client)

	// This ensures there is a listener ready for the Send channel
	go h.writePump(client)
	go h.readPump(client)

	// Now that the pumps are running, we fetch and send the state
	game, err := h.service.GetGame(context.Background(), gameID)
	if err != nil {
		log.Printf("Sync Error for game %s: %v", gameID, err)
		// Optional: send an error message to the client via their channel
		return
	}

	gameData, _ := json.Marshal(game)
	syncEvent := WsEvent{
		Type:    "GAME_UPDATE",
		Payload: gameData,
	}
	msg, _ := json.Marshal(syncEvent)

	// The writePump is now active and will immediately pick this up
	client.Send <- msg
}

func (h *WsHandler) readPump(c *Client) {
	defer func() {
		h.unregisterClient(c)
		c.Conn.Close()
	}()

	// ---  configs for the Heartbeat (Read Deadline) ---
	c.Conn.SetReadLimit(512 * 1024) // Max message size 512KB
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	// When the browser responds with a PONG, reset the timer
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}

		var event WsEvent
		if err := json.Unmarshal(message, &event); err != nil {
			continue
		}

		if event.Type == "MOVE" {
			var req struct {
				Move string `json:"move"`
			}
			json.Unmarshal(event.Payload, &req)

			game, err := h.service.MakeMove(context.Background(), c.GameID, c.PlayerID, req.Move)
			if err != nil {
				h.sendError(c, err.Error())
				continue
			}
			// Broadcast to the players that the game is updated
			h.broadcastToRoom(c.GameID, "GAME_UPDATE", game)

			// 2. Check if the game is over
			if game.IsGameOver() {
				h.broadcastToRoom(c.GameID, "GAME_OVER", map[string]interface{}{
					"winner": game.WinnerID,
					"reason": game.ResultReason,
				})
			}
		}
	}
}

//	func (h *WsHandler) writePump(c *Client) {
//		for message := range c.Send {
//			c.Conn.WriteMessage(websocket.TextMessage, message)
//		}
//	}
func (h *WsHandler) writePump(c *Client) {
	// 1. Create the heartbeat timer
	ticker := time.NewTicker(54 * time.Second)

	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		// 2. This case handles actual game messages (moves, etc.)
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// The Hub closed the channel, send a close message to browser
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		// 3. This case handles the "Are you still there?" Ping
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Ping failed for player %s, disconnecting", c.PlayerID)
				return // Kill the connection if Ping fails
			}
		}
	}
}

func (h *WsHandler) registerClient(c *Client) {
	h.mu.Lock()
	h.rooms[c.GameID] = append(h.rooms[c.GameID], c)
	h.mu.Unlock()
}

func (h *WsHandler) unregisterClient(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clients := h.rooms[c.GameID]
	for i, client := range clients {
		if client == c {
			// Remove the client from the slice
			h.rooms[c.GameID] = append(clients[:i], clients[i+1:]...)
			// Notify the room that a specific player is gone
			// doing this while still holding the lock for safety
			h.broadcastToRoom(c.GameID, "PLAYER_DISCONNECTED", map[string]string{
				"player_id": c.PlayerID,
			})
			break
		}
	}
}
func (h *WsHandler) broadcastToRoom(gameID uuid.UUID, eventType string, payload interface{}) {
	h.mu.RLock()
	clients := h.rooms[gameID]
	h.mu.RUnlock()

	data, _ := json.Marshal(payload)
	event := WsEvent{Type: eventType, Payload: data}
	msg, _ := json.Marshal(event)

	for _, client := range clients {
		client.Send <- msg
	}
}
func (h *WsHandler) sendError(c *Client, msg string) {
	// Ensure the payload is a valid JSON string without extra spaces
	payload := []byte(`"` + msg + `"`)
	event := WsEvent{Type: "ERROR", Payload: payload}
	data, _ := json.Marshal(event)
	c.Send <- data
}
