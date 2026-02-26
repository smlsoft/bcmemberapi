package main

import (
	"log"
	"net/http"

	"bcmemberapi/ai"
	"bcmemberapi/config"
	"bcmemberapi/server"
	"bcmemberapi/store"

	"github.com/line/line-bot-sdk-go/v7/linebot"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// Initialize dependencies
	st, bot, aiService := initDependencies(cfg)

	// Create server and register routes
	srv := server.New(st, bot, aiService, cfg)
	srv.RegisterRoutes()

	// Start HTTP server
	log.Printf("Server running on port %s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatal(err)
	}
}

// initDependencies initializes all application dependencies
func initDependencies(cfg *config.Config) (*store.Store, *linebot.Client, *ai.GeminiService) {
	// Initialize MongoDB
	st, err := store.NewStore(cfg.MongoURI)
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	// Initialize LINE Bot
	bot, err := linebot.New(cfg.LineChannelSecret, cfg.LineChannelToken)
	if err != nil {
		log.Fatalf("Failed to create LINE bot: %v", err)
	}

	// Initialize Gemini AI
	aiService := ai.NewGeminiService(cfg.GeminiAPIKey)

	return st, bot, aiService
}
