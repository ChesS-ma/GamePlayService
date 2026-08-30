//go:build integration

// Package tests holds integration tests: they drive gameplay_service from the
// outside, over real Redis, real MongoDB, and a real HTTP/WebSocket server.
//
// Unit tests live next to the code they test, as Go expects. These do not,
// because they test the wiring rather than any single package.
//
//	docker compose up -d
//	go test -tags=integration ./tests/...
package tests

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	gamehttp "github.com/ChesS-ma/gameplay_service/internal/adapters/handler/http"
	"github.com/ChesS-ma/gameplay_service/internal/adapters/repository/mongodb"
	redisrepo "github.com/ChesS-ma/gameplay_service/internal/adapters/repository/redis"
	"github.com/ChesS-ma/gameplay_service/internal/core/ports"
	"github.com/ChesS-ma/gameplay_service/internal/core/services"
	goredis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// stack is a fully wired service backed by real infrastructure.
type stack struct {
	Service ports.GameService
	Redis   *goredis.Client
	Mongo   *mongo.Client
	Archive *mongo.Collection
}

// newStack connects to Redis and MongoDB, or skips the test if either is not
// running. Integration tests should be skippable, never a mysterious failure.
func newStack(t *testing.T) *stack {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rdb := goredis.NewClient(&goredis.Options{Addr: envOr("REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		t.Skipf("redis unavailable (%v) — run: docker compose up -d", err)
	}

	mClient, err := mongo.Connect(ctx, options.Client().ApplyURI(envOr("MONGO_URI", "mongodb://localhost:27017")))
	if err != nil {
		rdb.Close()
		t.Skipf("mongo unreachable (%v) — run: docker compose up -d", err)
	}
	if err := mClient.Ping(ctx, nil); err != nil {
		rdb.Close()
		t.Skipf("mongo unavailable (%v) — run: docker compose up -d", err)
	}

	s := &stack{
		Service: services.NewService(redisrepo.NewRedisGameRepository(rdb), mongodb.NewMongoArchiveRepository(mClient)),
		Redis:   rdb,
		Mongo:   mClient,
		Archive: mClient.Database("chessma").Collection("archives"),
	}

	t.Cleanup(func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.purge(clean)
		rdb.Close()
		mClient.Disconnect(clean)
	})

	// Start from a clean slate so a previous run cannot mask a failure.
	s.purge(ctx)
	return s
}

// purge removes only what these tests create, never the whole database.
func (s *stack) purge(ctx context.Context) {
	keys, err := s.Redis.Keys(ctx, "game:*").Result()
	if err == nil && len(keys) > 0 {
		s.Redis.Del(ctx, keys...)
	}
	s.Archive.DeleteMany(ctx, bson.M{"white_id": bson.M{"$regex": "^it-"}})
}

// archived reads a game back out of the Mongo archive.
func (s *stack) archived(ctx context.Context, gameID string) (bson.M, error) {
	var doc bson.M
	err := s.Archive.FindOne(ctx, bson.M{"_id": gameID}).Decode(&doc)
	return doc, err
}

// server starts the real HTTP + WebSocket handlers and returns its ws:// base.
func (s *stack) server(t *testing.T) (*httptest.Server, string) {
	t.Helper()

	handler := gamehttp.NewGameHandler(s.Service)
	ws := gamehttp.NewWsHandler(s.Service)

	srv := httptest.NewServer(gamehttp.NewRouter(handler, ws))
	t.Cleanup(srv.Close)

	return srv, "ws" + strings.TrimPrefix(srv.URL, "http")
}

// foolsMate is the shortest possible checkmate: four moves, black wins.
var foolsMate = []struct{ Player, Move string }{
	{"white", "f3"}, {"black", "e5"}, {"white", "g4"}, {"black", "Qh4"},
}
