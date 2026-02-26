package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"bcmemberapi/store"
)

// RecalPointsRequest represents the request body
type RecalPointsRequest struct {
	LineUID string `json:"line_uid"` // optional - if empty, recalculate all
	ShopID  string `json:"shop_id"`  // optional - if empty, recalculate all shops
}

// RecalPointsResponse represents the response
type RecalPointsResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Count   int    `json:"count,omitempty"`
}

// Handler for POST /api/recal-points
func Handler(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
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

	// Verify admin authentication via Bearer token
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		sendError(w, http.StatusUnauthorized, "Authorization required")
		return
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")
	// Token format: lineUID_timestamp (from admin login)
	parts := strings.SplitN(token, "_", 2)
	if len(parts) < 2 || parts[0] == "" {
		sendError(w, http.StatusUnauthorized, "Invalid token")
		return
	}

	adminLineUID := parts[0]

	// Initialize global store for admin check
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Println("ERROR: MONGODB_URI not configured")
		sendError(w, http.StatusInternalServerError, "Database not configured")
		return
	}

	st, err := store.NewStore(mongoURI)
	if err != nil {
		log.Printf("ERROR: Database connection failed: %v", err)
		sendError(w, http.StatusInternalServerError, "Database connection failed")
		return
	}
	store.SetGlobalStore(st)

	// Verify admin role
	role, err := store.GetAdminRole(adminLineUID)
	if err != nil || role == "" {
		// Also check env ADMIN_LINE_UIDS
		envAdmins := os.Getenv("ADMIN_LINE_UIDS")
		isAdmin := false
		if envAdmins != "" {
			for _, uid := range strings.Split(envAdmins, ",") {
				if strings.TrimSpace(uid) == adminLineUID {
					isAdmin = true
					break
				}
			}
		}
		if !isAdmin {
			sendError(w, http.StatusForbidden, "Admin access required")
			return
		}
	}

	// Parse request body
	var req RecalPointsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// If body is empty, use default values (recalculate all)
		req = RecalPointsRequest{}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Recalculate points
	count, err := st.RecalculatePoints(ctx, req.LineUID, req.ShopID)
	if err != nil {
		log.Printf("ERROR: RecalculatePoints failed: %v", err)
		sendError(w, http.StatusInternalServerError, "Failed to recalculate points")
		return
	}

	// Audit log
	go store.SaveAuditLog(adminLineUID, "", "recalculate_points", "system", "", fmt.Sprintf("count: %d", count))

	// Send success response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(RecalPointsResponse{
		Success: true,
		Message: "Points recalculated successfully",
		Count:   count,
	})
}

func sendError(w http.ResponseWriter, statusCode int, errorMsg string) {
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(RecalPointsResponse{
		Success: false,
		Error:   errorMsg,
	})
}
