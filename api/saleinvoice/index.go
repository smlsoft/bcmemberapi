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

// SaleInvoiceRequest represents the request body
type SaleInvoiceRequest struct {
	LineUID  string  `json:"line_uid"`
	ShopID   string  `json:"shop_id"`
	ShopName string  `json:"shop_name"`
	DocNo    string  `json:"doc_no"`
	GetPoint float64 `json:"get_point"`
	UsePoint float64 `json:"use_point"`
}

// SaleInvoiceResponse represents the response
type SaleInvoiceResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Data    *struct {
		DocNo        string  `json:"doc_no"`
		PointBalance float64 `json:"point_balance"`
	} `json:"data,omitempty"`
}

// Handler for POST /api/saleinvoice
func Handler(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
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

	// Verify API Key
	apiKey := r.Header.Get("X-API-Key")
	if apiKey != "bcaicloudx" {
		sendError(w, http.StatusUnauthorized, "Invalid API Key")
		return
	}

	// Parse request body
	var req SaleInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.LineUID == "" || req.ShopID == "" {
		sendError(w, http.StatusBadRequest, "line_uid and shop_id are required")
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Connect to MongoDB
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

	// 1. Save point transaction
	err = st.SavePointTransaction(ctx, req.LineUID, req.ShopID, req.ShopName, req.DocNo, req.GetPoint, req.UsePoint)
	if err != nil {
		log.Printf("ERROR: SavePointTransaction failed: %v", err)
		sendError(w, http.StatusInternalServerError, "Failed to save transaction")
		return
	}

	// 2. Upsert member (create if not exists)
	err = st.UpsertMember(ctx, req.LineUID, req.ShopID, req.ShopName, "", "")
	if err != nil {
		log.Printf("ERROR: UpsertMember failed: %v", err)
		// Continue anyway
	}

	// 3. Update member points
	pointChange := req.GetPoint - req.UsePoint
	newBalance, err := st.UpdateMemberPoints(ctx, req.LineUID, req.ShopID, pointChange)
	if err != nil {
		log.Printf("ERROR: UpdateMemberPoints failed: %v", err)
		// Try to recalculate
		st.RecalculatePoints(ctx, req.LineUID, req.ShopID)
		newBalance = 0
	}

	// Send success response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SaleInvoiceResponse{
		Success: true,
		Message: "Invoice processed successfully",
		Data: &struct {
			DocNo        string  `json:"doc_no"`
			PointBalance float64 `json:"point_balance"`
		}{
			DocNo:        req.DocNo,
			PointBalance: newBalance,
		},
	})
}

func sendError(w http.ResponseWriter, statusCode int, errorMsg string) {
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(SaleInvoiceResponse{
		Success: false,
		Error:   errorMsg,
	})
}
