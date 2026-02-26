package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"bcmemberapi/store"
)

// Handler for /api/login-qr?action=generate|status|verify|kiosk
// Also handles /api/member/qr via route rewrite to action=kiosk
func Handler(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
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
	case "kiosk":
		// Kiosk member QR: POST=generate, GET=status with point_balance
		if r.Method == http.MethodPost {
			handleKioskGenerate(w, r)
		} else {
			handleKioskStatus(w, r)
		}
	default:
		// Legacy: default to generate for POST
		if r.Method == http.MethodPost {
			handleGenerate(w, r)
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Invalid action. Use ?action=generate|status|verify|kiosk",
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
		fmt.Printf("ERROR login-qr/generate: NewStore failed: %v\n", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database connection failed: " + err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	sessionID, err := st.GenerateLoginSession(ctx, req.Type, req.ShopID)
	if err != nil {
		fmt.Printf("ERROR login-qr/generate: GenerateLoginSession failed: %v\n", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to generate session: " + err.Error(),
		})
		return
	}

	// Build LIFF URL
	liffID := os.Getenv("LIFF_ID")
	if liffID == "" {
		liffID = "2008745223-8Ol0oVZk"
	}
	liffURL := "https://liff.line.me/" + liffID + "/verify?session=" + sessionID + "&type=" + req.Type

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"session_id": sessionID,
		"liff_url":   liffURL,
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

	// Upsert member with LINE profile (display_name + picture_url)
	if req.DisplayName != "" || req.PictureURL != "" {
		if err := st.UpsertMember(ctx, req.LineUserID, req.DisplayName, req.PictureURL); err != nil {
			// Log but don't fail the verify
			fmt.Printf("Error upserting member on verify: %v\n", err)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Verification successful",
	})
}

// handleKioskGenerate creates a QR session for kiosk member login (with point_balance)
func handleKioskGenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ShopID string `json:"shop_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	if req.ShopID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "shop_id is required",
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

	// Generate session with type "kiosk-member"
	sessionID, err := st.GenerateLoginSession(ctx, "kiosk-member", req.ShopID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to generate session",
		})
		return
	}

	// Build LIFF URL for member verification
	liffID := os.Getenv("LIFF_ID")
	if liffID == "" {
		liffID = "2008745223-8Ol0oVZk"
	}
	liffURL := "https://liff.line.me/" + liffID + "/verify?session=" + sessionID + "&type=kiosk-member"

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"session_id": sessionID,
		"liff_url":   liffURL,
		"shop_id":    req.ShopID,
		"expires_at": time.Now().Add(5 * time.Minute).Unix(),
	})
}

// handleKioskStatus checks session status and returns member info with point_balance
func handleKioskStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "session parameter is required",
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

	// Check if expired
	if session.ExpiresAt.Before(time.Now()) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"status":  "expired",
		})
		return
	}

	// Build base response
	response := map[string]interface{}{
		"success": true,
		"status":  session.Status,
	}

	// If login successful, get member info with point_balance
	if session.Status == "success" && session.User != nil {
		response["line_user_id"] = session.User.LineUserID
		response["display_name"] = session.User.DisplayName
		response["picture_url"] = session.User.PictureURL

		// Get point balance from members collection
		member, err := st.GetMemberByLineUID(ctx, session.User.LineUserID)
		if err == nil && member != nil {
			response["point_balance"] = member.PointBalance
		} else {
			// Member not found, return 0 points (will be created on first transaction)
			response["point_balance"] = 0.0
		}
	}

	json.NewEncoder(w).Encode(response)
}
