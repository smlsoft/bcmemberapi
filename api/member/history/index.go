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

type HistoryRequest struct {
	UserID  string `json:"user_id"`
	IDToken string `json:"id_token"`
}

type HistoryItem struct {
	ID          int       `json:"id"`
	Type        string    `json:"type"` // "earn" or "use"
	Points      float64   `json:"points"`
	Description string    `json:"description"`
	ShopName    string    `json:"shop_name,omitempty"`
	DocNo       string    `json:"doc_no,omitempty"`
	Date        time.Time `json:"date"`
}

type HistoryResponse struct {
	Success bool          `json:"success"`
	History []HistoryItem `json:"history"`
	Error   string        `json:"error,omitempty"`
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
		json.NewEncoder(w).Encode(HistoryResponse{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}

	var req HistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(HistoryResponse{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	if req.UserID == "" {
		json.NewEncoder(w).Encode(HistoryResponse{
			Success: false,
			Error:   "user_id is required",
		})
		return
	}

	// Connect to MongoDB
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Println("ERROR: MONGODB_URI not configured")
		json.NewEncoder(w).Encode(HistoryResponse{
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
		json.NewEncoder(w).Encode(HistoryResponse{
			Success: false,
			Error:   "Database connection failed",
		})
		return
	}

	// Get point transactions for this user
	transactions, err := st.GetPointTransactionsByLineUID(ctx, req.UserID)
	if err != nil {
		log.Printf("ERROR: GetPointTransactionsByLineUID failed: %v", err)
		json.NewEncoder(w).Encode(HistoryResponse{
			Success: false,
			Error:   "Failed to get history",
		})
		return
	}

	// Convert to history items
	history := make([]HistoryItem, 0)
	for i, t := range transactions {
		shopName := t.ShopName
		if shopName == "" {
			shopName = t.ShopID
		}

		// If has get_point, add earn entry
		if t.GetPoint > 0 {
			history = append(history, HistoryItem{
				ID:          i*2 + 1,
				Type:        "earn",
				Points:      t.GetPoint,
				Description: "สะสมแต้ม - " + shopName,
				ShopName:    shopName,
				DocNo:       t.DocNo,
				Date:        t.CreatedAt,
			})
		}

		// If has use_point, add use entry
		if t.UsePoint > 0 {
			history = append(history, HistoryItem{
				ID:          i*2 + 2,
				Type:        "use",
				Points:      t.UsePoint,
				Description: "ใช้แต้ม - " + shopName,
				ShopName:    shopName,
				DocNo:       t.DocNo,
				Date:        t.CreatedAt,
			})
		}
	}

	json.NewEncoder(w).Encode(HistoryResponse{
		Success: true,
		History: history,
	})
}
