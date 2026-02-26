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
		"success":            true,
		"totalMembers":       stats.TotalMembers,
		"totalShops":         stats.TotalShops,
		"pointsEarnedToday":  stats.PointsEarnedToday,
		"pointsUsedToday":    stats.PointsUsedToday,
		"shopsStats":         stats.ShopsStats,
		"recentTransactions": stats.RecentTransactions,
		"tierStats":          stats.TierStats,
		"chartData":          chartData,
		"chartDays":          days,
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

	case http.MethodPost:
		var req struct {
			Action  string `json:"action"`
			LineUID string `json:"line_uid"`
			Type    string `json:"type"`
			Points  int    `json:"points"`
			Note    string `json:"note"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Invalid request body",
			})
			return
		}

		if req.Action == "adjust_points" {
			if req.LineUID == "" {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error":   "line_uid is required",
				})
				return
			}
			if req.Points <= 0 {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error":   "points must be greater than 0",
				})
				return
			}
			if req.Type != "add" && req.Type != "deduct" {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error":   "type must be 'add' or 'deduct'",
				})
				return
			}

			newBalance, err := store.AdjustMemberPoints(req.LineUID, req.Type, req.Points, req.Note)
			if err != nil {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"success": false,
					"error":   "Failed to adjust points: " + err.Error(),
				})
				return
			}

			// Audit log
			adminUID := extractAdminFromToken(r)
			go store.SaveAuditLog(adminUID, "", "adjust_points", "member", req.LineUID, fmt.Sprintf("%s %d points, note: %s", req.Type, req.Points, req.Note))

			json.NewEncoder(w).Encode(map[string]interface{}{
				"success":    true,
				"message":    "Points adjusted successfully",
				"newBalance": int(newBalance),
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Unknown action",
		})

	default:
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Method not allowed",
		})
	}
}

// ========== Transactions ==========

func handleTransactions(w http.ResponseWriter, r *http.Request) {
	filter := store.TransactionFilter{
		StartDate: r.URL.Query().Get("start_date"),
		EndDate:   r.URL.Query().Get("end_date"),
		ShopID:    r.URL.Query().Get("shop_id"),
		TxType:    r.URL.Query().Get("type"),
	}
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil {
		filter.Page = p
	}
	if ps, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil {
		filter.PageSize = ps
	}

	transactions, todayStats, total, err := store.GetAllTransactions(filter)
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
