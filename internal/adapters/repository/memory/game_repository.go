package memory

import (
	"context"
	"errors"
	"github.com/ChesS-ma/gameplay_service/internal/core/domain"
	"github.com/google/uuid"
	"sync"
)

type INMemoryGameRepository struct {
	games map[uuid.UUID]*domain.Game
	mu    sync.RWMutex //safety lock for concurrent access
}

func NewInMemoryGameRepository() *INMemoryGameRepository {
	return &INMemoryGameRepository{
		games: make(map[uuid.UUID]*domain.Game),
	}
}
func (r *INMemoryGameRepository) Save(ctx context.Context, game *domain.Game) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.games[game.ID] = game
	return nil
}
func (r *INMemoryGameRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Game, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	game, exists := r.games[id]
	if !exists {
		return nil, errors.New("game not found")
	}
	return game, nil
}

func (r *INMemoryGameRepository) Update(ctx context.Context, game *domain.Game) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.games[game.ID] = game
	return nil

}
