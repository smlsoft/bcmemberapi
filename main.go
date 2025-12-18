package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"bcmemberapi/ai"
	"bcmemberapi/store"

	"github.com/joho/godotenv"
	"github.com/line/line-bot-sdk-go/v7/linebot"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	// Initialize MongoDB
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Fatal("MONGODB_URI is required")
	}
	st, err := store.NewStore(mongoURI)
	if err != nil {
		log.Fatal(err)
	}

	bot, err := linebot.New(
		os.Getenv("LINE_CHANNEL_SECRET"),
		os.Getenv("LINE_CHANNEL_TOKEN"),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize Gemini AI
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		log.Fatal("GEMINI_API_KEY is required")
	}
	aiService := ai.NewGeminiService(geminiAPIKey)

	// API: Generate Code
	http.HandleFunc("/api/login/code", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			ShopID string `json:"shop_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		code, err := st.GenerateCode(r.Context(), req.ShopID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"code": code})
	})

	// API: Check Code Status
	http.HandleFunc("/api/login/status", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "Code is required", http.StatusBadRequest)
			return
		}
		result, err := st.GetCodeStatus(r.Context(), code)
		if err != nil {
			http.Error(w, "Code not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	// LINE Webhook
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		events, err := bot.ParseRequest(r)
		if err != nil {
			if err == linebot.ErrInvalidSignature {
				w.WriteHeader(400)
			} else {
				w.WriteHeader(500)
			}
			return
		}
		for _, event := range events {
			if event.Type == linebot.EventTypeMessage {
				switch message := event.Message.(type) {
				case *linebot.TextMessage:
					text := strings.TrimSpace(message.Text)
					if matched, _ := regexp.MatchString(`^\d{4}$`, text); matched {
						// It's a 4-digit code
						profile, err := bot.GetProfile(event.Source.UserID).Do()
						displayName := ""
						if err == nil {
							displayName = profile.DisplayName
						}

						err = st.VerifyCode(r.Context(), text, event.Source.UserID, displayName)
						if err != nil {
							replyText(bot, event.ReplyToken, "Invalid or expired code.")
						} else {
							replyText(bot, event.ReplyToken, "Login Successful! You can now return to the website.")
						}
					} else {
						// AI Chatbot logic
						userID := event.Source.UserID

						// Get chat history (last 6 messages = 3 conversation pairs to save AI tokens)
						history, err := st.GetChatHistory(r.Context(), userID, 6)
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
						if err := st.SaveChatMessage(r.Context(), userID, "user", text); err != nil {
							log.Printf("Error saving user message: %v", err)
						}

						// Save AI response
						if err := st.SaveChatMessage(r.Context(), userID, "assistant", aiResponse); err != nil {
							log.Printf("Error saving AI response: %v", err)
						}

						// Reply to user
						replyText(bot, event.ReplyToken, aiResponse)
					}
				}
			}
		}
	})

	log.Println("Server running on port " + os.Getenv("PORT"))
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), nil); err != nil {
		log.Fatal(err)
	}
}

func replyText(bot *linebot.Client, token, text string) {
	if _, err := bot.ReplyMessage(token, linebot.NewTextMessage(text)).Do(); err != nil {
		log.Print(err)
	}
}
