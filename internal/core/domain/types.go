package domain

import "time"

type TimeControl struct {
	InitialTime int `json:"initial_time"` //total seconds
	Increment   int `json:"increment"`    //Seconds added per move
}

type PlayerStatus string

const (
	StatusOnline  PlayerStatus = "online"
	StatusOffline PlayerStatus = "offline"
)

type Participant struct {
	UserID        string        `json:"user_id"`
	Status        PlayerStatus  `json:"status"`
	TimeRemaining time.Duration `json:"time_remaining"`
}

type Move struct {
	FENBefore string    `json:"fen_before"`
	Notation  string    `json:"notation"` // e.g., "e4", "Nf3", "O-O"
	PlayerID  string    `json:"player_id"`
	Timestamp time.Time `json:"timestamp"`
}

// Add this helper to types.go
func (p Participant) TimeSeconds() float64 {
	return p.TimeRemaining.Seconds()
}
