//go:build integration

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/ChesS-ma/gameplay_service/internal/core/domain"
	"go.mongodb.org/mongo-driver/bson"
)

func fastControl() domain.TimeControl {
	return domain.TimeControl{InitialTime: 300, Increment: 2}
}

// A game survives the round trip through Redis: the chess engine is unexported
// and does not serialize, so every move depends on the service rehydrating it
// from the stored FEN.
func TestGameSurvivesRedisRoundTrip(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	created, err := s.Service.CreateGame(ctx, "it-white", "it-black", fastControl())
	if err != nil {
		t.Fatalf("CreateGame: %v", err)
	}

	opening := []struct{ player, move string }{
		{"it-white", "e4"}, {"it-black", "e5"}, {"it-white", "Nf3"},
		{"it-black", "Nc6"}, {"it-white", "Bb5"},
	}
	for _, m := range opening {
		if _, err := s.Service.MakeMove(ctx, created.ID, m.player, m.move); err != nil {
			t.Fatalf("%s: %v", m.move, err)
		}
	}

	loaded, err := s.Service.GetGame(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetGame: %v", err)
	}
	if len(loaded.History) != len(opening) {
		t.Fatalf("history has %d moves, want %d", len(loaded.History), len(opening))
	}
	if loaded.GetFEN() != loaded.CurrentFEN {
		t.Errorf("rehydrated engine at %q but stored FEN is %q", loaded.GetFEN(), loaded.CurrentFEN)
	}
	// The reloaded game must still be playable.
	if _, err := s.Service.MakeMove(ctx, created.ID, "it-black", "a6"); err != nil {
		t.Errorf("reloaded game rejected a legal move: %v", err)
	}
}

// Every move must be visible in Redis immediately, not buffered in memory.
func TestMovesArePersistedImmediately(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	created, _ := s.Service.CreateGame(ctx, "it-white", "it-black", fastControl())

	if _, err := s.Service.MakeMove(ctx, created.ID, "it-white", "d4"); err != nil {
		t.Fatalf("MakeMove: %v", err)
	}

	raw, err := s.Redis.Get(ctx, "game:"+created.ID.String()).Result()
	if err != nil {
		t.Fatalf("game not found in redis: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("redis holds an empty record")
	}

	ttl, err := s.Redis.TTL(ctx, "game:"+created.ID.String()).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 || ttl > 24*time.Hour {
		t.Errorf("TTL = %v, want a positive value no greater than 24h", ttl)
	}
}

// A finished game must reach the Mongo archive. This is the regression test for
// the archiving branch that had been commented out of the service.
func TestFinishedGameReachesMongoArchive(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	created, _ := s.Service.CreateGame(ctx, "it-white", "it-black", fastControl())
	for _, m := range foolsMate {
		if _, err := s.Service.MakeMove(ctx, created.ID, "it-"+m.Player, m.Move); err != nil {
			t.Fatalf("%s: %v", m.Move, err)
		}
	}

	doc, err := s.archived(ctx, created.ID.String())
	if err != nil {
		t.Fatalf("finished game was not archived: %v", err)
	}

	if doc["white_id"] != "it-white" || doc["black_id"] != "it-black" {
		t.Errorf("archived players are wrong: %v / %v", doc["white_id"], doc["black_id"])
	}
	if doc["board_fen"] == "" || doc["board_fen"] == nil {
		t.Error("archive is missing the final position")
	}
	history, ok := doc["history"].(bson.A)
	if !ok {
		t.Fatalf("archived history has type %T, want an array", doc["history"])
	}
	if len(history) != len(foolsMate) {
		t.Errorf("archived %d moves, want %d", len(history), len(foolsMate))
	}
	if doc["result"] == nil {
		t.Error("archive is missing the result")
	}
	if doc["archived_at"] == nil {
		t.Error("archive is missing archived_at")
	}
}

// The finished game stays readable until its TTL expires. Deleting it from
// Redis on checkmate meant a client reconnecting straight after got a 404.
func TestFinishedGameRemainsReadable(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	created, _ := s.Service.CreateGame(ctx, "it-white", "it-black", fastControl())
	for _, m := range foolsMate {
		s.Service.MakeMove(ctx, created.ID, "it-"+m.Player, m.Move)
	}

	got, err := s.Service.GetGame(ctx, created.ID)
	if err != nil {
		t.Fatalf("finished game should still be readable: %v", err)
	}
	if !got.IsFinished {
		t.Error("game is not marked finished")
	}
	if got.WinnerID != "it-black" {
		t.Errorf("WinnerID = %q, want it-black", got.WinnerID)
	}
	if got.ResultReason != "Checkmate" {
		t.Errorf("ResultReason = %q, want Checkmate", got.ResultReason)
	}
}

func TestUnfinishedGameIsNotArchived(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	created, _ := s.Service.CreateGame(ctx, "it-white", "it-black", fastControl())
	s.Service.MakeMove(ctx, created.ID, "it-white", "e4")

	if _, err := s.archived(ctx, created.ID.String()); err == nil {
		t.Error("a game still in progress was archived")
	}
}
