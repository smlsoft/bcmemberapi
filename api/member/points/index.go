package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"bcmemberapi/store"
)

type PointsRequest struct {
	UserID  string `json:"user_id"`
	IDToken string `json:"id_token"`
}

type PointsResponse struct {
	Success     bool    `json:"success"`
	Points      float64 `json:"points"`
	TotalEarned float64 `json:"total_earned"`
	TotalUsed   float64 `json:"total_used"`
	Tier        string  `json:"tier"`
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
		json.NewEncoder(w).Encode(PointsResponse{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}

	var req PointsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(PointsResponse{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	if req.UserID == "" {
		json.NewEncoder(w).Encode(PointsResponse{
			Success: false,
			Error:   "user_id is required",
		})
		return
	}

	// Connect to MongoDB
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Println("ERROR: MONGODB_URI not configured")
		json.NewEncoder(w).Encode(PointsResponse{
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
		json.NewEncoder(w).Encode(PointsResponse{
			Success: false,
			Error:   "Database connection failed",
		})
		return
	}

	// Get central member record for this LINE user
	member, err := st.GetMemberByLineUID(ctx, req.UserID)
	if err != nil {
		log.Printf("ERROR: GetMemberByLineUID failed: %v", err)
		json.NewEncoder(w).Encode(PointsResponse{
			Success: false,
			Error:   "Failed to get member data",
		})
		return
	}

	// Get point transactions to calculate total earned/used
	transactions, err := st.GetPointTransactionsByLineUID(ctx, req.UserID)
	if err != nil {
		log.Printf("ERROR: GetPointTransactionsByLineUID failed: %v", err)
	}

	// Calculate totals from transactions
	var totalEarned, totalUsed float64
	for _, t := range transactions {
		totalEarned += t.GetPoint
		totalUsed += t.UsePoint
	}

	// Get central point balance and tier
	var totalPoints float64
	var tier string
	if member != nil {
		totalPoints = member.PointBalance
		tier = member.Tier
		// Use stored total_earned if available (more accurate than transaction sum)
		if member.TotalEarned > 0 {
			totalEarned = member.TotalEarned
		}
	}
	if tier == "" {
		tier = store.CalculateTier(totalEarned)
	}

	json.NewEncoder(w).Encode(PointsResponse{
		Success:     true,
		Points:      totalPoints,
		TotalEarned: totalEarned,
		TotalUsed:   totalUsed,
		Tier:        tier,
	})
}
