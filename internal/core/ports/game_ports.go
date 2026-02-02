package ports

import (
	"context"
	"github.com/ChesS-ma/gameplay_service/internal/core/domain"
	"github.com/google/uuid"
)

type GameRepository interface {
	Save(ctx context.Context, game *domain.Game) error
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Game, error)
	Update(ctx context.Context, game *domain.Game) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// New Archive Port for MongoDB
type GameArchiveRepository interface {
	Archive(ctx context.Context, game *domain.Game) error
}

type GameService interface {
	CreateGame(ctx context.Context, whiteId, blackId string) (*domain.Game, error)
	// Updated to take playerID for turn validation
	MakeMove(ctx context.Context, gameId uuid.UUID, playerID string, moveNotation string) (*domain.Game, error)
	GetGame(ctx context.Context, gameId uuid.UUID) (*domain.Game, error)
}
