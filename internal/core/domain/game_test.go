package domain

import (
	"testing"
	"time"
)

const startFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

func newTestGame() *Game {
	return NewGame("white-1", "black-1", TimeControl{InitialTime: 300, Increment: 2})
}

// Bug 4: NewGame left CurrentFEN empty, so callers fell back to a hardcoded string.
func TestNewGameSetsCurrentFEN(t *testing.T) {
	g := newTestGame()
	if g.CurrentFEN != startFEN {
		t.Fatalf("CurrentFEN = %q, want the starting position", g.CurrentFEN)
	}
}

func TestNewGameInitialisesClocks(t *testing.T) {
	g := newTestGame()
	if g.White.TimeRemaining != 300*time.Second || g.Black.TimeRemaining != 300*time.Second {
		t.Fatalf("clocks not seeded: white=%v black=%v", g.White.TimeRemaining, g.Black.TimeRemaining)
	}
	if g.White.TimeFormatted != 300 || g.Black.TimeFormatted != 300 {
		t.Fatalf("clocks not synced for JSON: white=%v black=%v", g.White.TimeFormatted, g.Black.TimeFormatted)
	}
}

// Bug 3: FENBefore recorded the position AFTER the move was applied.
func TestMakeMoveRecordsPositionBeforeTheMove(t *testing.T) {
	g := newTestGame()

	if err := g.MakeMove("white-1", "e4"); err != nil {
		t.Fatalf("e4: %v", err)
	}

	if len(g.History) != 1 {
		t.Fatalf("history length = %d, want 1", len(g.History))
	}
	if got := g.History[0].FENBefore; got != startFEN {
		t.Errorf("FENBefore = %q, want the position before e4 (%q)", got, startFEN)
	}
	if g.CurrentFEN == startFEN {
		t.Error("CurrentFEN did not advance after the move")
	}
	if g.History[0].FENBefore == g.CurrentFEN {
		t.Error("FENBefore equals the position after the move")
	}
}

func TestMakeMoveHistoryChainsPositions(t *testing.T) {
	g := newTestGame()
	moves := []struct{ player, move string }{
		{"white-1", "e4"}, {"black-1", "e5"}, {"white-1", "Nf3"},
	}

	var fens []string
	for _, m := range moves {
		fens = append(fens, g.CurrentFEN)
		if err := g.MakeMove(m.player, m.move); err != nil {
			t.Fatalf("%s: %v", m.move, err)
		}
	}

	for i, want := range fens {
		if g.History[i].FENBefore != want {
			t.Errorf("move %d FENBefore = %q, want %q", i, g.History[i].FENBefore, want)
		}
	}
}

func TestMakeMoveRejectsIllegalMove(t *testing.T) {
	g := newTestGame()
	if err := g.MakeMove("white-1", "e9"); err == nil {
		t.Fatal("expected an error for an impossible move")
	}
	if len(g.History) != 0 {
		t.Errorf("illegal move was recorded in history")
	}
}

func TestMakeMoveRejectsWrongPlayer(t *testing.T) {
	g := newTestGame()
	if err := g.MakeMove("black-1", "e4"); err == nil {
		t.Fatal("black must not be able to open the game")
	}

	if err := g.MakeMove("white-1", "e4"); err != nil {
		t.Fatalf("e4: %v", err)
	}
	if err := g.MakeMove("white-1", "e5"); err == nil {
		t.Fatal("white must not be able to move twice in a row")
	}
}

func TestMakeMoveDetectsCheckmate(t *testing.T) {
	g := newTestGame()
	// Fool's mate.
	for _, m := range []struct{ player, move string }{
		{"white-1", "f3"}, {"black-1", "e5"}, {"white-1", "g4"}, {"black-1", "Qh4"},
	} {
		if err := g.MakeMove(m.player, m.move); err != nil {
			t.Fatalf("%s: %v", m.move, err)
		}
	}

	if !g.IsFinished {
		t.Fatal("game should be finished after fool's mate")
	}
	if g.WinnerID != "black-1" {
		t.Errorf("WinnerID = %q, want black-1", g.WinnerID)
	}
	if g.ResultReason != "Checkmate" {
		t.Errorf("ResultReason = %q, want Checkmate", g.ResultReason)
	}
}

func TestMakeMoveOnFinishedGameIsRejected(t *testing.T) {
	g := newTestGame()
	for _, m := range []struct{ player, move string }{
		{"white-1", "f3"}, {"black-1", "e5"}, {"white-1", "g4"}, {"black-1", "Qh4"},
	} {
		if err := g.MakeMove(m.player, m.move); err != nil {
			t.Fatalf("%s: %v", m.move, err)
		}
	}

	if err := g.MakeMove("white-1", "Nc3"); err == nil {
		t.Fatal("expected an error moving in a finished game")
	}
}

func TestMakeMoveFlagsPlayerOnTimeout(t *testing.T) {
	g := NewGame("white-1", "black-1", TimeControl{InitialTime: 1, Increment: 0})

	if err := g.MakeMove("white-1", "e4"); err != nil {
		t.Fatalf("e4: %v", err)
	}
	// Black now thinks for longer than the whole clock.
	g.Black.TimeRemaining = 5 * time.Millisecond
	time.Sleep(20 * time.Millisecond)

	if err := g.MakeMove("black-1", "e5"); err != nil {
		t.Fatalf("e5: %v", err)
	}

	if !g.IsFinished {
		t.Fatal("game should end when a player runs out of time")
	}
	if g.WinnerID != "white-1" {
		t.Errorf("WinnerID = %q, want white-1", g.WinnerID)
	}
	if g.ResultReason != "TIMEOUT" {
		t.Errorf("ResultReason = %q, want TIMEOUT", g.ResultReason)
	}
	if g.Black.TimeRemaining != 0 {
		t.Errorf("flagged clock should be pinned to 0, got %v", g.Black.TimeRemaining)
	}
}

func TestRehydrateEngineRestoresPosition(t *testing.T) {
	g := newTestGame()
	if err := g.MakeMove("white-1", "e4"); err != nil {
		t.Fatalf("e4: %v", err)
	}
	saved := g.CurrentFEN

	// Simulate a round trip through Redis: the engine is unexported and lost.
	restored := &Game{ID: g.ID, White: g.White, Black: g.Black, Settings: g.Settings, History: g.History}
	if err := restored.RehydrateEngine(saved); err != nil {
		t.Fatalf("rehydrate: %v", err)
	}

	if restored.GetFEN() != saved {
		t.Errorf("GetFEN = %q, want %q", restored.GetFEN(), saved)
	}
	if err := restored.MakeMove("black-1", "e5"); err != nil {
		t.Errorf("restored game should accept black's reply: %v", err)
	}
}

// Games written to Redis before CurrentFEN was populated must still load.
func TestRehydrateEngineTreatsEmptyFENAsStart(t *testing.T) {
	g := &Game{}
	if err := g.RehydrateEngine(""); err != nil {
		t.Fatalf("empty FEN should not error: %v", err)
	}
	if g.GetFEN() != startFEN {
		t.Errorf("GetFEN = %q, want the starting position", g.GetFEN())
	}
	if g.CurrentFEN != startFEN {
		t.Errorf("CurrentFEN = %q, want the starting position", g.CurrentFEN)
	}
}

func TestRehydrateEngineRejectsGarbage(t *testing.T) {
	g := &Game{}
	if err := g.RehydrateEngine("not a fen"); err == nil {
		t.Fatal("expected an error for an invalid FEN")
	}
}
