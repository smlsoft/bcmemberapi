package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"bcmemberapi/store"
)

// Handler for /api/login-qr?action=generate|status|verify
func Handler(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	// Handle OPTIONS request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Get action from query params
	action := r.URL.Query().Get("action")

	switch action {
	case "generate":
		handleGenerate(w, r)
	case "status":
		handleStatus(w, r)
	case "verify":
		handleVerify(w, r)
	default:
		// Legacy: default to generate for POST
		if r.Method == http.MethodPost {
			handleGenerate(w, r)
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Invalid action. Use ?action=generate|status|verify",
			})
		}
	}
}

func handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Method not allowed",
		})
		return
	}

	var req struct {
		Type   string `json:"type"`
		ShopID string `json:"shop_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	if req.Type == "" {
		req.Type = "member"
	}

	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database not configured",
		})
		return
	}

	st, err := store.NewStore(mongoURI)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database connection failed",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	sessionID, err := st.GenerateLoginSession(ctx, req.Type, req.ShopID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to generate session",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"session_id": sessionID,
		"type":       req.Type,
		"expires_at": time.Now().Add(5 * time.Minute).Unix(),
	})
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Session ID required",
		})
		return
	}

	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database not configured",
		})
		return
	}

	st, err := store.NewStore(mongoURI)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database connection failed",
		})
		return
	}

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

	if session.ExpiresAt.Before(time.Now()) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"status":  "expired",
		})
		return
	}

	response := map[string]interface{}{
		"success": true,
		"status":  session.Status,
		"type":    session.Type,
	}

	if session.Status == "success" && session.User != nil {
		response["user"] = session.User
	}

	json.NewEncoder(w).Encode(response)
}

func handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Method not allowed",
		})
		return
	}

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

	if req.SessionID == "" || req.LineUserID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Session ID and LINE User ID required",
		})
		return
	}

	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database not configured",
		})
		return
	}

	st, err := store.NewStore(mongoURI)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database connection failed",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	session, err := st.GetLoginSession(ctx, req.SessionID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Session not found",
		})
		return
	}

	if session.ExpiresAt.Before(time.Now()) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "expired",
		})
		return
	}

	if session.Status == "success" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Session already verified",
		})
		return
	}

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

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Verification successful",
	})
}
