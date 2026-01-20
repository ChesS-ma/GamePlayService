package domain

import (
	"github.com/google/uuid"
	"time"
)

type Game struct {
	ID        uuid.UUID `json:"id"`
	Players   Players   `json:"players"`
	Board     string    `json:"board"` //FEN string (e.g, "rngbl..." )
	State     GameState `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GameState struct {
	Turn      string //white or black
	IsOver    bool
	Result    string //white , black or draw
	Checkmate bool
	Stalemate bool
}
type Players struct {
	WhiteId string `json:"white_id"`
	BlackId string `json:"black_id"`
}

type Move struct {
	From string `json:"from"`
	To   string `json:"to"`
}
