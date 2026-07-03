package server

import (
	"net/http"
	"net/url"
	"strings"

	adminHandler "bcmemberapi/api/admin"
	loginQRHandler "bcmemberapi/api/login-qr"
	tableOrderHandler "bcmemberapi/api/table-order"
)

// RegisterRoutes registers all HTTP routes
func (s *Server) RegisterRoutes() {
	// Original routes
	http.HandleFunc("/api/login/code", s.HandleGenerateCode)
	http.HandleFunc("/api/login/status", s.HandleCheckStatus)
	http.HandleFunc("/api/login/qr", s.HandleGenerateQR)
	http.HandleFunc("/api/login/liff-verify", s.HandleLiffVerify)
	http.HandleFunc("/callback", s.HandleLineCallback)

	// Admin routes (rewrite path to ?action= like Vercel)
	http.HandleFunc("/api/admin/", func(w http.ResponseWriter, r *http.Request) {
		action := strings.TrimPrefix(r.URL.Path, "/api/admin/")
		action = strings.Split(action, "/")[0] // take first segment
		if action == "" {
			action = r.URL.Query().Get("action")
		}
		// Inject action as query parameter
		q := r.URL.Query()
		q.Set("action", action)
		r.URL.RawQuery = q.Encode()
		adminHandler.Handler(w, r)
	})

	// Login QR routes (rewrite path to ?action= like Vercel)
	http.HandleFunc("/api/login-qr/", func(w http.ResponseWriter, r *http.Request) {
		action := strings.TrimPrefix(r.URL.Path, "/api/login-qr/")
		action = strings.Split(action, "/")[0]
		if action == "" {
			action = r.URL.Query().Get("action")
		}
		q := r.URL.Query()
		q.Set("action", action)
		r.URL.RawQuery = q.Encode()
		loginQRHandler.Handler(w, r)
	})

	// Member QR route (Vercel rewrites /api/member/qr to login-qr?action=kiosk)
	http.HandleFunc("/api/member/qr", func(w http.ResponseWriter, r *http.Request) {
		q := url.Values{}
		q.Set("action", "kiosk")
		r.URL.RawQuery = q.Encode()
		loginQRHandler.Handler(w, r)
	})

	// Table order routes
	http.HandleFunc("/api/table-order/", func(w http.ResponseWriter, r *http.Request) {
		tableOrderHandler.Handler(w, r)
	})
}
