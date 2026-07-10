package sync

import (
	"net/http"
	"reflect"
)

// HTTPDoer is the transport port shared by the production source loaders.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

func isNilHTTPDoer(client HTTPDoer) bool {
	if client == nil {
		return true
	}
	v := reflect.ValueOf(client)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
