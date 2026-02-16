package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"bcmemberapi/store"
)

// Handler for /api/admin?action=login|dashboard|shops|members|transactions
func Handler(w http.ResponseWriter, r *http.Request) {
	// CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	action := r.URL.Query().Get("action")

	switch action {
	case "login":
		handleAdminLogin(w, r)
	case "dashboard":
		handleDashboard(w, r)
	case "shops":
		handleShops(w, r)
	case "members":
		handleMembers(w, r)
	case "transactions":
		handleTransactions(w, r)
	default:
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid action",
		})
	}
}

// ========== Admin Login ==========

type Admin struct {
	LineUID     string `json:"line_uid"`
	DisplayName string `json:"display_name"`
	PictureURL  string `json:"picture_url"`
	Role        string `json:"role"`
}

func handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Method not allowed",
		})
		return
	}

	var req struct {
		LineUID     string `json:"line_uid"`
		DisplayName string `json:"display_name"`
		PictureURL  string `json:"picture_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid request body",
		})
		return
	}

	if req.LineUID == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "LINE UID is required",
		})
		return
	}

	// For development, allow any LINE user to be admin
	role := "admin"

	// Check environment for restricted admin list
	if envAdmins := os.Getenv("ADMIN_LINE_UIDS"); envAdmins != "" {
		// In production with ADMIN_LINE_UIDS set, restrict access
		// For now, we allow all users for development
		_ = envAdmins
	}

	// Save/update admin in database
	if err := store.SaveAdmin(req.LineUID, req.DisplayName, req.PictureURL, role); err != nil {
		// Log error but continue
		println("Error saving admin:", err.Error())
	}

	// Generate simple token
	token := req.LineUID + "_" + time.Now().Format("20060102150405")

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"token":   token,
		"admin": Admin{
			LineUID:     req.LineUID,
			DisplayName: req.DisplayName,
			PictureURL:  req.PictureURL,
			Role:        role,
		},
	})
}

// ========== Dashboard ==========

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := store.GetDashboardStats()
	if err != nil || stats == nil {
		// Return mock data on error
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":           true,
			"totalMembers":      0,
			"totalShops":        0,
			"pointsEarnedToday": 0,
			"pointsUsedToday":   0,
			"shopsStats":        []interface{}{},
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":            true,
		"totalMembers":       stats.TotalMembers,
		"totalShops":         stats.TotalShops,
		"pointsEarnedToday":  stats.PointsEarnedToday,
		"pointsUsedToday":    stats.PointsUsedToday,
		"shopsStats":         stats.ShopsStats,
		"recentTransactions": stats.RecentTransactions,
	})
}

// ========== Shops ==========

func handleShops(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Get all shops
		shops, err := store.GetAllShops()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Failed to get shops: " + err.Error(),
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"shops":   shops,
		})

	case http.MethodPost:
		// Create new shop
		var req struct {
			Name         string `json:"name"`
			OwnerLineUID string `json:"owner_line_uid"`
			PointRate    int    `json:"point_rate"`
			Status       string `json:"status"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Invalid request body",
			})
			return
		}

		if req.Name == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Shop name is required",
			})
			return
		}

		if req.PointRate <= 0 {
			req.PointRate = 25 // Default: 25 baht = 1 point
		}

		if req.Status == "" {
			req.Status = "active"
		}

		// Generate API key
		apiKey := fmt.Sprintf("bc_live_%d", time.Now().UnixNano())

		shopData := &store.ShopData{
			Name:         req.Name,
			OwnerLineUID: req.OwnerLineUID,
			PointRate:    req.PointRate,
			Status:       req.Status,
			APIKey:       apiKey,
		}

		shopID, err := store.CreateShop(shopData)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Failed to create shop: " + err.Error(),
			})
			return
		}

		shopData.ID = shopID
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"shop":    shopData,
		})

	case http.MethodPut:
		// Update shop settings
		var req struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			PointRate      int    `json:"point_rate"`
			MinAmount      int    `json:"min_amount"`
			MaxPointsPerTx int    `json:"max_points_per_tx"`
			RedeemRate     int    `json:"redeem_rate"`
			Status         string `json:"status"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Invalid request body",
			})
			return
		}

		if req.ID == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Shop ID is required",
			})
			return
		}

		// Build updates map
		updates := map[string]interface{}{}
		if req.Name != "" {
			updates["name"] = req.Name
		}
		if req.PointRate > 0 {
			updates["point_rate"] = req.PointRate
		}
		if req.MinAmount >= 0 {
			updates["min_amount"] = req.MinAmount
		}
		if req.MaxPointsPerTx >= 0 {
			updates["max_points_per_tx"] = req.MaxPointsPerTx
		}
		if req.RedeemRate >= 0 {
			updates["redeem_rate"] = req.RedeemRate
		}
		if req.Status != "" {
			updates["status"] = req.Status
		}

		err := store.UpdateShop(req.ID, updates)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Failed to update shop: " + err.Error(),
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Shop updated successfully",
		})

	default:
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Method not allowed",
		})
	}
}

// ========== Members ==========

func handleMembers(w http.ResponseWriter, r *http.Request) {
	members, err := store.GetAllMembers()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to get members",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"members": members,
		"total":   len(members),
	})
}

// ========== Transactions ==========

func handleTransactions(w http.ResponseWriter, r *http.Request) {
	transactions, todayStats, err := store.GetAllTransactions()
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to get transactions",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"transactions": transactions,
		"todayStats":   todayStats,
	})
}
