package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/line/line-bot-sdk-go/v7/linebot"
)

// SendReceiptRequest represents the request body
type SendReceiptRequest struct {
	LineUID  string `json:"line_uid"`
	ImageURL string `json:"image_url"`
}

// SendReceiptResponse represents the response
type SendReceiptResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Handler for POST /api/send-receipt
func Handler(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
	w.Header().Set("Content-Type", "application/json")

	// Handle OPTIONS request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow POST
	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Verify API Key
	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "bcaicloudx" {
		sendError(w, http.StatusUnauthorized, "Invalid API Key")
		return
	}

	// Parse request body
	var req SendReceiptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.LineUID == "" || req.ImageURL == "" {
		sendError(w, http.StatusBadRequest, "line_uid and image_url are required")
		return
	}

	// Send LINE message
	err := sendLineReceipt(req.LineUID, req.ImageURL)
	if err != nil {
		log.Printf("ERROR: sendLineReceipt failed: %v", err)
		sendError(w, http.StatusInternalServerError, "Failed to send receipt")
		return
	}

	// Send success response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SendReceiptResponse{
		Success: true,
		Message: "Receipt sent successfully",
	})
}

func sendLineReceipt(lineUID, imageURL string) error {
	channelSecret := strings.TrimSpace(os.Getenv("LINE_CHANNEL_SECRET"))
	channelToken := strings.TrimSpace(os.Getenv("LINE_CHANNEL_TOKEN"))

	if channelSecret == "" || channelToken == "" {
		log.Println("ERROR: LINE credentials not configured")
		return nil
	}

	bot, err := linebot.New(channelSecret, channelToken)
	if err != nil {
		return err
	}

	imageMessage := linebot.NewImageMessage(imageURL, imageURL)

	_, err = bot.PushMessage(lineUID, imageMessage).Do()
	return err
}

func sendError(w http.ResponseWriter, statusCode int, errorMsg string) {
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(SendReceiptResponse{
		Success: false,
		Error:   errorMsg,
	})
}
