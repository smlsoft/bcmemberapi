package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"bcmemberapi/store"
)

func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// extractAdminFromToken extracts admin LINE UID from Bearer token
func extractAdminFromToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	parts := strings.SplitN(token, "_", 2)
	if len(parts) >= 1 && parts[0] != "" {
		return parts[0]
	}
	return ""
}

// Handler for /api/admin?action=login|dashboard|shops|members|transactions|audit_logs
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
	case "audit_logs":
		handleAuditLogs(w, r)
	case "settings":
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
		})
	case "admins":
		handleAdmins(w, r)
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

	// Check if user is allowed admin access
	role := ""
	envAdmins := os.Getenv("ADMIN_LINE_UIDS")
	if envAdmins != "" {
		// Check if user's LINE UID is in the allowed admin list
		allowed := false
		for _, uid := range splitAndTrim(envAdmins, ",") {
			if uid == req.LineUID {
				allowed = true
				break
			}
		}
		if !allowed {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "คุณไม่มีสิทธิ์เข้าใช้งาน Admin Panel",
			})
			return
		}
		role = "admin"
	} else {
		// No ADMIN_LINE_UIDS set — check from admins collection in database
		existingRole, err := store.GetAdminRole(req.LineUID)
		if err != nil || existingRole == "" {
			// Auto-bootstrap: if no admins exist yet, first login becomes super_admin
			count, countErr := store.CountAdmins()
			if countErr == nil && count == 0 {
				role = "super_admin"
			} else {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error":   "คุณไม่มีสิทธิ์เข้าใช้งาน Admin Panel",
				})
				return
			}
		} else {
			role = existingRole
		}
	}

	// Save/update admin in database
	if err := store.SaveAdmin(req.LineUID, req.DisplayName, req.PictureURL, role); err != nil {
		println("Error saving admin:", err.Error())
	}

	// Generate simple token
	token := req.LineUID + "_" + time.Now().Format("20060102150405")

	// Audit log
	go store.SaveAuditLog(req.LineUID, req.DisplayName, "admin_login", "admin", req.LineUID, "")

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

	// Chart data with optional days param (7 or 30)
	days := 7
	if d, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && (d == 7 || d == 30) {
		days = d
	}
	chartData, _ := store.GetDashboardChartData(days)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":                true,
		"totalMembers":           stats.TotalMembers,
		"totalShops":             stats.TotalShops,
		"pointsEarnedToday":      stats.PointsEarnedToday,
		"pointsUsedToday":        stats.PointsUsedToday,
		"shopsStats":             stats.ShopsStats,
		"recentTransactions":     stats.RecentTransactions,
		"tierStats":              stats.TierStats,
		"chartData":              chartData,
		"chartDays":              days,
		"outstandingPoints":      stats.OutstandingPoints,
		"topMembers":             stats.TopMembers,
		"abnormalAdjustments":    stats.AbnormalAdjustments,
		"duplicateInvoicesToday": stats.DuplicateInvoicesToday,
		"rejectedInvoicesToday":  stats.RejectedInvoicesToday,
		"recentInvoiceIssues":    stats.RecentInvoiceIssues,
	})
}

// ========== Shops ==========

