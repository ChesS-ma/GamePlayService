package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/notnil/chess"
)

type Game struct {
	ID        uuid.UUID   `json:"id"`
	White     Participant `json:"white"`
	Black     Participant `json:"black"`
	Settings  TimeControl `json:"settings"`
	History   []Move      `json:"history"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`

	// internalGame is not exported to JSON.
	// We use it for move validation and state calculation.
	internalGame *chess.Game
}

// NewGame is a Factory function to initialize a game correctly
func NewGame(whiteID, blackID string, tc TimeControl) *Game {
	return &Game{
		ID:           uuid.New(),
		White:        Participant{UserID: whiteID, Status: StatusOnline, TimeRemaining: time.Duration(tc.InitialTime) * time.Second},
		Black:        Participant{UserID: blackID, Status: StatusOnline, TimeRemaining: time.Duration(tc.InitialTime) * time.Second},
		Settings:     tc,
		History:      []Move{},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		internalGame: chess.NewGame(),
	}
}

// MakeMove validates and applies a move in Algebraic Notation (e.g., "e4")
func (g *Game) MakeMove(playerID string, moveNotation string) error {
	if g.IsGameOver() {
		return errors.New("game is already finished")
	}

	now := time.Now()
	// Calculate how long the player thought
	thinkTime := now.Sub(g.UpdatedAt)

	// Identify the current turn from the chess engine
	currentTurn := g.internalGame.Position().Turn()

	// Validate Turn and Update Clock
	if currentTurn == chess.White {
		if playerID != g.White.UserID {
			return errors.New("it is not your turn")
		}
		g.White.TimeRemaining -= thinkTime
		// Add increment if your settings have it
		g.White.TimeRemaining += time.Duration(g.Settings.Increment) * time.Second
	} else {
		if playerID != g.Black.UserID {
			return errors.New("it is not your turn")
		}
		g.Black.TimeRemaining -= thinkTime
		g.Black.TimeRemaining += time.Duration(g.Settings.Increment) * time.Second
	}

	// Check if player flagged (lost on time)
	if g.White.TimeRemaining <= 0 || g.Black.TimeRemaining <= 0 {
		return errors.New("game over: player ran out of time")
	}

	// Apply Move to Engine
	err := g.internalGame.MoveStr(moveNotation)
	if err != nil {
		return errors.New("invalid move: " + moveNotation)
	}

	// Update History & Metadata
	g.History = append(g.History, Move{
		FENBefore: g.GetFEN(),
		Notation:  moveNotation,
		PlayerID:  playerID,
		Timestamp: now,
	})

	g.UpdatedAt = now // Reset the clock start for the next player
	return nil
}

// GetFEN returns the current board position
func (g *Game) GetFEN() string {
	return g.internalGame.FEN()
}

// IsGameOver checks if the game has ended (Checkmate, Draw, etc.)
func (g *Game) IsGameOver() bool {
	return g.internalGame.Outcome() != chess.NoOutcome
}

// GetResult returns the winner or draw reason
func (g *Game) GetResult() string {
	return g.internalGame.Outcome().String()
}

// RehydrateEngine reconstructs the chess engine from a FEN string.
// This is essential when loading from a database/Redis.
func (g *Game) RehydrateEngine(fen string) error {
	f, err := chess.FEN(fen)
	if err != nil {
		return err
	}
	g.internalGame = chess.NewGame(f)
	return nil
}
