package redis

import (
	"context"
	"encoding/json"
	"errors"
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
	data, err := json.Marshal(redisGameModel{Game: game, FEN: game.GetFEN()})
	if err != nil {
		return err
	}
	return r.client.Set(ctx, "game:"+game.ID.String(), data, 24*time.Hour).Err()
}

func (r *RedisGameRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Game, error) {
	data, err := r.client.Get(ctx, "game:"+id.String()).Bytes()
	if err != nil {
		return nil, err
	}
	var model redisGameModel
	if err := json.Unmarshal(data, &model); err != nil {
		return nil, err
	}
	if model.Game == nil {
		return nil, errors.New("corrupt game record in redis")
	}
	if err := model.Game.RehydrateEngine(model.FEN); err != nil {
		return nil, err
	}
	return model.Game, nil
}

func (r *RedisGameRepository) Update(ctx context.Context, game *domain.Game) error {
	return r.Save(ctx, game)
}

func (r *RedisGameRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.client.Del(ctx, "game:"+id.String()).Err()
}
