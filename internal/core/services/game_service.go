package services

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/ChesS-ma/gameplay_service/internal/core/domain"
	"github.com/ChesS-ma/gameplay_service/internal/core/ports"
	"github.com/google/uuid"
)

type service struct {
	repo    ports.GameRepository        // Usually Redis
	archive ports.GameArchiveRepository // Usually MongoDB

	mu    sync.Mutex
	locks map[uuid.UUID]*gameLock
}

// gameLock serializes moves for a single game. Entries are reference counted
// so the map does not grow for the lifetime of the process.
type gameLock struct {
	mu   sync.Mutex
	refs int
}

// lockGame blocks until this game is free and returns its release function.
func (s *service) lockGame(id uuid.UUID) func() {
	s.mu.Lock()
	l, ok := s.locks[id]
	if !ok {
		l = &gameLock{}
		s.locks[id] = l
	}
	l.refs++
	s.mu.Unlock()

	l.mu.Lock()

	return func() {
		l.mu.Unlock()

		s.mu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(s.locks, id)
		}
		s.mu.Unlock()
	}
}

func NewService(repo ports.GameRepository, archive ports.GameArchiveRepository) ports.GameService {
	return &service{
		repo:    repo,
		archive: archive,
		locks:   make(map[uuid.UUID]*gameLock),
	}
}

func (s *service) CreateGame(ctx context.Context, whiteId, blackId string, tc domain.TimeControl) (*domain.Game, error) {
	newGame := domain.NewGame(whiteId, blackId, tc)

	if err := s.repo.Save(ctx, newGame); err != nil {
		return nil, err
	}
	return newGame, nil
}

func (s *service) MakeMove(ctx context.Context, gameId uuid.UUID, playerID string, moveNotation string) (*domain.Game, error) {
	// One game is mutated by two players over separate connections. Without a
	// per-game lock the read-modify-write against Redis loses moves.
	unlock := s.lockGame(gameId)
	defer unlock()

	game, err := s.repo.FindByID(ctx, gameId)
	if err != nil {
		return nil, err
	}

	// The chess engine is unexported and does not survive serialization, so it
	// has to be rebuilt from the stored FEN before the domain can validate.
	if err := game.RehydrateEngine(game.CurrentFEN); err != nil {
		return nil, fmt.Errorf("restoring game %s: %w", gameId, err)
	}

	if err := game.MakeMove(playerID, moveNotation); err != nil {
		return nil, err
	}

	// Keep the finished game in Redis so clients can still read the final
	// position; its TTL cleans it up. Mongo holds the permanent record.
	if err := s.repo.Update(ctx, game); err != nil {
		return nil, fmt.Errorf("saving game %s: %w", gameId, err)
	}

	if game.IsFinished {
		// A failed archive must not fail the move that just ended the game.
		if err := s.archive.Archive(ctx, game); err != nil {
			log.Printf("archiving game %s failed: %v", gameId, err)
		}
	}

	return game, nil
}

func (s *service) GetGame(ctx context.Context, gameId uuid.UUID) (*domain.Game, error) {
	game, err := s.repo.FindByID(ctx, gameId)
	if err != nil {
		return nil, err
	}

	if err := game.RehydrateEngine(game.CurrentFEN); err != nil {
		return nil, fmt.Errorf("restoring game %s: %w", gameId, err)
	}

	return game, nil
}
