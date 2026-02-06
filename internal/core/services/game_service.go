package services

import (
	"context"
	"github.com/ChesS-ma/gameplay_service/internal/core/domain"
	"github.com/ChesS-ma/gameplay_service/internal/core/ports"
	"github.com/google/uuid"
)

type service struct {
	repo    ports.GameRepository        // Usually Redis
	archive ports.GameArchiveRepository // Usually MongoDB
}

func NewService(repo ports.GameRepository, archive ports.GameArchiveRepository) ports.GameService {
	return &service{
		repo:    repo,
		archive: archive,
	}
}

func (s *service) CreateGame(ctx context.Context, whiteId, blackId string, tc domain.TimeControl) (*domain.Game, error) {
	newGame := domain.NewGame(whiteId, blackId, tc)

	if err := s.repo.Save(ctx, newGame); err != nil {
		return nil, err
	}
	return newGame, nil
}

//	func (s *service) MakeMove(ctx context.Context, gameId uuid.UUID, playerID string, moveNotation string) (*domain.Game, error) {
//		// 1. Fetch current state from Redis
//		game, err := s.repo.FindByID(ctx, gameId)
//		if err != nil {
//			return nil, err
//		}
//
//		// 2. Apply move (Business Logic inside Domain)
//		if err := game.MakeMove(playerID, moveNotation); err != nil {
//			return nil, err
//		}
//
//		// 3. Orchestrate Persistence
//		if game.IsGameOver() {
//			// Store in MongoDB permanently
//			_ = s.archive.Archive(ctx, game)
//			// Clean up Redis
//			_ = s.repo.Delete(ctx, gameId)
//		} else {
//			// Update Redis for next move
//			if err := s.repo.Update(ctx, game); err != nil {
//				return nil, err
//			}
//		}
//
//		return game, nil
//	}
func (s *service) MakeMove(ctx context.Context, gameId uuid.UUID, playerID string, moveNotation string) (*domain.Game, error) {
	game, err := s.repo.FindByID(ctx, gameId)
	if err != nil {
		return nil, err
	}

	// --- REHYDRATE ENGINE ---
	fen := game.CurrentFEN
	if fen == "" {
		fen = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
	}
	game.RehydrateEngine(fen) // Rebuild the chess logic engine
	// ------------------------

	if err := game.MakeMove(playerID, moveNotation); err != nil {
		return nil, err
	}

	// ... persistence logic (Update Redis or Archive) ...
	s.repo.Save(ctx, game)
	return game, nil
}
func (s *service) GetGame(ctx context.Context, gameId uuid.UUID) (*domain.Game, error) {
	game, err := s.repo.FindByID(ctx, gameId)
	if err != nil {
		return nil, err
	}

	// Use the stored FEN to rebuild the engine
	if game.CurrentFEN == "" {
		game.CurrentFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"
	}
	game.RehydrateEngine(game.CurrentFEN)

	return game, nil
}
