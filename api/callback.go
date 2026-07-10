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
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Line-Signature")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	channelSecret := strings.TrimSpace(os.Getenv("LINE_CHANNEL_SECRET"))
	channelToken := strings.TrimSpace(os.Getenv("LINE_CHANNEL_TOKEN"))
	if channelSecret == "" || channelToken == "" {
		log.Println("ERROR: LINE credentials not configured")
		http.Error(w, "LINE credentials not configured", http.StatusInternalServerError)
		return
	}

	bot, err := linebot.New(channelSecret, channelToken)
	if err != nil {
		log.Printf("ERROR: Failed to create LINE bot: %v", err)
		http.Error(w, "Failed to create LINE bot: "+err.Error(), http.StatusInternalServerError)
		return
	}

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

	mongoURI := strings.TrimSpace(os.Getenv("MONGODB_URI"))
	if mongoURI == "" {
		log.Println("ERROR: MongoDB URI not configured")
		http.Error(w, "MongoDB URI not configured", http.StatusInternalServerError)
		return
	}

	st, err := store.NewStore(mongoURI)
	if err != nil {
		log.Printf("ERROR: Database connection failed: %v", err)
		http.Error(w, "Database connection failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, event := range events {
		switch event.Type {
		case linebot.EventTypeFollow:
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

				displayName, pictureURL := "", ""
				if profile, profileErr := bot.GetProfile(userID).Do(); profileErr == nil {
					displayName = profile.DisplayName
					pictureURL = profile.PictureURL
				}
				if err := st.UpsertMember(ctx, userID, displayName, pictureURL); err != nil {
					log.Printf("ERROR: UpsertMember failed: %v", err)
				}

				if matched, _ := regexp.MatchString(`^\d{4}$`, text); matched {
					err = st.VerifyCode(ctx, text, userID, displayName, pictureURL)
					if err != nil {
						log.Printf("ERROR: VerifyCode failed: %v", err)
						replyText(bot, event.ReplyToken, "รหัสไม่ถูกต้องหรือหมดอายุแล้ว")
					} else {
						replyText(bot, event.ReplyToken, "ยืนยันตัวตนสำเร็จแล้ว\n\nกรุณากลับไปที่แอปเพื่อดำเนินการต่อ")
					}
				} else if isUIDCommand(text) {
					replyText(bot, event.ReplyToken, fmt.Sprintf("LINE UID ของคุณคือ:\n\n%s\n\nคัดลอกไปใช้ในระบบ Admin ได้เลย", userID))
				} else if isPointCommand(text) {
					response := getPointSummary(ctx, userID, st)
					replyText(bot, event.ReplyToken, response)
				} else {
					aiResponse := generateAIReply(ctx, text, userID, st)
					replyText(bot, event.ReplyToken, aiResponse)
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

func isUIDCommand(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	return text == "uid" || text == "ไอดี" || text == "id" || text == "line uid"
}

func isPointCommand(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	switch text {
	case "แต้ม", "แต้มสะสม", "แต้มคงเหลือ", "เช็คแต้ม", "ดูแต้ม", "คะแนน", "พ้อยท์", "พอยท์", "point", "points", "balance":
		return true
	default:
		return false
	}
}

func generateAIReply(ctx context.Context, text, userID string, st *store.Store) string {
	geminiAPIKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if geminiAPIKey == "" {
		return fallbackHelpMessage()
	}

	history, err := st.GetChatHistory(ctx, userID, 6)
	if err != nil {
		log.Printf("ERROR: GetChatHistory failed: %v", err)
	}

	aiHistory := make([]ai.ChatMessage, len(history))
	for i, msg := range history {
		aiHistory[i] = ai.ChatMessage{Role: msg.Role, Message: msg.Message}
	}

	aiService := ai.NewGeminiService(geminiAPIKey)
	aiResponse, err := aiService.GenerateResponseWithHistory(text, aiHistory)
	if err != nil {
		log.Printf("ERROR: GenerateResponseWithHistory failed: %v", err)
		return fallbackHelpMessage()
	}

	if err := st.SaveChatMessage(ctx, userID, "user", text); err != nil {
		log.Printf("ERROR: SaveChatMessage (user) failed: %v", err)
	}
	if err := st.SaveChatMessage(ctx, userID, "assistant", aiResponse); err != nil {
		log.Printf("ERROR: SaveChatMessage (assistant) failed: %v", err)
	}
	return aiResponse
}

func fallbackHelpMessage() string {
	return "พิมพ์ \"แต้มคงเหลือ\" เพื่อดูแต้มสะสม หรือพิมพ์ \"uid\" เพื่อดู LINE UID ของคุณ"
}

func replyText(bot *linebot.Client, token, text string) {
	if _, err := bot.ReplyMessage(token, linebot.NewTextMessage(text)).Do(); err != nil {
		log.Printf("ERROR: ReplyMessage failed: %v", err)
	}
}

// getPointSummary retrieves central point summary from members collection
func getPointSummary(ctx context.Context, lineUID string, st *store.Store) string {
	member, err := st.GetMemberByLineUID(ctx, lineUID)
	if err != nil {
		log.Printf("ERROR: GetMemberByLineUID failed: %v", err)
		return "เกิดข้อผิดพลาดในการดึงข้อมูลแต้มสะสม กรุณาลองใหม่อีกครั้ง"
	}
	if member == nil {
		return "คุณยังไม่มีแต้มสะสม"
	}

	var sb strings.Builder
	sb.WriteString("แต้มสะสมของคุณ\n\n")
	sb.WriteString(fmt.Sprintf("แต้มคงเหลือ: %.0f แต้ม\n", member.PointBalance))
	sb.WriteString(fmt.Sprintf("แต้มสะสมทั้งหมด: %.0f แต้ม\n", member.TotalEarned))
	sb.WriteString(fmt.Sprintf("ระดับสมาชิก: %s", member.Tier))
	return sb.String()
}
