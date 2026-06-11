// Package adblock provides a simple domain-based ad/tracker blocklist.
package adblock

import "strings"

// BuiltinDomains is a minimal list of well-known ad, tracking, and analytics domains.
// Each entry is a suffix match: "doubleclick.net" blocks "ad.doubleclick.net" etc.
var BuiltinDomains = []string{
	// Google advertising
	"doubleclick.net",
	"googlesyndication.com",
	"googleadservices.com",
	"googletagmanager.com",
	"googletagservices.com",
	"google-analytics.com",
	"adservice.google.com",
	"pagead2.googlesyndication.com",
	// Meta / Facebook
	"facebook.net",
	"facebook.com/tr",
	"connect.facebook.net",
	// Amazon
	"amazon-adsystem.com",
	"aax.amazon-adsystem.com",
	// Microsoft
	"bat.bing.com",
	"c.bing.com",
	// Twitter/X
	"ads-twitter.com",
	"analytics.twitter.com",
	// AppNexus / Xandr
	"adnxs.com",
	// Criteo
	"criteo.com",
	"criteo.net",
	// Taboola / Outbrain
	"taboola.com",
	"outbrain.com",
	// Media.net
	"media.net",
	// OpenX
	"openx.net",
	"openx.com",
	// Rubicon / Magnite
	"rubiconproject.com",
	// PubMatic
	"pubmatic.com",
	// Index Exchange
	"casalemedia.com",
	// Sovrn
	"lijit.com",
	// Advertising.com
	"advertising.com",
	// TradeDesk
	"adsrvr.org",
	// ShareThis
	"sharethis.com",
	// AddThis
	"addthis.com",
	// Hotjar
	"hotjar.com",
	// Mixpanel
	"mixpanel.com",
	// Segment
	"segment.io",
	"segment.com",
	// Amplitude
	"amplitude.com",
	// FullStory
	"fullstory.com",
	// Heap
	"heapanalytics.com",
	// Intercom
	"intercomcdn.com",
	// HubSpot tracking
	"hs-analytics.net",
	"hubspot.com/analytics",
	// Chartbeat
	"chartbeat.com",
	"chartbeat.net",
	// ComScore
	"scorecardresearch.com",
	"comscore.com",
	// Nielsen
	"imrworldwide.com",
	// Lotame
	"crwdcntrl.net",
	// Quantcast
	"quantserve.com",
	// Turn
	"turn.com",
	// Yieldmo
	"yieldmo.com",
	// Moat
	"moatads.com",
	// Sizmek
	"serving-sys.com",
	// Verizon Media / Oath
	"oath.com",
	"advertising.com",
}

// Matcher checks URLs against the blocklist.
type Matcher struct {
	domains []string
	extra   []string
}

// NewMatcher builds a Matcher from the built-in list plus any extra domains.
func NewMatcher(extraDomains []string) *Matcher {
	return &Matcher{
		domains: BuiltinDomains,
		extra:   extraDomains,
	}
}

// ShouldBlock returns true if the URL matches a blocked domain.
func (m *Matcher) ShouldBlock(rawURL string) bool {
	u := strings.ToLower(rawURL)
	for _, d := range m.domains {
		if containsDomain(u, d) {
			return true
		}
	}
	for _, d := range m.extra {
		if containsDomain(u, strings.ToLower(d)) {
			return true
		}
	}
	return false
}

// FetchPatterns returns CDP Fetch URL patterns for all blocked domains.
// Using explicit patterns is more efficient: only matching requests are paused.
func (m *Matcher) FetchPatterns() []string {
	seen := make(map[string]bool)
	var patterns []string
	for _, d := range m.domains {
		p := "*" + d + "*"
		if !seen[p] {
			patterns = append(patterns, p)
			seen[p] = true
		}
	}
	for _, d := range m.extra {
		p := "*" + strings.ToLower(d) + "*"
		if !seen[p] {
			patterns = append(patterns, p)
			seen[p] = true
		}
	}
	return patterns
}

func containsDomain(url, domain string) bool {
	// Match as suffix after "://" host component, or anywhere in the URL.
	if strings.Contains(url, "://"+domain) ||
		strings.Contains(url, "."+domain) ||
		strings.Contains(url, "/"+domain) {
		return true
	}
	return false
}
