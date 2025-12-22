package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"bcmemberapi/store"
)

// Handler for POST /api/login/qr
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req struct {
		ShopID string `json:"shop_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get MongoDB URI from environment
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		http.Error(w, "MongoDB URI not configured", http.StatusInternalServerError)
		return
	}

	// Get LIFF ID from environment
	liffID := os.Getenv("LIFF_ID")
	if liffID == "" {
		liffID = "2008745223-8Ol0oVZk"
	}
	liffURL := "https://liff.line.me/" + liffID

	// Create MongoDB store
	st, err := store.NewStore(mongoURI)
	if err != nil {
		http.Error(w, "Database connection failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Generate QR session with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	sessionID, err := st.GenerateQRSession(ctx, req.ShopID)
	if err != nil {
		http.Error(w, "Failed to generate session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"session_id": sessionID,
		"liff_url":   liffURL + "?session=" + sessionID,
		"expires_at": time.Now().Add(5 * time.Minute),
	})
}
