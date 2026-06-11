package chromedp

import (
	"context"
	"log"
	"sync/atomic"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	cdp2 "github.com/chromedp/chromedp"
	"github.com/user/miniweb/internal/adblock"
	"github.com/user/miniweb/internal/metrics"
)

// enableAdBlocking wires up CDP Fetch.enable on the given tab context.
// The enabled atomic controls whether blocking is active; it can be flipped
// at runtime for per-session toggles. When disabled the intercepted requests
// are continued normally.
func enableAdBlocking(ctx context.Context, matcher *adblock.Matcher, enabled *atomic.Bool) error {
	// Register the event listener before enabling the domain.
	cdp2.ListenTarget(ctx, func(ev interface{}) {
		e, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		go func() {
			c := cdp2.FromContext(ctx)
			if c == nil || c.Target == nil {
				return
			}
			execCtx := cdp.WithExecutor(ctx, c.Target)
			if enabled.Load() && matcher.ShouldBlock(e.Request.URL) {
				metrics.AdBlockBlocked.Inc()
				if err := fetch.FailRequest(e.RequestID, network.ErrorReasonBlockedByClient).Do(execCtx); err != nil {
					// Tab may already be closing; suppress noise.
					log.Printf("adblock: failRequest: %v", err)
				}
			} else {
				if err := fetch.ContinueRequest(e.RequestID).Do(execCtx); err != nil {
					log.Printf("adblock: continueRequest: %v", err)
				}
			}
		}()
	})

	// Enable Fetch domain for ALL requests (we do the matching in-process).
	// This is required when the domain set is large (filter lists with 70k+ entries).
	return cdp2.Run(ctx, cdp2.ActionFunc(func(ctx context.Context) error {
		return fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "*"}}).Do(ctx)
	}))
}
