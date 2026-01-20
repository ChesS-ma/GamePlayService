package ports

import (
	"context"
	"github.com/ChesS-ma/gameplay_service/internal/core/domain"
	"github.com/google/uuid"
)

// GameRepository is a "Driven Port". It defines what we need from our database.
type GameRepository interface {
	Save(ctx context.Context, game *domain.Game) error
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Game, error)
	Update(ctx context.Context, game *domain.Game) error
}

// GameService is a "Driving Port". It defines what the application can actually DO.
type GameService interface {
	CreateGame(ctx context.Context, whiteId, blackId string) (*domain.Game, error)
	MakeMove(ctx context.Context, gameId uuid.UUID, move domain.Move) (*domain.Game, error)
	GetGame(ctx context.Context, gameId uuid.UUID) (*domain.Game, error)
}
