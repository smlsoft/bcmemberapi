package handler

import (
	"context"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"bcmemberapi/ai"
	"bcmemberapi/store"

	"github.com/line/line-bot-sdk-go/v7/linebot"
)

// Handler for POST /api/callback (LINE Webhook)
func CallbackHandler(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Line-Signature")

	// Handle OPTIONS request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow POST
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get LINE credentials from environment
	channelSecret := os.Getenv("LINE_CHANNEL_SECRET")
	channelToken := os.Getenv("LINE_CHANNEL_TOKEN")
	if channelSecret == "" || channelToken == "" {
		http.Error(w, "LINE credentials not configured", http.StatusInternalServerError)
		return
	}

	// Create LINE bot client
	bot, err := linebot.New(channelSecret, channelToken)
	if err != nil {
		http.Error(w, "Failed to create LINE bot: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse LINE request
	events, err := bot.ParseRequest(r)
	if err != nil {
		if err == linebot.ErrInvalidSignature {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	// Get MongoDB URI
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		http.Error(w, "MongoDB URI not configured", http.StatusInternalServerError)
		return
	}

	// Create MongoDB store
	st, err := store.NewStore(mongoURI)
	if err != nil {
		http.Error(w, "Database connection failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get Gemini API key
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		http.Error(w, "Gemini API key not configured", http.StatusInternalServerError)
		return
	}

	// Create AI service
	aiService := ai.NewGeminiService(geminiAPIKey)

	// Process each event
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	for _, event := range events {
		if event.Type == linebot.EventTypeMessage {
			switch message := event.Message.(type) {
			case *linebot.TextMessage:
				text := strings.TrimSpace(message.Text)

				// Check if it's a 4-digit code
				if matched, _ := regexp.MatchString(`^\d{4}$`, text); matched {
					// It's a login code
					profile, err := bot.GetProfile(event.Source.UserID).Do()
					displayName := ""
					if err == nil {
						displayName = profile.DisplayName
					}

					err = st.VerifyCode(ctx, text, event.Source.UserID, displayName)
					if err != nil {
						replyText(bot, event.ReplyToken, "รหัสไม่ถูกต้องหรือหมดอายุแล้ว")
					} else {
						replyText(bot, event.ReplyToken, "✅ เข้าสู่ระบบสำเร็จ!\n\nกรุณากลับไปที่เว็บไซต์เพื่อดำเนินการต่อ")
					}
				} else {
					// AI Chatbot logic
					userID := event.Source.UserID

					// Get chat history (last 6 messages = 3 conversation pairs to save AI tokens)
					history, err := st.GetChatHistory(ctx, userID, 6)
					if err != nil {
						log.Printf("Error getting chat history: %v", err)
					}

					// Convert to AI message format
					aiHistory := make([]ai.ChatMessage, len(history))
					for i, msg := range history {
						aiHistory[i] = ai.ChatMessage{
							Role:    msg.Role,
							Message: msg.Message,
						}
					}

					// Generate AI response
					aiResponse, err := aiService.GenerateResponseWithHistory(text, aiHistory)
					if err != nil {
						log.Printf("Error generating AI response: %v", err)
						aiResponse = "ขอโทษครับ เกิดข้อผิดพลาดในการประมวลผล กรุณาลองใหม่อีกครั้ง"
					}

					// Save user message
					if err := st.SaveChatMessage(ctx, userID, "user", text); err != nil {
						log.Printf("Error saving user message: %v", err)
					}

					// Save AI response
					if err := st.SaveChatMessage(ctx, userID, "assistant", aiResponse); err != nil {
						log.Printf("Error saving AI response: %v", err)
					}

					// Reply to user
					replyText(bot, event.ReplyToken, aiResponse)
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func replyText(bot *linebot.Client, token, text string) {
	if _, err := bot.ReplyMessage(token, linebot.NewTextMessage(text)).Do(); err != nil {
		log.Print(err)
	}
}