func handleShops(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if shopID := strings.TrimSpace(r.URL.Query().Get("id")); shopID != "" {
			shop, err := store.GetShopByID(shopID)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to get shop: " + err.Error()})
				return
			}
			if shop == nil {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Shop not found"})
				return
			}
			amount := 0.0
			redeemPoint := 0.0
			if v, err := strconv.ParseFloat(r.URL.Query().Get("amount"), 64); err == nil {
				amount = v
			}
			if v, err := strconv.ParseFloat(r.URL.Query().Get("redeem_point"), 64); err == nil {
				redeemPoint = v
			}
			preview, _ := store.PreviewShopPoints(shopID, amount, redeemPoint)
			detail, err := store.GetShopDetail(shopID, 20)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to get shop detail: " + err.Error()})
				return
			}
			response := map[string]interface{}{"success": true, "shop": shop, "preview": preview, "active": store.IsShopActive(shop)}
			if detail != nil {
				response["detail"] = detail
				response["stats"] = detail.Stats
				response["members"] = detail.Members
				response["top_members"] = detail.TopMembers
				response["recent_transactions"] = detail.RecentTransactions
				response["invoice_issues"] = detail.InvoiceIssues
			}
			json.NewEncoder(w).Encode(response)
			return
		}

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
			Name           string `json:"name"`
			OwnerLineUID   string `json:"owner_line_uid"`
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
			Name:           req.Name,
			OwnerLineUID:   req.OwnerLineUID,
			PointRate:      req.PointRate,
			MinAmount:      req.MinAmount,
			MaxPointsPerTx: req.MaxPointsPerTx,
			RedeemRate:     req.RedeemRate,
			Status:         req.Status,
			APIKey:         apiKey,
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

		// Audit log
		adminUID := extractAdminFromToken(r)
		go store.SaveAuditLog(adminUID, "", "create_shop", "shop", shopID, req.Name)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"shop":    shopData,
		})

	case http.MethodPut:
		// Update shop settings
		var req struct {
			Action         string `json:"action"`
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

		switch req.Action {
		case "rotate_api_key":
			newKey, err := store.RotateShopAPIKey(req.ID)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to rotate API key: " + err.Error()})
				return
			}
			adminUID := extractAdminFromToken(r)
			go store.SaveAuditLog(adminUID, "", "rotate_api_key", "shop", req.ID, "rotated API key")
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "api_key": newKey})
			return
		case "revoke_api_key":
			if err := store.RevokeShopAPIKey(req.ID); err != nil {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to revoke API key: " + err.Error()})
				return
			}
			adminUID := extractAdminFromToken(r)
			go store.SaveAuditLog(adminUID, "", "revoke_api_key", "shop", req.ID, "revoked API key and set inactive")
			json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
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

		// Audit log
		adminUID := extractAdminFromToken(r)
		go store.SaveAuditLog(adminUID, "", "update_shop", "shop", req.ID, fmt.Sprintf("fields: %v", updates))

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
	switch r.Method {
	case http.MethodGet:
		if lineUID := strings.TrimSpace(r.URL.Query().Get("line_uid")); lineUID != "" {
			txPage := 1
			txPageSize := 20
			if p, err := strconv.Atoi(r.URL.Query().Get("tx_page")); err == nil {
				txPage = p
			}
			if ps, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil {
				txPageSize = ps
			}
			detail, txTotal, err := store.GetMemberDetail(lineUID, txPage, txPageSize)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to get member detail: " + err.Error()})
				return
			}
			if detail == nil {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Member not found"})
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":            true,
				"member":             detail.Member,
				"shops":              detail.Shops,
				"transactions":       detail.Transactions,
				"transactions_total": txTotal,
			})
			return
		}

		filter := store.MemberFilter{
			Query:  r.URL.Query().Get("q"),
			ShopID: r.URL.Query().Get("shop_id"),
			Tier:   r.URL.Query().Get("tier"),
			SortBy: r.URL.Query().Get("sort"),
		}
		if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil {
			filter.Page = p
		}
		if ps, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil {
			filter.PageSize = ps
		}
		result, err := store.GetMembers(filter)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to get members: " + err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      true,
			"members":      result.Members,
			"total":        result.Total,
			"page":         result.Page,
			"page_size":    result.PageSize,
			"totalPoints":  result.TotalPoints,
			"total_points": result.TotalPoints,
		})

	case http.MethodPost:
		var req struct {
			Action  string `json:"action"`
			LineUID string `json:"line_uid"`
			Type    string `json:"type"`
			Points  int    `json:"points"`
			Note    string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid request body"})
			return
		}
		if req.Action != "adjust_points" {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Unknown action"})
			return
		}
		result, err := store.AdjustMemberPoints(req.LineUID, req.Type, req.Points, req.Note)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to adjust points: " + err.Error()})
			return
		}

		adminUID := extractAdminFromToken(r)
		details := fmt.Sprintf("%s %d points, before: %.0f, after: %.0f, tx: %s, reason: %s", req.Type, req.Points, result.PreviousBalance, result.NewBalance, result.TransactionID, req.Note)
		go store.SaveAuditLog(adminUID, "", "adjust_points", "member", req.LineUID, details)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":         true,
			"message":         "Points adjusted successfully",
			"newBalance":      int(result.NewBalance),
			"previousBalance": int(result.PreviousBalance),
			"transactionId":   result.TransactionID,
			"result":          result,
		})
	default:
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
	}
}

