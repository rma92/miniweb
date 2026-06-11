// Package adblock provides a domain-based ad/tracker blocklist with support
// for loading EasyList / ABP filter lists.
package adblock

import (
	"strings"
	"sync"
)

// BuiltinDomains is a minimal list of well-known ad, tracking, and analytics
// domains used as the baseline when no filter lists are configured.
var BuiltinDomains = []string{
	// Google advertising
	"doubleclick.net", "googlesyndication.com", "googleadservices.com",
	"googletagmanager.com", "googletagservices.com", "google-analytics.com",
	// Meta
	"facebook.net", "connect.facebook.net",
	// Amazon ads
	"amazon-adsystem.com",
	// Microsoft
	"bat.bing.com",
	// Twitter/X
	"ads-twitter.com", "analytics.twitter.com",
	// AppNexus / Xandr
	"adnxs.com",
	// Criteo
	"criteo.com", "criteo.net",
	// Taboola / Outbrain
	"taboola.com", "outbrain.com",
	// Media.net / OpenX / Rubicon
	"media.net", "openx.net", "rubiconproject.com",
	// PubMatic / Index Exchange / Sovrn
	"pubmatic.com", "casalemedia.com", "lijit.com",
	// TradeDesk / Sizmek
	"adsrvr.org", "serving-sys.com",
	// Analytics/tracking
	"hotjar.com", "mixpanel.com", "segment.io", "segment.com",
	"amplitude.com", "fullstory.com", "heapanalytics.com",
	"hs-analytics.net", "chartbeat.com", "chartbeat.net",
	"scorecardresearch.com", "imrworldwide.com", "quantserve.com",
	"sharethis.com", "addthis.com",
}

// Matcher checks URLs against a configurable blocklist. Thread-safe.
type Matcher struct {
	mu      sync.RWMutex
	domains map[string]bool // exact domain → block
}

// NewMatcher builds a Matcher seeded with the built-in list plus any extra domains.
func NewMatcher(extraDomains []string) *Matcher {
	m := &Matcher{}
	all := make([]string, 0, len(BuiltinDomains)+len(extraDomains))
	all = append(all, BuiltinDomains...)
	all = append(all, extraDomains...)
	m.loadLocked(all)
	return m
}

// LoadDomains atomically replaces the domain set with a new one.
// Used by the filter-list refresh goroutine.
func (m *Matcher) LoadDomains(domains []string) {
	m.mu.Lock()
	m.loadLocked(domains)
	m.mu.Unlock()
}

// AddDomains merges additional domains into the current set (thread-safe).
func (m *Matcher) AddDomains(domains []string) {
	m.mu.Lock()
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" {
			m.domains[d] = true
		}
	}
	m.mu.Unlock()
}

func (m *Matcher) loadLocked(domains []string) {
	set := make(map[string]bool, len(domains))
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" {
			set[d] = true
		}
	}
	m.domains = set
}

// Len returns the number of blocked domains.
func (m *Matcher) Len() int {
	m.mu.RLock()
	n := len(m.domains)
	m.mu.RUnlock()
	return n
}

// ShouldBlock returns true if the URL matches a blocked domain.
func (m *Matcher) ShouldBlock(rawURL string) bool {
	host := extractHost(rawURL)
	if host == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Check exact match and each suffix ("ads.example.com" → also check "example.com").
	for {
		if m.domains[host] {
			return true
		}
		dot := strings.IndexByte(host, '.')
		if dot < 0 {
			break
		}
		host = host[dot+1:]
	}
	return false
}

// FetchPatterns returns CDP Fetch URL patterns for all blocked domains.
// We build one wildcard pattern per domain; CDP only pauses matching requests.
func (m *Matcher) FetchPatterns() []string {
	m.mu.RLock()
	patterns := make([]string, 0, len(m.domains))
	for d := range m.domains {
		patterns = append(patterns, "*."+d+"/*")
		patterns = append(patterns, "://"+d+"/*")
	}
	m.mu.RUnlock()
	return patterns
}

// extractHost pulls the hostname from a URL string without allocating a full URL parse.
func extractHost(rawURL string) string {
	// Strip scheme.
	s := rawURL
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	// Strip path.
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	// Strip port.
	if i := strings.LastIndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(s)
}
