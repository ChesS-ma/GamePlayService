package services

import (
	"context"
	"errors"
	"github.com/ChesS-ma/gameplay_service/internal/core/domain"
	"github.com/ChesS-ma/gameplay_service/internal/core/ports"
	"github.com/google/uuid"
	"time"
)

type service struct {
	repo ports.GameRepository
}

func (s *service) NewService(repo ports.GameRepository) ports.GameService {
	return &service{
		repo: repo,
	}
}

func (s *service) CreateGame(ctx context.Context, WhiteID, BlackId string) (*domain.Game, error) {
	newGame := &domain.Game{
		ID:        uuid.New(),
		Board:     "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1", // Standard starting FEN
		State:     domain.GameState{IsOver: false, Turn: "white"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := s.repo.Save(ctx, newGame); err != nil {
		return nil, err
	}
	return newGame, nil
}

func (s *service) MakeMove(ctx context.Context, gameId uuid.UUID, move domain.Move) (*domain.Game, error) {
	return nil, errors.New("move logic not implemented yet")

}
func (s *service) GetGame(ctx context.Context, gameId uuid.UUID) (*domain.Game, error) {
	game, err := s.repo.FindByID(ctx, gameId)

	if err != nil {
		return nil, err
	}
	return game, nil
}
