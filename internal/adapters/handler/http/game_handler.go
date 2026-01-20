package http

import (
	"encoding/json"
	"github.com/ChesS-ma/gameplay_service/internal/core/ports"
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

type CreateGameRequest struct {
	WhiteId string `json:"white_id"`
	BlackId string `json:"black_id"`
}

func (h *GameHandler) CreateGame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req CreateGameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
