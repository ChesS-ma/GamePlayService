package main

import (
	"github.com/ChesS-ma/gameplay_service/internal/core/services"
	"log"
	"net/http"

	handler "github.com/ChesS-ma/gameplay_service/internal/adapters/handler/http"
	"github.com/ChesS-ma/gameplay_service/internal/adapters/repository/memory"
)

func main() {
	// 1. Initialize the Adapter (Repository)
	repo := memory.NewInMemoryGameRepository()

	// 2. Initialize the Service (injecting the repo)
	gameService := services.NewService(repo)

	// 3. Initialize the Handler (injecting the service)
	gameHandler := handler.NewGameHandler(gameService)

	// 4. Register Routes
	http.HandleFunc("/games", gameHandler.CreateGame)

	// Keep the health check
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Gameplay Service is Up and Running!"))
	})

	port := ":8080"
	log.Printf("Starting server on port %s", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal(err)
	}
}
