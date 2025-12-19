package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"bcmemberapi/store"
)

// StatusResponse represents the response for login status with point balance
type StatusResponse struct {
	Code         string    `json:"code"`
	ShopID       string    `json:"shop_id,omitempty"`
	Status       string    `json:"status"`
	LineUserID   string    `json:"line_user_id,omitempty"`
	DisplayName  string    `json:"display_name,omitempty"`
	PictureURL   string    `json:"picture_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	PointBalance float64   `json:"point_balance"`
}

// Handler for GET /api/login/status
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	// Handle OPTIONS request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow GET
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get code from query parameter
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Code parameter is required", http.StatusBadRequest)
		return
	}

	// Get MongoDB URI from environment
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

	// Get code status with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	result, err := st.GetCodeStatus(ctx, code)
	if err != nil {
		http.Error(w, "Code not found", http.StatusNotFound)
		return
	}

	// Build response with point balance
	response := StatusResponse{
		Code:         result.Code,
		ShopID:       result.ShopID,
		Status:       result.Status,
		LineUserID:   result.LineUserID,
		DisplayName:  result.DisplayName,
		PictureURL:   result.PictureURL,
		CreatedAt:    result.CreatedAt,
		ExpiresAt:    result.ExpiresAt,
		PointBalance: 0,
	}

	// Get point balance if login is successful and has shop_id
	if result.Status == "success" && result.LineUserID != "" && result.ShopID != "" {
		member, err := st.GetMemberByLineUIDAndShopID(ctx, result.LineUserID, result.ShopID)
		if err == nil && member != nil {
			response.PointBalance = member.PointBalance
		}
	}

	// Return response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
