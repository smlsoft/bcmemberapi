package server

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"

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
		if event.Type == linebot.EventTypeMessage {
			switch message := event.Message.(type) {
			case *linebot.TextMessage:
				s.handleTextMessage(r, event, message)
			}
		}
	}
}

// handleTextMessage processes text messages from LINE
func (s *Server) handleTextMessage(r *http.Request, event *linebot.Event, message *linebot.TextMessage) {
	text := strings.TrimSpace(message.Text)
	userID := event.Source.UserID

	if s.isLoginCode(text) {
		s.handleLoginCode(r, event, text, userID)
	} else {
		s.handleChatMessage(r, event, text, userID)
	}
}

// isLoginCode checks if the text is a 4-digit login code
func (s *Server) isLoginCode(text string) bool {
	matched, _ := regexp.MatchString(`^\d{4}$`, text)
	return matched
}

// handleLoginCode processes 4-digit login codes
func (s *Server) handleLoginCode(r *http.Request, event *linebot.Event, code, userID string) {
	displayName, pictureURL := s.getUserProfile(userID)

	err := s.Store.VerifyCode(r.Context(), code, userID, displayName, pictureURL)
	if err != nil {
		s.replyText(event.ReplyToken, "Invalid or expired code.")
	} else {
		s.replyText(event.ReplyToken, "Login Successful! You can now return to the website.")
	}
}

// handleChatMessage processes general chat messages with AI
func (s *Server) handleChatMessage(r *http.Request, event *linebot.Event, text, userID string) {
	// Get chat history (last 6 messages = 3 conversation pairs to save AI tokens)
	history, err := s.Store.GetChatHistory(r.Context(), userID, 6)
	if err != nil {
		log.Printf("Error getting chat history: %v", err)
	}

	// Convert to AI message format
	aiHistory := s.convertToAIHistory(history)

	// Generate AI response
	aiResponse, err := s.AIService.GenerateResponseWithHistory(text, aiHistory)
	if err != nil {
		log.Printf("Error generating AI response: %v", err)
		aiResponse = "ขอโทษครับ เกิดข้อผิดพลาดในการประมวลผล กรุณาลองใหม่อีกครั้ง"
	}

	// Save messages to history
	s.saveChatMessages(r, userID, text, aiResponse)

	// Reply to user
	s.replyText(event.ReplyToken, aiResponse)
}

// getUserProfile fetches the user's display name and picture URL from LINE
func (s *Server) getUserProfile(userID string) (displayName, pictureURL string) {
	profile, err := s.Bot.GetProfile(userID).Do()
	if err != nil {
		return "", ""
	}
	return profile.DisplayName, profile.PictureURL
}

// convertToAIHistory converts store.ChatMessage to ai.ChatMessage
func (s *Server) convertToAIHistory(history []store.ChatMessage) []ai.ChatMessage {
	aiHistory := make([]ai.ChatMessage, len(history))
	for i, msg := range history {
		aiHistory[i] = ai.ChatMessage{
			Role:    msg.Role,
			Message: msg.Message,
		}
	}
	return aiHistory
}

// saveChatMessages saves both user message and AI response to the database
func (s *Server) saveChatMessages(r *http.Request, userID, userMessage, aiResponse string) {
	if err := s.Store.SaveChatMessage(r.Context(), userID, "user", userMessage); err != nil {
		log.Printf("Error saving user message: %v", err)
	}
	if err := s.Store.SaveChatMessage(r.Context(), userID, "assistant", aiResponse); err != nil {
		log.Printf("Error saving AI response: %v", err)
	}
}

// replyText sends a text reply to the user
func (s *Server) replyText(token, text string) {
	if _, err := s.Bot.ReplyMessage(token, linebot.NewTextMessage(text)).Do(); err != nil {
		log.Printf("Error replying to user: %v", err)
	}
}
