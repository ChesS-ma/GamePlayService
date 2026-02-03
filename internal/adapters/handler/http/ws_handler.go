package http

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	//"time"

	"github.com/ChesS-ma/gameplay_service/internal/core/ports"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
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

	// Start the pumps!
	go h.writePump(client)
	go h.readPump(client)
}

func (h *WsHandler) readPump(c *Client) {
	defer func() {
		h.unregisterClient(c)
		c.Conn.Close()
	}()

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
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

			// THE NO-REFRESH PART: Tell everyone the board updated
			h.broadcastToRoom(c.GameID, "GAME_UPDATE", game)
		}
	}
}

func (h *WsHandler) writePump(c *Client) {
	for message := range c.Send {
		c.Conn.WriteMessage(websocket.TextMessage, message)
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
			h.rooms[c.GameID] = append(clients[:i], clients[i+1:]...)
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
	event := WsEvent{Type: "ERROR", Payload: json.RawMessage(`"` + msg + `" `)}
	data, _ := json.Marshal(event)
	c.Send <- data
}
