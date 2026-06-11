package chromedp

import (
	"context"
	"log"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	cdp2 "github.com/chromedp/chromedp"
	"github.com/user/miniweb/internal/adblock"
)

// enableAdBlocking wires up CDP Fetch.enable on the given tab context.
// Requests matching blocked patterns are failed before they leave the browser;
// non-matching requests are never paused (patterns filter at the CDP level).
func enableAdBlocking(ctx context.Context, matcher *adblock.Matcher) error {
	patterns := matcher.FetchPatterns()
	if len(patterns) == 0 {
		return nil
	}

	// Build CDP RequestPattern slice from the string patterns.
	cdpPatterns := make([]*fetch.RequestPattern, 0, len(patterns))
	for _, p := range patterns {
		cdpPatterns = append(cdpPatterns, &fetch.RequestPattern{URLPattern: p})
	}

	// Register the event listener before enabling the domain.
	cdp2.ListenTarget(ctx, func(ev interface{}) {
		e, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		// Run on a goroutine so we don't block the event pump.
		go func() {
			c := cdp2.FromContext(ctx)
			if c == nil || c.Target == nil {
				return
			}
			execCtx := cdp.WithExecutor(ctx, c.Target)
			if err := fetch.FailRequest(e.RequestID, network.ErrorReasonBlockedByClient).Do(execCtx); err != nil {
				log.Printf("adblock: failRequest %s: %v", e.Request.URL, err)
			}
		}()
	})

	// Enable Fetch interception for the matched patterns only.
	return cdp2.Run(ctx, cdp2.ActionFunc(func(ctx context.Context) error {
		return fetch.Enable().WithPatterns(cdpPatterns).Do(ctx)
	}))
}
