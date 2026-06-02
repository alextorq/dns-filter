package web

import "testing"

func TestBoolOr(t *testing.T) {
	tru, fls := true, false
	cases := []struct {
		name string
		p    *bool
		def  bool
		want bool
	}{
		{"nil takes default true", nil, true, true},
		{"nil takes default false", nil, false, false},
		{"non-nil true overrides default false", &tru, false, true},
		{"non-nil false overrides default true", &fls, true, false},
	}
	for _, c := range cases {
		if got := boolOr(c.p, c.def); got != c.want {
			t.Errorf("%s: boolOr = %v, want %v", c.name, got, c.want)
		}
	}
}
