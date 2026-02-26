package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"bcmemberapi/store"
)

// Handler for GET /api/login-qr/status
func Handler(w http.ResponseWriter, r *http.Request) {
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
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Method not allowed",
		})
		return
	}

	// Get session ID from query params
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Session ID required",
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

	// Check session status with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	session, err := st.GetLoginSession(ctx, sessionID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"status":  "not_found",
			"error":   "Session not found",
		})
		return
	}

	// Check if expired
	if session.ExpiresAt.Before(time.Now()) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"status":  "expired",
		})
		return
	}

	// Return session status
	response := map[string]interface{}{
		"success": true,
		"status":  session.Status,
		"type":    session.Type,
	}

	// If verified, include user data
	if session.Status == "success" && session.User != nil {
		response["user"] = session.User
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
