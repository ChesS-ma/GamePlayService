package mongodb

import (
	"context"
	"time"

	"github.com/ChesS-ma/gameplay_service/internal/core/domain"
	"go.mongodb.org/mongo-driver/mongo"
)

type MongoArchiveRepository struct {
	collection *mongo.Collection
}

func NewMongoArchiveRepository(client *mongo.Client) *MongoArchiveRepository {
	return &MongoArchiveRepository{
		// This will automatically create the 'chessma' db and 'archives' collection if they don't exist
		collection: client.Database("chessma").Collection("archives"),
	}
}

func (r *MongoArchiveRepository) Archive(ctx context.Context, game *domain.Game) error {
	// 1. Create a dedicated context with a timeout for the DB operation
	// This prevents a slow DB from hanging your entire service
	archiveCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 2. Map the domain object to a BSON-friendly structure
	// We do this explicitly to control exactly how the history is stored
	doc := map[string]interface{}{
		"_id":       game.ID.String(), // Using ID string as the MongoDB Primary Key
		"white_id":  game.White.UserID,
		"black_id":  game.Black.UserID,
		"board_fen": game.GetFEN(),
		"history":   game.History,
		"result":    game.GetResult(),
		"settings": map[string]interface{}{
			"initial_time": game.Settings.InitialTime,
			"increment":    game.Settings.Increment,
		},
		"created_at":  game.CreatedAt,
		"archived_at": time.Now(),
	}

	// 3. Execute the insert
	_, err := r.collection.InsertOne(archiveCtx, doc)
	return err
}
