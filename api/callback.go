package handler

import (
	"context"
	"fmt"
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
func Handler(w http.ResponseWriter, r *http.Request) {
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
	channelSecret := strings.TrimSpace(os.Getenv("LINE_CHANNEL_SECRET"))
	channelToken := strings.TrimSpace(os.Getenv("LINE_CHANNEL_TOKEN"))

	if channelSecret == "" || channelToken == "" {
		log.Println("ERROR: LINE credentials not configured")
		http.Error(w, "LINE credentials not configured", http.StatusInternalServerError)
		return
	}

	// Create LINE bot client
	bot, err := linebot.New(channelSecret, channelToken)
	if err != nil {
		log.Printf("ERROR: Failed to create LINE bot: %v", err)
		http.Error(w, "Failed to create LINE bot: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse LINE request
	events, err := bot.ParseRequest(r)
	if err != nil {
		log.Printf("ERROR: ParseRequest failed: %v", err)
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
		log.Println("ERROR: MongoDB URI not configured")
		http.Error(w, "MongoDB URI not configured", http.StatusInternalServerError)
		return
	}

	// Create MongoDB store
	st, err := store.NewStore(mongoURI)
	if err != nil {
		log.Printf("ERROR: Database connection failed: %v", err)
		http.Error(w, "Database connection failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get Gemini API key
	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		log.Println("ERROR: Gemini API key not configured")
		http.Error(w, "Gemini API key not configured", http.StatusInternalServerError)
		return
	}

	// Create AI service
	aiService := ai.NewGeminiService(geminiAPIKey)

	// Process each event
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, event := range events {
		switch event.Type {
		case linebot.EventTypeFollow:
			// User added bot as friend — save profile immediately
			userID := event.Source.UserID
			if profile, err := bot.GetProfile(userID).Do(); err == nil {
				if err := st.UpsertMember(ctx, userID, profile.DisplayName, profile.PictureURL); err != nil {
					log.Printf("ERROR: UpsertMember on follow failed: %v", err)
				}
				log.Printf("New follower: %s (%s)", profile.DisplayName, userID)
			}

		case linebot.EventTypeMessage:
			switch message := event.Message.(type) {
			case *linebot.TextMessage:
				text := strings.TrimSpace(message.Text)
				userID := event.Source.UserID

				// Get profile and update member on every interaction
				profile, profileErr := bot.GetProfile(userID).Do()
				displayName := ""
				pictureURL := ""
				if profileErr == nil {
					displayName = profile.DisplayName
					pictureURL = profile.PictureURL
				}
				if err := st.UpsertMember(ctx, userID, displayName, pictureURL); err != nil {
					log.Printf("ERROR: UpsertMember failed: %v", err)
				}

				// Check if it's a 4-digit code
				if matched, _ := regexp.MatchString(`^\d{4}$`, text); matched {
					// It's a login code
					err = st.VerifyCode(ctx, text, userID, displayName, pictureURL)
					if err != nil {
						log.Printf("ERROR: VerifyCode failed: %v", err)
						replyText(bot, event.ReplyToken, "รหัสไม่ถูกต้องหรือหมดอายุแล้ว")
					} else {
						replyText(bot, event.ReplyToken, "✅ ยืนยันตัวตนสำเร็จแล้ว! \n\nกรุณากลับไปที่แอพเพื่อดำเนินการต่อ")
					}
				} else if text == "แต้มสะสม" || text == "แต้ม" || text == "พ้อยท์" || text == "point" || text == "points" {
					// Check points command
					response := getPointSummary(ctx, userID, st)
					replyText(bot, event.ReplyToken, response)
				} else {
					// AI Chatbot logic
					// Get chat history (last 6 messages = 3 conversation pairs to save AI tokens)
					history, err := st.GetChatHistory(ctx, userID, 6)
					if err != nil {
						log.Printf("ERROR: GetChatHistory failed: %v", err)
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
						log.Printf("ERROR: GenerateResponseWithHistory failed: %v", err)
						aiResponse = "ขอโทษครับ เกิดข้อผิดพลาดในการประมวลผล กรุณาลองใหม่อีกครั้ง"
					}

					// Save user message
					if err := st.SaveChatMessage(ctx, userID, "user", text); err != nil {
						log.Printf("ERROR: SaveChatMessage (user) failed: %v", err)
					}

					// Save AI response
					if err := st.SaveChatMessage(ctx, userID, "assistant", aiResponse); err != nil {
						log.Printf("ERROR: SaveChatMessage (assistant) failed: %v", err)
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
		log.Printf("ERROR: ReplyMessage failed: %v", err)
	}
}

// getPointSummary retrieves central point summary from members collection
func getPointSummary(ctx context.Context, lineUID string, st *store.Store) string {
	// Get central member record
	member, err := st.GetMemberByLineUID(ctx, lineUID)
	if err != nil {
		log.Printf("ERROR: GetMemberByLineUID failed: %v", err)
		return "❌ เกิดข้อผิดพลาดในการดึงข้อมูลแต้มสะสม"
	}

	// No points found
	if member == nil {
		return "📋 คุณยังไม่มีแต้มสะสม"
	}

	// Build response message
	var sb strings.Builder
	sb.WriteString("✨ แต้มสะสมของคุณ\n\n")
	sb.WriteString(fmt.Sprintf("💰 แต้มรวม: %.0f แต้ม", member.PointBalance))

	return sb.String()
}
