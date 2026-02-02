package redis

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ChesS-ma/gameplay_service/internal/core/domain"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type RedisGameRepository struct {
	client *redis.Client
}

func NewRedisGameRepository(client *redis.Client) *RedisGameRepository {
	return &RedisGameRepository{client: client}
}

// Internal wrapper to save the FEN since the engine field is private
type redisGameModel struct {
	*domain.Game
	FEN string `json:"fen"`
}

func (r *RedisGameRepository) Save(ctx context.Context, game *domain.Game) error {
	data, _ := json.Marshal(redisGameModel{Game: game, FEN: game.GetFEN()})
	return r.client.Set(ctx, "game:"+game.ID.String(), data, 24*time.Hour).Err()
}

func (r *RedisGameRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Game, error) {
	data, err := r.client.Get(ctx, "game:"+id.String()).Bytes()
	if err != nil {
		return nil, err
	}
	var model redisGameModel
	json.Unmarshal(data, &model)
	model.Game.RehydrateEngine(model.FEN)
	return model.Game, nil
}

func (r *RedisGameRepository) Update(ctx context.Context, game *domain.Game) error {
	return r.Save(ctx, game)
}

func (r *RedisGameRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.client.Del(ctx, "game:"+id.String()).Err()
}
