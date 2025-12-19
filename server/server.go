package server

import (
	"bcmemberapi/ai"
	"bcmemberapi/store"

	"github.com/line/line-bot-sdk-go/v7/linebot"
)

// Server holds all dependencies for the HTTP server
type Server struct {
	Store     *store.Store
	Bot       *linebot.Client
	AIService *ai.GeminiService
}

// New creates a new Server with all dependencies
func New(st *store.Store, bot *linebot.Client, aiService *ai.GeminiService) *Server {
	return &Server{
		Store:     st,
		Bot:       bot,
		AIService: aiService,
	}
}
