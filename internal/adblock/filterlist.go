package adblock

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FilterListConfig describes a filter list source.
type FilterListConfig struct {
	URLs         []string
	RefreshHours int    // 0 → 24h default
	CacheDir     string // "" → ".adblock_cache"
}

// StartRefreshLoop downloads and parses the configured filter lists, then
// merges them into matcher. It re-runs every RefreshHours. The loop exits when
// ctx is cancelled.
func StartRefreshLoop(ctx context.Context, cfg FilterListConfig, matcher *Matcher) {
	if len(cfg.URLs) == 0 {
		return
	}
	cacheDir := cfg.CacheDir
	if cacheDir == "" {
		cacheDir = ".adblock_cache"
	}
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		log.Printf("adblock: cannot create cache dir %s: %v", cacheDir, err)
	}

	interval := time.Duration(cfg.RefreshHours) * time.Hour
	if interval <= 0 {
		interval = 24 * time.Hour
	}

	refresh := func() {
		var all []string
		// Keep builtin + extra domains already in the matcher.
		// Collect only newly parsed filter-list domains.
		for _, u := range cfg.URLs {
			domains, err := fetchAndParse(ctx, u, cacheDir)
			if err != nil {
				log.Printf("adblock: %s: %v", u, err)
				continue
			}
			log.Printf("adblock: loaded %d domains from %s", len(domains), u)
			all = append(all, domains...)
		}
		if len(all) > 0 {
			matcher.AddDomains(all)
			log.Printf("adblock: total blocked domains: %d", matcher.Len())
		}
	}

	// Run immediately.
	go func() {
		refresh()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				refresh()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// fetchAndParse downloads (or reads from cache) a filter list and returns
// the list of blocked domains extracted from it.
func fetchAndParse(ctx context.Context, listURL, cacheDir string) ([]string, error) {
	cacheFile := filepath.Join(cacheDir, urlCacheKey(listURL)+".txt")

	// Try cache first if it's fresh enough (< 23h old).
	if fi, err := os.Stat(cacheFile); err == nil {
		if time.Since(fi.ModTime()) < 23*time.Hour {
			return parseFile(cacheFile)
		}
	}

	// Download.
	reqCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, listURL, nil)
	if err != nil {
		return parseFileFallback(cacheFile)
	}
	req.Header.Set("User-Agent", "MiniNext/1.0 adblock-updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return parseFileFallback(cacheFile)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return parseFileFallback(cacheFile)
	}

	// Write to cache.
	f, err := os.CreateTemp(cacheDir, "*.tmp")
	if err != nil {
		// Parse directly from response body without caching.
		return parseReader(resp.Body)
	}
	tmpName := f.Name()
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpName)
		return nil, err
	}
	f.Close()
	if err := os.Rename(tmpName, cacheFile); err != nil {
		os.Remove(tmpName)
	}

	return parseFile(cacheFile)
}

// parseFile reads a cached filter list file.
func parseFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseReader(f)
}

// parseFileFallback returns cached data (even if stale) on network error.
func parseFileFallback(path string) ([]string, error) {
	if _, err := os.Stat(path); err == nil {
		return parseFile(path)
	}
	return nil, nil // no cache and no network: skip silently
}

// parseReader parses an EasyList/ABP format filter list and returns domains.
// Only network-blocking domain rules are extracted; cosmetic rules are skipped.
func parseReader(r io.Reader) ([]string, error) {
	var domains []string
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if d, ok := parseRule(line); ok {
			domains = append(domains, d)
		}
	}
	return domains, scanner.Err()
}

// parseRule extracts a blocked domain from a single filter-list line.
// Returns ("", false) for rules we can't or don't want to handle.
func parseRule(line string) (string, bool) {
	if line == "" || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "[") {
		return "", false
	}
	// Skip exception rules (@@) and cosmetic rules (##, #@#).
	if strings.HasPrefix(line, "@@") || strings.Contains(line, "##") ||
		strings.Contains(line, "#@#") || strings.Contains(line, "#?#") {
		return "", false
	}
	// Only handle domain-anchor rules: ||domain.tld^  or  ||domain.tld^$options
	if !strings.HasPrefix(line, "||") {
		return "", false
	}
	// Strip leading ||
	line = line[2:]
	// Strip options after $
	if i := strings.IndexByte(line, '$'); i >= 0 {
		opts := line[i+1:]
		// Skip rules with domain= restrictions or ~third-party, document, etc.
		if strings.Contains(opts, "domain=") {
			return "", false
		}
		line = line[:i]
	}
	// Strip trailing separator ^ (and anything after it)
	if i := strings.IndexByte(line, '^'); i >= 0 {
		line = line[:i]
	}
	// Strip trailing wildcard
	line = strings.TrimRight(line, "*")
	// Strip trailing slash
	line = strings.TrimRight(line, "/")

	// Must look like a domain: at least one dot, no spaces, no path separator complexity
	if !looksLikeDomain(line) {
		return "", false
	}
	return strings.ToLower(line), true
}

func looksLikeDomain(s string) bool {
	if s == "" || !strings.Contains(s, ".") {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func urlCacheKey(u string) string {
	h := sha256.Sum256([]byte(u))
	return hex.EncodeToString(h[:8])
}
