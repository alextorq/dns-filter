package checks

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"time"

	domain_inspect "github.com/alextorq/dns-filter/domain-inspect"
)

// HTTPDoer is the outbound HTTP port shared by provider checks. Production
// supplies one timeout-configured *http.Client from the composition root;
// tests can inject isolated clients without swapping package globals.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Clock is the wall-clock port used by checks whose verdict depends on age.
// RDAP uses it to derive registration age deterministically.
type Clock interface {
	Now() time.Time
}

// ClockFunc adapts a function (normally time.Now) to Clock.
type ClockFunc func() time.Time

func (f ClockFunc) Now() time.Time { return f() }

func requireDependency(name string, dep any) {
	if dep == nil {
		panic("domain-inspect/checks: " + name + " is required")
	}
	v := reflect.ValueOf(dep)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			panic("domain-inspect/checks: " + name + " is required")
		}
	}
}

func requireEndpoint(name, endpoint string) {
	if endpoint == "" {
		panic("domain-inspect/checks: " + name + " endpoint is required")
	}
}

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