// ========== Transactions ==========

func handleTransactions(w http.ResponseWriter, r *http.Request) {
	filter := store.TransactionFilter{
		StartDate: r.URL.Query().Get("start_date"),
		EndDate:   r.URL.Query().Get("end_date"),
		ShopID:    r.URL.Query().Get("shop_id"),
		LineUID:   r.URL.Query().Get("line_uid"),
		Query:     r.URL.Query().Get("q"),
		TxType:    r.URL.Query().Get("type"),
	}
	if filter.ShopID == "all" {
		filter.ShopID = ""
	}
	if filter.TxType == "all" {
		filter.TxType = ""
	}
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil {
		filter.Page = p
	}
	if ps, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil {
		filter.PageSize = ps
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 50
	}

	transactions, todayStats, total, err := store.GetAllTransactions(filter)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to get transactions: " + err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"transactions": transactions,
		"todayStats":   todayStats,
		"total":        total,
		"page":         filter.Page,
		"page_size":    filter.PageSize,
	})
}

// ========== Audit Logs ==========

func handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	filter := store.AuditLogFilter{
		Action:    r.URL.Query().Get("action_type"),
		StartDate: r.URL.Query().Get("start_date"),
		EndDate:   r.URL.Query().Get("end_date"),
	}
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil {
		filter.Page = p
	}
	if ps, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil {
		filter.PageSize = ps
	}

	logs, total, err := store.GetAuditLogs(filter)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to get audit logs",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"logs":      logs,
		"total":     total,
		"page":      filter.Page,
		"page_size": filter.PageSize,
	})
}

// ========== Admin Management ==========

func handleAdmins(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		admins, err := store.GetAllAdmins()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Failed to get admins: " + err.Error(),
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"admins":  admins,
		})

	case http.MethodPost:
		// Add new admin
		var req struct {
			LineUID string `json:"line_uid"`
			Role    string `json:"role"`
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

		if req.Role == "" {
			req.Role = "admin"
		}

		if err := store.SaveAdmin(req.LineUID, "", "", req.Role); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Failed to add admin: " + err.Error(),
			})
			return
		}

		adminUID := extractAdminFromToken(r)
		go store.SaveAuditLog(adminUID, "", "add_admin", "admin", req.LineUID, "role: "+req.Role)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Admin added successfully",
		})

	case http.MethodDelete:
		var req struct {
			LineUID string `json:"line_uid"`
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

		// Prevent deleting yourself
		callerUID := extractAdminFromToken(r)
		if callerUID == req.LineUID {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "ไม่สามารถลบตัวเองได้",
			})
			return
		}

		// Prevent deleting last super_admin
		targetRole, _ := store.GetAdminRole(req.LineUID)
		if targetRole == "super_admin" {
			admins, err := store.GetAllAdmins()
			if err == nil {
				superCount := 0
				for _, a := range admins {
					if a.Role == "super_admin" {
						superCount++
					}
				}
				if superCount <= 1 {
					json.NewEncoder(w).Encode(map[string]interface{}{
						"success": false,
						"error":   "ไม่สามารถลบ Super Admin คนสุดท้ายได้",
					})
					return
				}
			}
		}

		if err := store.DeleteAdmin(req.LineUID); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Failed to delete admin: " + err.Error(),
			})
			return
		}

		go store.SaveAuditLog(callerUID, "", "delete_admin", "admin", req.LineUID, "")

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "Admin deleted successfully",
		})

	default:
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Method not allowed",
		})
	}
}
