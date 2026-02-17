package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"bcmemberapi/store"
)

// SaleInvoiceRequest represents the request body
type SaleInvoiceRequest struct {
	LineUID     string  `json:"line_uid"`
	ShopID      string  `json:"shop_id"` // External ID from POS system
	DocNo       string  `json:"doc_no"`
	Amount      float64 `json:"amount"`       // ยอดซื้อ (ใหม่) - API จะคำนวณแต้มให้
	GetPoint    float64 `json:"get_point"`    // (เดิม) backward compatible - ถ้าส่งมาจะใช้ค่านี้
	UsePoint    float64 `json:"use_point"`
	DisplayName string  `json:"display_name"` // ชื่อสมาชิก (optional)
	PictureURL  string  `json:"picture_url"`  // รูปโปรไฟล์ (optional)
}

// SaleInvoiceResponse represents the response
type SaleInvoiceResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Data    *struct {
		DocNo        string  `json:"doc_no"`
		Amount       float64 `json:"amount,omitempty"`
		GetPoint     float64 `json:"get_point"`
		UsePoint     float64 `json:"use_point"`
		PointBalance float64 `json:"point_balance"`
		ShopName     string  `json:"shop_name"`
		Tier         string  `json:"tier"`
	} `json:"data,omitempty"`
}

// CalculatePointResponse represents calculate-point response
type CalculatePointResponse struct {
	Success       bool        `json:"success"`
	Amount        float64     `json:"amount"`
	GetPoint      int         `json:"get_point"`
	UsePoint      float64     `json:"use_point,omitempty"`
	UsePointValid bool        `json:"use_point_valid,omitempty"`
	RedeemValue   float64     `json:"redeem_value,omitempty"`
	MemberBalance float64     `json:"member_balance,omitempty"`
	MaxUsePoint   float64     `json:"max_use_point,omitempty"`
	Shop          *ShopConfig `json:"shop,omitempty"`
	Error         string      `json:"error,omitempty"`
}

// ShopConfig for calculate-point response
type ShopConfig struct {
	Name           string `json:"name"`
	PointRate      int    `json:"point_rate"`
	MinAmount      int    `json:"min_amount"`
	MaxPointsPerTx int    `json:"max_points_per_tx"`
	RedeemRate     int    `json:"redeem_rate"`
}

// Handler for /api/saleinvoice (POST) and /api/calculate-point (GET)
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

	// Route based on method
	if r.Method == http.MethodGet {
		handleCalculatePoint(w, r)
		return
	}

	if r.Method != http.MethodPost {
		sendError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	handleSaleInvoice(w, r)
}

