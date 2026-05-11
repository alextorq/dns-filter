package checks

import (
	"context"
	"errors"
	"net/http"
	"time"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

// httpClient is shared by all outbound-HTTP checks. Per-call deadlines are
// driven by the request context passed into Inspect(); this timeout is the
// hard ceiling that protects against a misbehaving caller that did not set one.
var httpClient = &http.Client{Timeout: 10 * time.Second}

func errorResult(err error) domain_inspect.CheckResult {
	return domain_inspect.CheckResult{Status: domain_inspect.StatusError, Error: err.Error()}
}

// contextErrorResult turns a transport error into a "timeout" result when the
// caller's context expired, so the UI can distinguish slow upstreams from
// broken ones. Any other transport failure becomes a regular error.
func contextErrorResult(ctx context.Context, err error) domain_inspect.CheckResult {
	if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(ctxErr, context.DeadlineExceeded) {
		return domain_inspect.CheckResult{Status: domain_inspect.StatusTimeout, Error: ctxErr.Error()}
	}
	return errorResult(err)
}

func skipped(reason string) domain_inspect.CheckResult {
	return domain_inspect.CheckResult{Status: domain_inspect.StatusSkipped, Error: reason}
}
