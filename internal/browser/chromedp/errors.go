package chromedp

import (
	"context"
	"strings"
)

// wrapNavError converts raw chromedp/CDP errors into friendlier messages.
func wrapNavError(url string, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()

	switch {
	case err == context.DeadlineExceeded || strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "context deadline"):
		return &BrowserError{Code: "timeout", Message: "page load timed out for " + url}

	case err == context.Canceled || strings.Contains(msg, "context canceled"):
		return &BrowserError{Code: "canceled", Message: "navigation canceled"}

	case strings.Contains(msg, "net::ERR_NAME_NOT_RESOLVED") ||
		strings.Contains(msg, "NameNotResolved"):
		return &BrowserError{Code: "dns_failure", Message: "DNS lookup failed: " + url}

	case strings.Contains(msg, "net::ERR_CONNECTION_REFUSED"):
		return &BrowserError{Code: "connection_refused", Message: "connection refused: " + url}

	case strings.Contains(msg, "net::ERR_CONNECTION_TIMED_OUT"):
		return &BrowserError{Code: "connection_timeout", Message: "connection timed out: " + url}

	case strings.Contains(msg, "net::ERR_INTERNET_DISCONNECTED"):
		return &BrowserError{Code: "offline", Message: "no internet connection"}

	case strings.Contains(msg, "net::ERR_CERT") || strings.Contains(msg, "SSL"):
		return &BrowserError{Code: "tls_error", Message: "TLS/certificate error for " + url}

	case strings.Contains(msg, "Page.navigate") || strings.Contains(msg, "Cannot navigate"):
		return &BrowserError{Code: "nav_error", Message: "navigation failed: " + msg}

	default:
		return &BrowserError{Code: "browser_error", Message: msg}
	}
}

// BrowserError is a structured error from the browser layer.
type BrowserError struct {
	Code    string
	Message string
}

func (e *BrowserError) Error() string { return e.Message }
