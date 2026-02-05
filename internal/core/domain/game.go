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

	WinnerID     string `json:"winner_id,omitempty"` // "" for draw, or the player's UserID
	ResultReason string `json:"result_reason"`       // "CHECKMATE", "TIMEOUT", "STALEMATE", etc.
	IsFinished   bool   `json:"is_finished"`

	// internalGame is not exported to JSON.
	// We use it for move validation and state calculation (using the chess package )
	internalGame *chess.Game
}

// NewGame is a Factory function to initialize a game correctly
func NewGame(whiteID, blackID string, tc TimeControl) *Game {
	game := &Game{
		ID:           uuid.New(),
		White:        Participant{UserID: whiteID, Status: StatusOnline, TimeRemaining: time.Duration(tc.InitialTime) * time.Second},
		Black:        Participant{UserID: blackID, Status: StatusOnline, TimeRemaining: time.Duration(tc.InitialTime) * time.Second},
		Settings:     tc,
		History:      []Move{},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		internalGame: chess.NewGame(),
	}
	// Initialize the JSON-friendly fields
	game.White.SyncTime()
	game.Black.SyncTime()
	return game
}

// MakeMove validates and applies a move in Algebraic Notation (e.g., "e4")
func (g *Game) MakeMove(playerID string, moveNotation string) error {
	if g.IsFinished || g.IsGameOver() {
		return errors.New("game is already finished")
	}

	now := time.Now()
	currentTurn := g.internalGame.Position().Turn()

	// 1. CLOCK LOGIC
	// Only calculate thinkTime if this is NOT the first move of the game
	if len(g.History) > 0 {
		thinkTime := now.Sub(g.UpdatedAt)

		if currentTurn == chess.White {
			if playerID != g.White.UserID {
				return errors.New("it is not your turn")
			}
			g.White.TimeRemaining -= thinkTime
			g.White.TimeRemaining += time.Duration(g.Settings.Increment) * time.Second
		} else {
			if playerID != g.Black.UserID {
				return errors.New("it is not your turn")
			}
			g.Black.TimeRemaining -= thinkTime
			g.Black.TimeRemaining += time.Duration(g.Settings.Increment) * time.Second
		}
	} else {
		// FIRST MOVE: No time is subtracted.
		// Just verify the right player is starting.
		if playerID != g.White.UserID {
			return errors.New("white must start the game")
		}
	}

	// 2. TIMEOUT PROTECTION
	// If a player hits 0, they lose.
	if g.White.TimeRemaining <= 0 {
		g.White.TimeRemaining = 0
		g.White.SyncTime()
		g.finishGame(g.Black.UserID, "TIMEOUT")
		return nil
	}
	if g.Black.TimeRemaining <= 0 {
		g.Black.TimeRemaining = 0
		g.Black.SyncTime()
		g.finishGame(g.White.UserID, "TIMEOUT")
		return nil
	}

	// 3. APPLY TO ENGINE
	err := g.internalGame.MoveStr(moveNotation)
	if err != nil {
		return errors.New("invalid move format")
	}

	// 4. UPDATE STATE
	g.History = append(g.History, Move{
		FENBefore: g.GetFEN(),
		Notation:  moveNotation,
		PlayerID:  playerID,
		Timestamp: now,
	})

	// Sync the float64 fields for JSON
	g.White.SyncTime()
	g.Black.SyncTime()

	if g.IsGameOver() {
		outcome := g.internalGame.Outcome()
		var winner string
		if outcome == chess.WhiteWon {
			winner = g.White.UserID
		} else if outcome == chess.BlackWon {
			winner = g.Black.UserID
		} else {
			winner = "DRAW"
		}
		g.finishGame(winner, g.internalGame.Method().String())
	}

	// 5. IMPORTANT: Reset the clock start point for the NEXT move
	g.UpdatedAt = now
	return nil
}
func (g *Game) finishGame(winnerID string, reason string) {
	g.IsFinished = true
	g.WinnerID = winnerID
	g.ResultReason = reason
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
