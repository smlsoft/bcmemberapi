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

// Handler for /api/table-order/session
// POST   → create session (kiosk opens table)
// GET    → read session (web or kiosk polls)
// PATCH  → update session (web registers LINE user, or kiosk closes table)
func Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.Method {
	case http.MethodPost:
		handleCreateSession(w, r)
	case http.MethodGet:
		handleGetSession(w, r)
	case http.MethodPatch:
		action := r.URL.Query().Get("action")
		if action == "close" {
			handleCloseSession(w, r)
		} else {
			handleUpdateSessionUser(w, r)
		}
	default:
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Method not allowed",
		})
	}
}

func newStore() (*store.Store, error) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		return nil, nil
	}
	return store.NewStore(mongoURI)
}

// POST /api/table-order/session
// Body: { order_id, shop_id, branch_id, table_number, buffet_code, mode,
//         price_index, category_group_number, dedepos_token,
//         man_count, woman_count, child_count }
func handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrderID             string `json:"order_id"`
		ShopID              string `json:"shop_id"`
		BranchID            string `json:"branch_id"`
		TableNumber         string `json:"table_number"`
		BuffetCode          string `json:"buffet_code"`
		Mode                string `json:"mode"`
		PriceIndex          int    `json:"price_index"`
		CategoryGroupNumber int    `json:"category_group_number"`
		DedeposToken        string `json:"dedepos_token"`
		ManCount            int    `json:"man_count"`
		WomanCount          int    `json:"woman_count"`
		ChildCount          int    `json:"child_count"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	if req.OrderID == "" || req.ShopID == "" || req.TableNumber == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "order_id, shop_id, table_number are required",
		})
		return
	}

	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Println("ERROR: MONGODB_URI not configured")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, err := store.NewStore(mongoURI)
	if err != nil {
		log.Printf("ERROR: Database connection failed: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database connection failed",
		})
		return
	}

	sess := &store.TableOrderSession{
		OrderID:             req.OrderID,
		ShopID:              req.ShopID,
		BranchID:            req.BranchID,
		TableNumber:         req.TableNumber,
		BuffetCode:          req.BuffetCode,
		Mode:                req.Mode,
		PriceIndex:          req.PriceIndex,
		CategoryGroupNumber: req.CategoryGroupNumber,
		DedeposToken:        req.DedeposToken,
		ManCount:            req.ManCount,
		WomanCount:          req.WomanCount,
		ChildCount:          req.ChildCount,
	}

	sessionID, err := st.CreateTableOrderSession(ctx, sess)
	if err != nil {
		log.Printf("ERROR: CreateTableOrderSession failed: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to create session",
		})
		return
	}

	tableLiffID := os.Getenv("TABLE_LIFF_ID")
	liffURL := "https://liff.line.me/" + tableLiffID + "?session=" + sessionID

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"session_id": sessionID,
		"liff_url":   liffURL,
		"expires_at": sess.ExpiresAt.Format(time.RFC3339),
	})
}

// GET /api/table-order/session?session_id=X
func handleGetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "session_id is required",
		})
		return
	}

	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Println("ERROR: MONGODB_URI not configured")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, err := store.NewStore(mongoURI)
	if err != nil {
		log.Printf("ERROR: Database connection failed: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database connection failed",
		})
		return
	}

	sess, err := st.GetTableOrderSession(ctx, sessionID)
	if err != nil {
		log.Printf("ERROR: GetTableOrderSession failed: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Session not found",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":               true,
		"session_id":            sess.SessionID,
		"order_id":              sess.OrderID,
		"shop_id":               sess.ShopID,
		"branch_id":             sess.BranchID,
		"table_number":          sess.TableNumber,
		"buffet_code":           sess.BuffetCode,
		"mode":                  sess.Mode,
		"price_index":           sess.PriceIndex,
		"category_group_number": sess.CategoryGroupNumber,
		"dedepos_token":         sess.DedeposToken,
		"man_count":             sess.ManCount,
		"woman_count":           sess.WomanCount,
		"child_count":           sess.ChildCount,
		"status":                sess.Status,
		"line_user_id":          sess.LineUserID,
		"display_name":          sess.DisplayName,
		"picture_url":           sess.PictureURL,
		"expires_at":            sess.ExpiresAt.Format(time.RFC3339),
	})
}

// PATCH /api/table-order/session?session_id=X
// Body: { line_user_id, display_name, picture_url, status:"ordering" }
func handleUpdateSessionUser(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "session_id is required",
		})
		return
	}

	var req struct {
		LineUserID  string `json:"line_user_id"`
		DisplayName string `json:"display_name"`
		PictureURL  string `json:"picture_url"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Println("ERROR: MONGODB_URI not configured")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, err := store.NewStore(mongoURI)
	if err != nil {
		log.Printf("ERROR: Database connection failed: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database connection failed",
		})
		return
	}

	status := req.Status
	if status == "" {
		status = "ordering"
	}

	err = st.UpdateTableOrderSessionUser(ctx, sessionID, req.LineUserID, req.DisplayName, req.PictureURL, status)
	if err != nil {
		log.Printf("ERROR: UpdateTableOrderSessionUser failed: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to update session",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// PATCH /api/table-order/session?session_id=X&action=close
func handleCloseSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "session_id is required",
		})
		return
	}

	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		log.Println("ERROR: MONGODB_URI not configured")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, err := store.NewStore(mongoURI)
	if err != nil {
		log.Printf("ERROR: Database connection failed: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Database connection failed",
		})
		return
	}

	err = st.CloseTableOrderSession(ctx, sessionID)
	if err != nil {
		log.Printf("ERROR: CloseTableOrderSession failed: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to close session",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}
