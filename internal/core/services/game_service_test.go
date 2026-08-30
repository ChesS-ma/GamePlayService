package services

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/ChesS-ma/gameplay_service/internal/core/domain"
	"github.com/google/uuid"
)

// fakeRepo stands in for Redis. It serializes games the way the real adapter
// does, so the unexported chess engine is dropped on every read.
type fakeRepo struct {
	mu      sync.Mutex
	games   map[uuid.UUID][]byte
	deleted []uuid.UUID
	saves   int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{games: make(map[uuid.UUID][]byte)}
}

func (r *fakeRepo) Save(ctx context.Context, game *domain.Game) error {
	// Round trip through JSON exactly like the Redis adapter, so the unexported
	// chess engine is genuinely lost and the service has to rehydrate it.
	data, err := json.Marshal(game)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saves++
	r.games[game.ID] = data
	return nil
}

func (r *fakeRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Game, error) {
	r.mu.Lock()
	data, ok := r.games[id]
	r.mu.Unlock()
	if !ok {
		return nil, errors.New("game not found")
	}
	var stored domain.Game
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}
	return &stored, nil
}

func (r *fakeRepo) Update(ctx context.Context, game *domain.Game) error {
	return r.Save(ctx, game)
}

func (r *fakeRepo) Delete(ctx context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.games, id)
	r.deleted = append(r.deleted, id)
	return nil
}

type fakeArchive struct {
	mu       sync.Mutex
	archived []uuid.UUID
	err      error
}

func (a *fakeArchive) Archive(ctx context.Context, game *domain.Game) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return a.err
	}
	a.archived = append(a.archived, game.ID)
	return nil
}

func (a *fakeArchive) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.archived)
}

func newHarness() (*fakeRepo, *fakeArchive, *service) {
	repo := newFakeRepo()
	arch := &fakeArchive{}
	return repo, arch, NewService(repo, arch).(*service)
}

var foolsMate = []struct{ player, move string }{
	{"white-1", "f3"}, {"black-1", "e5"}, {"white-1", "g4"}, {"black-1", "Qh4"},
}

func TestCreateGamePersistsAndSetsFEN(t *testing.T) {
	repo, _, svc := newHarness()
	ctx := context.Background()

	game, err := svc.CreateGame(ctx, "white-1", "black-1", domain.TimeControl{InitialTime: 300, Increment: 2})
	if err != nil {
		t.Fatalf("CreateGame: %v", err)
	}
	if game.CurrentFEN == "" {
		t.Error("a newly created game must carry its starting FEN")
	}
	if _, ok := repo.games[game.ID]; !ok {
		t.Error("game was not written to the repository")
	}
}

// The service must rebuild the engine from the stored FEN; a game read back
// from Redis has no engine at all.
func TestMakeMoveRehydratesStoredGame(t *testing.T) {
	_, _, svc := newHarness()
	ctx := context.Background()

	created, err := svc.CreateGame(ctx, "white-1", "black-1", domain.TimeControl{InitialTime: 300})
	if err != nil {
		t.Fatalf("CreateGame: %v", err)
	}

	after, err := svc.MakeMove(ctx, created.ID, "white-1", "e4")
	if err != nil {
		t.Fatalf("MakeMove: %v", err)
	}
	if len(after.History) != 1 {
		t.Fatalf("history length = %d, want 1", len(after.History))
	}

	// Second move proves the position from the first survived the round trip.
	if _, err := svc.MakeMove(ctx, created.ID, "black-1", "e5"); err != nil {
		t.Fatalf("second move: %v", err)
	}
}

// Bug 2: the archive branch was commented out, so finished games were never
// written to Mongo and simply expired out of Redis.
func TestFinishedGameIsArchived(t *testing.T) {
	_, arch, svc := newHarness()
	ctx := context.Background()

	created, _ := svc.CreateGame(ctx, "white-1", "black-1", domain.TimeControl{InitialTime: 300})
	for _, m := range foolsMate {
		if _, err := svc.MakeMove(ctx, created.ID, m.player, m.move); err != nil {
			t.Fatalf("%s: %v", m.move, err)
		}
	}

	if arch.count() != 1 {
		t.Fatalf("archived %d games, want 1", arch.count())
	}
	if arch.archived[0] != created.ID {
		t.Errorf("archived the wrong game")
	}
}

func TestUnfinishedGameIsNotArchived(t *testing.T) {
	_, arch, svc := newHarness()
	ctx := context.Background()

	created, _ := svc.CreateGame(ctx, "white-1", "black-1", domain.TimeControl{InitialTime: 300})
	if _, err := svc.MakeMove(ctx, created.ID, "white-1", "e4"); err != nil {
		t.Fatalf("MakeMove: %v", err)
	}

	if arch.count() != 0 {
		t.Errorf("archived %d games mid-play, want 0", arch.count())
	}
}

