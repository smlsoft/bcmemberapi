package server

import "net/http"

// RegisterRoutes registers all HTTP routes
func (s *Server) RegisterRoutes() {
	http.HandleFunc("/api/login/code", s.HandleGenerateCode)
	http.HandleFunc("/api/login/status", s.HandleCheckStatus)
	http.HandleFunc("/callback", s.HandleLineCallback)
}
