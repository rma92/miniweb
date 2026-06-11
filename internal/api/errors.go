package api

import (
	"encoding/json"
	"errors"
	"net/http"

	cdpbrowser "github.com/user/miniweb/internal/browser/chromedp"
	"github.com/user/miniweb/internal/session"
)

type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

func writeError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{Error: msg})
}

func writeBrowserError(w http.ResponseWriter, err error) {
	var be *cdpbrowser.BrowserError
	if errors.As(err, &be) {
		status := http.StatusBadGateway
		if be.Code == "timeout" || be.Code == "connection_timeout" {
			status = http.StatusGatewayTimeout
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(errorResponse{Error: be.Message, Code: be.Code})
		return
	}
	writeError(w, err.Error(), http.StatusBadGateway)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// statusForSessionErr maps session/browser errors to HTTP status codes.
func statusForSessionErr(err error) int {
	switch err {
	case session.ErrNotFound:
		return http.StatusNotFound
	case session.ErrForbidden:
		return http.StatusForbidden
	default:
		var be *cdpbrowser.BrowserError
		if errors.As(err, &be) {
			if be.Code == "timeout" || be.Code == "connection_timeout" {
				return http.StatusGatewayTimeout
			}
			return http.StatusBadGateway
		}
		return http.StatusInternalServerError
	}
}
