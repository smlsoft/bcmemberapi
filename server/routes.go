package server

import "net/http"

// RegisterRoutes registers all HTTP routes
func (s *Server) RegisterRoutes() {
	http.HandleFunc("/api/login/code", s.HandleGenerateCode)
	http.HandleFunc("/api/login/status", s.HandleCheckStatus)
	http.HandleFunc("/api/login/qr", s.HandleGenerateQR)
	http.HandleFunc("/api/login/liff-verify", s.HandleLiffVerify)
	http.HandleFunc("/callback", s.HandleLineCallback)
}
