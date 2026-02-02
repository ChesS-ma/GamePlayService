package main

import (
	"context"
	"log"
	"net/http"
	"time"

	gamehttp "github.com/ChesS-ma/gameplay_service/internal/adapters/handler/http"
	"github.com/ChesS-ma/gameplay_service/internal/adapters/repository/mongodb"
	"github.com/ChesS-ma/gameplay_service/internal/adapters/repository/redis"
	"github.com/ChesS-ma/gameplay_service/internal/core/services"
	goredis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Initialize Redis
	rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	redisRepo := redis.NewRedisGameRepository(rdb)

	// 2. Initialize MongoDB
	mClient, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatal(err)
	}
	mongoRepo := mongodb.NewMongoArchiveRepository(mClient)

	// 3. Initialize Service (Injecting BOTH repos)
	gameService := services.NewService(redisRepo, mongoRepo)

	// 4. Initialize Handler
	gameHandler := gamehttp.NewGameHandler(gameService)

	// 5. Routes
	http.HandleFunc("/games/create", gameHandler.CreateGame)
	http.HandleFunc("/games/move", gameHandler.MakeMove)
	http.HandleFunc("/games/get", gameHandler.GetGame)

	log.Println("Chess Service running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
