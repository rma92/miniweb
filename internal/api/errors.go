package api

import (
	"encoding/json"
	"net/http"

	"github.com/user/miniweb/internal/session"
)

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// statusForSessionErr maps session package errors to HTTP status codes.
func statusForSessionErr(err error) int {
	switch err {
	case session.ErrNotFound:
		return http.StatusNotFound
	case session.ErrForbidden:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}
