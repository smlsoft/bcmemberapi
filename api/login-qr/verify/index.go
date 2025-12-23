package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"bcmemberapi/store"
)

// Handler for POST /api/login-qr/verify
func Handler(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	// Handle OPTIONS request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow POST
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Method not allowed",
		})
		return
	}

	// Parse request body
	var req struct {
		SessionID   string `json:"session_id"`
		LineUserID  string `json:"line_user_id"`
		DisplayName string `json:"display_name"`
		PictureURL  string `json:"picture_url"`
		Type        string `json:"type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	// Validate required fields
	if req.SessionID == "" || req.LineUserID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Session ID and LINE User ID required",
		})
		return
	}

	// Get MongoDB URI from environment
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database not configured",
		})
		return
	}

	// Create MongoDB store
	st, err := store.NewStore(mongoURI)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database connection failed",
		})
		return
	}

	// Verify session with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	// Get session first
	session, err := st.GetLoginSession(ctx, req.SessionID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Session not found",
		})
		return
	}

	// Check if expired
	if session.ExpiresAt.Before(time.Now()) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "expired",
		})
		return
	}

	// Check if already verified
	if session.Status == "success" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Session already verified",
		})
		return
	}

	// Update session with user data
	userData := &store.LoginUser{
		LineUserID:  req.LineUserID,
		DisplayName: req.DisplayName,
		PictureURL:  req.PictureURL,
	}

	err = st.VerifyLoginSession(ctx, req.SessionID, userData)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to verify session",
		})
		return
	}

	// Return success
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Verification successful",
	})
}
