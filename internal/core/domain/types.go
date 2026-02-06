package domain

import "time"

type TimeControl struct {
	InitialTime int `json:"initial_time"` // total seconds
	Increment   int `json:"increment"`    // Seconds added per move
}

type PlayerStatus string

const (
	StatusOnline  PlayerStatus = "online"
	StatusOffline PlayerStatus = "offline"
)

type Participant struct {
	UserID string       `json:"user_id"`
	Status PlayerStatus `json:"status"`
	// We keep this unexported (lowercase) or tagged with "-" so it stays out of JSON
	TimeRemaining time.Duration `json:"time_remaining_raw"`
	// This is what the frontend will see
	TimeFormatted float64 `json:"time_remaining"`
}

// SyncTime updates the exported float field from the internal duration
func (p *Participant) SyncTime() {
	p.TimeFormatted = p.TimeRemaining.Seconds()
}

type Move struct {
	FENBefore string    `json:"fen_before"`
	Notation  string    `json:"notation"`
	PlayerID  string    `json:"player_id"`
	Timestamp time.Time `json:"timestamp"`
}
