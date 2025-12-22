package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"bcmemberapi/store"
)

type MemberProfileRequest struct {
	UserID  string `json:"user_id"`
	IDToken string `json:"id_token"`
}

type MemberProfileResponse struct {
	Success     bool    `json:"success"`
	Points      float64 `json:"points"`
	MemberCode  string  `json:"member_code"`
	DisplayName string  `json:"display_name,omitempty"`
	PictureURL  string  `json:"picture_url,omitempty"`
	ShopCount   int     `json:"shop_count"`
	Error       string  `json:"error,omitempty"`
}

func Handler(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(MemberProfileResponse{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}

	var req MemberProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(MemberProfileResponse{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	if req.UserID == "" {
		json.NewEncoder(w).Encode(MemberProfileResponse{
			Success: false,
			Error:   "user_id is required",
		})
		return
	}

	// Connect to MongoDB
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Println("ERROR: MONGODB_URI not configured")
		json.NewEncoder(w).Encode(MemberProfileResponse{
			Success: false,
			Error:   "Database not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, err := store.NewStore(mongoURI)
	if err != nil {
		log.Printf("ERROR: Database connection failed: %v", err)
		json.NewEncoder(w).Encode(MemberProfileResponse{
			Success: false,
			Error:   "Database connection failed",
		})
		return
	}

	// Get all members for this LINE user (across all shops)
	members, err := st.GetMembersByLineUID(ctx, req.UserID)
	if err != nil {
		log.Printf("ERROR: GetMembersByLineUID failed: %v", err)
		json.NewEncoder(w).Encode(MemberProfileResponse{
			Success: false,
			Error:   "Failed to get member data",
		})
		return
	}

	// Calculate total points from all shops
	var totalPoints float64
	var displayName, pictureURL string
	for _, m := range members {
		totalPoints += m.PointBalance
		if displayName == "" && m.DisplayName != "" {
			displayName = m.DisplayName
		}
		if pictureURL == "" && m.PictureURL != "" {
			pictureURL = m.PictureURL
		}
	}

	// Generate member code from userId
	memberCode := "BC-" + strings.ToUpper(req.UserID[:8])

	json.NewEncoder(w).Encode(MemberProfileResponse{
		Success:     true,
		Points:      totalPoints,
		MemberCode:  memberCode,
		DisplayName: displayName,
		PictureURL:  pictureURL,
		ShopCount:   len(members),
	})
}
