package domain

import (
	"time"

	"github.com/google/uuid"
)

// GameEventType helps identify what happened
type GameEventType string

const (
	EventGameStarted  GameEventType = "GAME_STARTED"
	EventMoveMade     GameEventType = "MOVE_MADE"
	EventGameFinished GameEventType = "GAME_FINISHED"
)

// GameEvent is a generic structure to represent changes in the domain
type GameEvent struct {
	GameID     uuid.UUID     `json:"game_id"`
	Type       GameEventType `json:"type"`
	Payload    interface{}   `json:"payload"`
	OccurredAt time.Time     `json:"occurred_at"`
}
