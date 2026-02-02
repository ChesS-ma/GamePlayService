package http

import (
	"encoding/json"
	"github.com/ChesS-ma/gameplay_service/internal/core/ports"
	"github.com/google/uuid" // Needed to parse IDs
	"net/http"
)

type GameHandler struct {
	service ports.GameService
}

func NewGameHandler(service ports.GameService) *GameHandler {
	return &GameHandler{
		service: service,
	}
}

// --- Request DTOs ---

type CreateGameRequest struct {
	WhiteId string `json:"white_id"`
	BlackId string `json:"black_id"`
}

type MakeMoveRequest struct {
	PlayerId string `json:"player_id"`
	Move     string `json:"move"` // Standard Algebraic Notation e.g., "e4"
}

// --- Handler Methods ---

func (h *GameHandler) CreateGame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req CreateGameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	game, err := h.service.CreateGame(r.Context(), req.WhiteId, req.BlackId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(game)
}

func (h *GameHandler) MakeMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Extract Game ID from URL (e.g., /games/move?id=xxx)
	idStr := r.URL.Query().Get("id")
	gameId, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid game ID format", http.StatusBadRequest)
		return
	}

	// 2. Decode Move payload
	var req MakeMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid move data", http.StatusBadRequest)
		return
	}

	// 3. Call service
	game, err := h.service.MakeMove(r.Context(), gameId, req.PlayerId, req.Move)
	if err != nil {
		// We use StatusConflict or BadRequest for illegal moves
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	//log.Printf("Player %s moved. Time remaining: %.2f seconds",
	//	game.White.UserID,
	//	game.White.TimeSeconds(),
	//)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(game)
}

func (h *GameHandler) GetGame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.URL.Query().Get("id")
	gameId, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	game, err := h.service.GetGame(r.Context(), gameId)
	if err != nil {
		http.Error(w, "Game not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(game)
}
