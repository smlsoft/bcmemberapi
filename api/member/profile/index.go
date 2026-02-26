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
	Tier        string  `json:"tier"`
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

	// Get central member record for this LINE user
	member, err := st.GetMemberByLineUID(ctx, req.UserID)
	if err != nil {
		log.Printf("ERROR: GetMemberByLineUID failed: %v", err)
		json.NewEncoder(w).Encode(MemberProfileResponse{
			Success: false,
			Error:   "Failed to get member data",
		})
		return
	}

	// Get point balance and profile
	var totalPoints float64
	var displayName, pictureURL, tier string
	if member != nil {
		totalPoints = member.PointBalance
		displayName = member.DisplayName
		pictureURL = member.PictureURL
		tier = member.Tier
	}
	if tier == "" {
		tier = "Standard"
	}

	// Generate member code from userId
	memberCode := "BC-" + strings.ToUpper(req.UserID[:8])

	json.NewEncoder(w).Encode(MemberProfileResponse{
		Success:     true,
		Points:      totalPoints,
		MemberCode:  memberCode,
		DisplayName: displayName,
		PictureURL:  pictureURL,
		Tier:        tier,
		ShopCount:   1, // Central system
	})
}