// Losing the archive must not lose the move that ended the game.
func TestArchiveFailureDoesNotFailTheMove(t *testing.T) {
	repo, arch, svc := newHarness()
	arch.err = errors.New("mongo is down")
	ctx := context.Background()

	created, _ := svc.CreateGame(ctx, "white-1", "black-1", domain.TimeControl{InitialTime: 300})
	var last *domain.Game
	for _, m := range foolsMate {
		g, err := svc.MakeMove(ctx, created.ID, m.player, m.move)
		if err != nil {
			t.Fatalf("%s: %v", m.move, err)
		}
		last = g
	}

	if !last.IsFinished {
		t.Error("game should still be marked finished")
	}
	if _, ok := repo.games[created.ID]; !ok {
		t.Error("finished game should remain readable until its TTL expires")
	}
}

func TestMakeMoveOnMissingGame(t *testing.T) {
	_, _, svc := newHarness()
	if _, err := svc.MakeMove(context.Background(), uuid.New(), "white-1", "e4"); err == nil {
		t.Fatal("expected an error for an unknown game")
	}
}

func TestGetGameReturnsPlayablePosition(t *testing.T) {
	_, _, svc := newHarness()
	ctx := context.Background()

	created, _ := svc.CreateGame(ctx, "white-1", "black-1", domain.TimeControl{InitialTime: 300})
	svc.MakeMove(ctx, created.ID, "white-1", "e4")

	got, err := svc.GetGame(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetGame: %v", err)
	}
	if got.GetFEN() != got.CurrentFEN {
		t.Errorf("engine position %q does not match stored FEN %q", got.GetFEN(), got.CurrentFEN)
	}
}

// Two players moving at once must not lose a move to a read-modify-write race.
// Either ordering is legal chess, so the invariant is that every move the
// service accepted is present in the stored history — none silently vanish.
func TestConcurrentMovesAreNotLost(t *testing.T) {
	_, _, svc := newHarness()
	ctx := context.Background()

	created, _ := svc.CreateGame(ctx, "white-1", "black-1", domain.TimeControl{InitialTime: 300})

	var (
		mu        sync.Mutex
		succeeded int
		wg        sync.WaitGroup
	)
	record := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if err == nil {
			succeeded++
		}
	}

	wg.Add(2)
	go func() { defer wg.Done(); _, err := svc.MakeMove(ctx, created.ID, "white-1", "e4"); record(err) }()
	go func() { defer wg.Done(); _, err := svc.MakeMove(ctx, created.ID, "black-1", "e5"); record(err) }()
	wg.Wait()

	got, err := svc.GetGame(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetGame: %v", err)
	}
	if succeeded == 0 {
		t.Fatal("both moves were rejected; white's opening should always be legal")
	}
	if len(got.History) != succeeded {
		t.Errorf("history has %d moves but %d were accepted: a move was lost", len(got.History), succeeded)
	}
	// The stored position must match the moves that were actually played.
	if got.GetFEN() != got.CurrentFEN {
		t.Errorf("engine position %q does not match stored FEN %q", got.GetFEN(), got.CurrentFEN)
	}
}

// A longer concurrent burst gives the race far more chances to show up.
func TestConcurrentMoveBurstKeepsHistoryConsistent(t *testing.T) {
	_, _, svc := newHarness()
	ctx := context.Background()

	created, _ := svc.CreateGame(ctx, "white-1", "black-1", domain.TimeControl{InitialTime: 300})

	moves := []struct{ player, move string }{
		{"white-1", "e4"}, {"black-1", "e5"}, {"white-1", "Nf3"}, {"black-1", "Nc6"},
		{"white-1", "Bb5"}, {"black-1", "a6"}, {"white-1", "Ba4"}, {"black-1", "Nf6"},
	}

	var (
		mu        sync.Mutex
		succeeded int
		wg        sync.WaitGroup
	)
	for _, m := range moves {
		wg.Add(1)
		go func(player, move string) {
			defer wg.Done()
			if _, err := svc.MakeMove(ctx, created.ID, player, move); err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}(m.player, m.move)
	}
	wg.Wait()

	got, err := svc.GetGame(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetGame: %v", err)
	}
	if len(got.History) != succeeded {
		t.Errorf("history has %d moves but %d were accepted: a move was lost", len(got.History), succeeded)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(svc.locks) != 0 {
		t.Errorf("lock table leaked %d entries", len(svc.locks))
	}
}