// handleCalculatePoint handles GET /api/calculate-point
func handleCalculatePoint(w http.ResponseWriter, r *http.Request) {
	// Get API Key from header
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		sendCalcError(w, http.StatusUnauthorized, "API Key is required")
		return
	}

	// Verify API Key and get shop info
	shop, err := store.GetShopByAPIKey(apiKey)
	if err != nil {
		log.Printf("ERROR: GetShopByAPIKey failed: %v", err)
		sendCalcError(w, http.StatusInternalServerError, "Failed to verify API Key")
		return
	}
	if shop == nil {
		sendCalcError(w, http.StatusUnauthorized, "Invalid API Key")
		return
	}
	if shop.Status != "active" {
		sendCalcError(w, http.StatusForbidden, "Shop is not active")
		return
	}

	// Parse query parameters
	amountStr := r.URL.Query().Get("amount")
	if amountStr == "" {
		sendCalcError(w, http.StatusBadRequest, "amount is required")
		return
	}

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount < 0 {
		sendCalcError(w, http.StatusBadRequest, "Invalid amount")
		return
	}

	usePointStr := r.URL.Query().Get("use_point")
	var usePoint float64
	if usePointStr != "" {
		usePoint, _ = strconv.ParseFloat(usePointStr, 64)
	}

	lineUID := r.URL.Query().Get("line_uid")

	// Calculate points
	getPoint := store.CalculatePoints(shop, amount)

	// Set defaults
	if shop.PointRate <= 0 {
		shop.PointRate = 25
	}
	if shop.RedeemRate <= 0 {
		shop.RedeemRate = 1
	}

	// Build response
	response := CalculatePointResponse{
		Success:  true,
		Amount:   amount,
		GetPoint: getPoint,
		Shop: &ShopConfig{
			Name:           shop.Name,
			PointRate:      shop.PointRate,
			MinAmount:      shop.MinAmount,
			MaxPointsPerTx: shop.MaxPointsPerTx,
			RedeemRate:     shop.RedeemRate,
		},
	}

	// If line_uid provided, get member balance and validate use_point
	if lineUID != "" {
		mongoURI := os.Getenv("MONGODB_URI")
		if mongoURI != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			st, err := store.NewStore(mongoURI)
			if err == nil {
				member, err := st.GetMemberByLineUID(ctx, lineUID)
				if err == nil && member != nil {
					response.MemberBalance = member.PointBalance
					response.MaxUsePoint = member.PointBalance

					// Validate use_point
					if usePoint > 0 {
						response.UsePoint = usePoint
						response.UsePointValid = usePoint <= member.PointBalance
						response.RedeemValue = usePoint * float64(shop.RedeemRate)
					}
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleSaleInvoice handles POST /api/saleinvoice
func handleSaleInvoice(w http.ResponseWriter, r *http.Request) {

	// Get API Key from header
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		sendError(w, http.StatusUnauthorized, "API Key is required")
		return
	}

	// Verify API Key and get shop info
	shop, err := store.GetShopByAPIKey(apiKey)
	if err != nil {
		log.Printf("ERROR: GetShopByAPIKey failed: %v", err)
		sendError(w, http.StatusInternalServerError, "Failed to verify API Key")
		return
	}
	if shop == nil {
		sendError(w, http.StatusUnauthorized, "Invalid API Key")
		return
	}
	if shop.Status != "active" {
		sendError(w, http.StatusForbidden, "Shop is not active")
		return
	}

	// Parse request body
	var req SaleInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.LineUID == "" {
		sendError(w, http.StatusBadRequest, "line_uid is required")
		return
	}

	// Use shop name from DB
	shopName := shop.Name

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

	// Calculate get_point from amount if not provided (backward compatible)
	getPoint := req.GetPoint
	if getPoint == 0 && req.Amount > 0 {
		getPoint = float64(store.CalculatePoints(shop, req.Amount))
	}

	// Validate use_point - check member has enough balance
	if req.UsePoint > 0 {
		member, err := st.GetMemberByLineUID(ctx, req.LineUID)
		if err != nil {
			log.Printf("ERROR: GetMemberByLineUID failed: %v", err)
			sendError(w, http.StatusInternalServerError, "Failed to get member data")
			return
		}

		memberBalance := float64(0)
		if member != nil {
			memberBalance = member.PointBalance
		}

		if req.UsePoint > memberBalance {
			sendError(w, http.StatusBadRequest, "Insufficient points balance")
			return
		}
	}

	// 1. Save point transaction (records which shop for tracking)
	err = st.SavePointTransaction(ctx, req.LineUID, req.ShopID, shopName, req.DocNo, getPoint, req.UsePoint)
	if err != nil {
		if err == store.ErrDuplicateTransaction {
			log.Printf("WARN: Duplicate transaction doc_no=%s shop_id=%s", req.DocNo, req.ShopID)
			sendError(w, http.StatusConflict, "Duplicate transaction: doc_no already exists")
			return
		}
		log.Printf("ERROR: SavePointTransaction failed: %v", err)
		sendError(w, http.StatusInternalServerError, "Failed to save transaction")
		return
	}

	// 2. Upsert central member (create if not exists, update name/picture if provided)
	err = st.UpsertMember(ctx, req.LineUID, req.DisplayName, req.PictureURL)
	if err != nil {
		log.Printf("ERROR: UpsertMember failed: %v", err)
		// Continue anyway
	}

	// 3. Update central member points
	pointChange := getPoint - req.UsePoint
	newBalance, newTier, err := st.UpdateMemberPoints(ctx, req.LineUID, pointChange, getPoint)
	if err != nil {
		log.Printf("ERROR: UpdateMemberPoints failed: %v", err)
		// Try to recalculate
		st.RecalculatePoints(ctx, req.LineUID, "")
		newBalance = 0
		newTier = "Standard"
	}

	// Send success response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SaleInvoiceResponse{
		Success: true,
		Message: "Invoice processed successfully",
		Data: &struct {
			DocNo        string  `json:"doc_no"`
			Amount       float64 `json:"amount,omitempty"`
			GetPoint     float64 `json:"get_point"`
			UsePoint     float64 `json:"use_point"`
			PointBalance float64 `json:"point_balance"`
			ShopName     string  `json:"shop_name"`
			Tier         string  `json:"tier"`
		}{
			DocNo:        req.DocNo,
			Amount:       req.Amount,
			GetPoint:     getPoint,
			UsePoint:     req.UsePoint,
			PointBalance: newBalance,
			ShopName:     shopName,
			Tier:         newTier,
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

func sendCalcError(w http.ResponseWriter, statusCode int, errorMsg string) {
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(CalculatePointResponse{
		Success: false,
		Error:   errorMsg,
	})
}
