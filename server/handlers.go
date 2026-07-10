package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"bcmemberapi/ai"
	"bcmemberapi/store"

	"github.com/line/line-bot-sdk-go/v7/linebot"
)

// HandleGenerateCode handles POST /api/login/code
func (s *Server) HandleGenerateCode(w http.ResponseWriter, r *http.Request) {
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

	code, err := s.Store.GenerateCode(r.Context(), req.ShopID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"code": code})
}

// HandleCheckStatus handles GET /api/login/status
func (s *Server) HandleCheckStatus(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Code is required", http.StatusBadRequest)
		return
	}

	result, err := s.Store.GetCodeStatus(r.Context(), code)
	if err != nil {
		http.Error(w, "Code not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleLineCallback handles POST /callback (LINE Webhook)
func (s *Server) HandleLineCallback(w http.ResponseWriter, r *http.Request) {
	events, err := s.Bot.ParseRequest(r)
	if err != nil {
		if err == linebot.ErrInvalidSignature {
			w.WriteHeader(http.StatusBadRequest)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	for _, event := range events {
		switch event.Type {
		case linebot.EventTypeFollow:
			s.handleFollowEvent(r, event)
		case linebot.EventTypeMessage:
			switch message := event.Message.(type) {
			case *linebot.TextMessage:
				s.handleTextMessage(r, event, message)
			}
		}
	}
}

func (s *Server) handleFollowEvent(r *http.Request, event *linebot.Event) {
	userID := event.Source.UserID
	displayName, pictureURL := s.getUserProfile(userID)
	if err := s.Store.UpsertMember(r.Context(), userID, displayName, pictureURL); err != nil {
		log.Printf("Error upserting member on follow: %v", err)
	}
	log.Printf("New follower: %s (%s)", displayName, userID)
}

func (s *Server) handleTextMessage(r *http.Request, event *linebot.Event, message *linebot.TextMessage) {
	text := strings.TrimSpace(message.Text)
	userID := event.Source.UserID

	if s.isLoginCode(text) {
		s.handleLoginCode(r, event, text, userID)
	} else if isServerUIDCommand(text) {
		s.replyText(event.ReplyToken, fmt.Sprintf("LINE UID ของคุณคือ:\n\n%s\n\nคัดลอกไปใช้ในระบบ Admin ได้เลย", userID))
	} else if isServerPointCommand(text) {
		s.replyText(event.ReplyToken, s.getPointSummary(r, userID))
	} else {
		s.handleChatMessage(r, event, text, userID)
	}
}

func (s *Server) isLoginCode(text string) bool {
	matched, _ := regexp.MatchString(`^\d{4}$`, text)
	return matched
}

func isServerUIDCommand(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	return text == "uid" || text == "ไอดี" || text == "id" || text == "line uid"
}

func isServerPointCommand(text string) bool {
	text = strings.TrimSpace(strings.ToLower(text))
	switch text {
	case "แต้ม", "แต้มสะสม", "แต้มคงเหลือ", "เช็คแต้ม", "ดูแต้ม", "คะแนน", "พ้อยท์", "พอยท์", "point", "points", "balance":
		return true
	default:
		return false
	}
}

func serverFallbackHelpMessage() string {
	return "พิมพ์ \"แต้มคงเหลือ\" เพื่อดูแต้มสะสม หรือพิมพ์ \"uid\" เพื่อดู LINE UID ของคุณ"
}

func (s *Server) handleLoginCode(r *http.Request, event *linebot.Event, code, userID string) {
	displayName, pictureURL := s.getUserProfile(userID)

	err := s.Store.VerifyCode(r.Context(), code, userID, displayName, pictureURL)
	if err != nil {
		s.replyText(event.ReplyToken, "รหัสไม่ถูกต้องหรือหมดอายุแล้ว")
		return
	}

	if err := s.Store.UpsertMember(r.Context(), userID, displayName, pictureURL); err != nil {
		log.Printf("Error upserting member on login code: %v", err)
	}
	s.replyText(event.ReplyToken, "ยืนยันตัวตนสำเร็จแล้ว\n\nกรุณากลับไปที่แอปเพื่อดำเนินการต่อ")
}

func (s *Server) handleChatMessage(r *http.Request, event *linebot.Event, text, userID string) {
	displayName, pictureURL := s.getUserProfile(userID)
	if err := s.Store.UpsertMember(r.Context(), userID, displayName, pictureURL); err != nil {
		log.Printf("Error upserting member: %v", err)
	}

	aiResponse := serverFallbackHelpMessage()
	if s.AIService != nil {
		history, err := s.Store.GetChatHistory(r.Context(), userID, 6)
		if err != nil {
			log.Printf("Error getting chat history: %v", err)
		}

		aiHistory := s.convertToAIHistory(history)
		if response, err := s.AIService.GenerateResponseWithHistory(text, aiHistory); err != nil {
			log.Printf("Error generating AI response: %v", err)
		} else {
			aiResponse = response
		}
	}

	s.saveChatMessages(r, userID, text, aiResponse)
	s.replyText(event.ReplyToken, aiResponse)
}

func (s *Server) getUserProfile(userID string) (displayName, pictureURL string) {
	profile, err := s.Bot.GetProfile(userID).Do()
	if err != nil {
		return "", ""
	}
	return profile.DisplayName, profile.PictureURL
}

func (s *Server) convertToAIHistory(history []store.ChatMessage) []ai.ChatMessage {
	aiHistory := make([]ai.ChatMessage, len(history))
	for i, msg := range history {
		aiHistory[i] = ai.ChatMessage{Role: msg.Role, Message: msg.Message}
	}
	return aiHistory
}

func (s *Server) saveChatMessages(r *http.Request, userID, userMessage, aiResponse string) {
	if err := s.Store.SaveChatMessage(r.Context(), userID, "user", userMessage); err != nil {
		log.Printf("Error saving user message: %v", err)
	}
	if err := s.Store.SaveChatMessage(r.Context(), userID, "assistant", aiResponse); err != nil {
		log.Printf("Error saving AI response: %v", err)
	}
}

func (s *Server) getPointSummary(r *http.Request, lineUID string) string {
	member, err := s.Store.GetMemberByLineUID(r.Context(), lineUID)
	if err != nil {
		log.Printf("Error getting point summary: %v", err)
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

func (s *Server) replyText(token, text string) {
	if _, err := s.Bot.ReplyMessage(token, linebot.NewTextMessage(text)).Do(); err != nil {
		log.Printf("Error replying to user: %v", err)
	}
}

// HandleGenerateQR handles POST /api/login/qr - generates QR session for LIFF login
func (s *Server) HandleGenerateQR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
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

	sessionID, err := s.Store.GenerateQRSession(r.Context(), req.ShopID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	liffURL := s.Config.LiffURL + "?session=" + sessionID
	response := map[string]interface{}{
		"session_id": sessionID,
		"liff_url":   liffURL,
		"expires_at": time.Now().Add(5 * time.Minute),
	}
	json.NewEncoder(w).Encode(response)
}

// HandleLiffVerify handles POST /api/login/liff-verify - verifies LIFF login
func (s *Server) HandleLiffVerify(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID   string `json:"session_id"`
		IDToken     string `json:"id_token"`
		UserID      string `json:"user_id"`
		DisplayName string `json:"display_name"`
		PictureURL  string `json:"picture_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid request body"})
		return
	}
	if req.SessionID == "" || req.UserID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "session_id and user_id are required"})
		return
	}

	err := s.Store.VerifyLiffSession(r.Context(), req.SessionID, req.UserID, req.DisplayName, req.PictureURL)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid or expired session"})
		return
	}

	if err := s.Store.UpsertMember(r.Context(), req.UserID, req.DisplayName, req.PictureURL); err != nil {
		log.Printf("Error upserting member on LIFF verify: %v", err)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "เชื่อมต่อสำเร็จ"})
}
