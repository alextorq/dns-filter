package checks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestCredentials_InstancesAreIndependent(t *testing.T) {
	first := NewCredentials()
	second := NewCredentials()

	first.SetVirusTotal("vt-one")
	first.SetSafeBrowsing("sb-one")

	if first.VirusTotalKey() != "vt-one" || first.SafeBrowsingKey() != "sb-one" || !first.HasAnyKey() {
		t.Fatalf("first credentials did not retain keys")
	}
	if second.VirusTotalKey() != "" || second.SafeBrowsingKey() != "" || second.HasAnyKey() {
		t.Fatalf("credentials leaked across instances")
	}
}

func TestCredentials_RuntimeUpdatesAreVisibleToChecks(t *testing.T) {
	var vtKeys, sbKeys []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			vtKeys = append(vtKeys, r.Header.Get("x-apikey"))
			_, _ = w.Write([]byte(`{"data":{"attributes":{"last_analysis_stats":{"harmless":1}}}}`))
		case r.Method == http.MethodPost:
			sbKeys = append(sbKeys, r.URL.Query().Get("key"))
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer ts.Close()

	prevVT, prevSB := vtEndpoint, sbEndpoint
	vtEndpoint, sbEndpoint = ts.URL+"/vt/", ts.URL+"/sb"
	t.Cleanup(func() { vtEndpoint, sbEndpoint = prevVT, prevSB })

	keys := NewCredentials()
	vt := NewVirusTotal(keys)
	sb := NewSafeBrowsing(keys)

	if got := vt(context.Background(), "example.com"); got.Status != "skipped" {
		t.Fatalf("VT without key: got %q", got.Status)
	}
	if got := sb(context.Background(), "example.com"); got.Status != "skipped" {
		t.Fatalf("SB without key: got %q", got.Status)
	}

	for _, pair := range [][2]string{{"vt-one", "sb-one"}, {"vt-two", "sb-two"}} {
		keys.SetVirusTotal(pair[0])
		keys.SetSafeBrowsing(pair[1])
		if got := vt(context.Background(), "example.com"); got.Status != "ok" {
			t.Fatalf("VT after key update: got %q (%s)", got.Status, got.Error)
		}
		if got := sb(context.Background(), "example.com"); got.Status != "ok" {
			t.Fatalf("SB after key update: got %q (%s)", got.Status, got.Error)
		}
	}

	if want := []string{"vt-one", "vt-two"}; !reflect.DeepEqual(vtKeys, want) {
		t.Errorf("existing VT closure did not observe rotations: got %v want %v", vtKeys, want)
	}
	if want := []string{"sb-one", "sb-two"}; !reflect.DeepEqual(sbKeys, want) {
		t.Errorf("existing SB closure did not observe rotations: got %v want %v", sbKeys, want)
	}
}

func TestProviderConstructors_RejectNilCredentials(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func()
	}{
		{name: "VirusTotal", call: func() { NewVirusTotal(nil) }},
		{name: "Safe Browsing", call: func() { NewSafeBrowsing(nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected nil credentials to panic")
				}
			}()
			tc.call()
		})
	}
}
