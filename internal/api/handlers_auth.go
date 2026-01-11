package api

import (
	"encoding/json"
	"net/http"

	"github.com/clustercost/clustercost-dashboard/internal/auth"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

// Login handles user authentication and returns a JWT.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	token, err := auth.Login(h.db, req.Username, req.Password)
	if err != nil {
		// Log error internally if needed, but return generic error to user
		// In a real app, distinguish between internal error and invalid creds safely
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{Token: token})
}
