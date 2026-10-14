package internal

import (
	"testing"
	"time"
)

func TestServiceNamePattern(t *testing.T) {
	cases := []struct {
		srv, name string
		want      bool
	}{
		{"res-backend", "/res-backend-" + time.Now().Format(nameLayout), true},
		{"res-backend", "/res-backend-10.10-04.44.07", true},
		{"res-backend", "res-backend-10.10-04.44.07", true},
		{"res-backend", "/res-backend-admin-10.15-00.15.52", false},
		{"res", "/res-backend-10.15-00.15.52", false},
		{"cdk-backend", "/cdk-backend", false},
		{"user-frontend", "/user-frontend-10.10-04.43.32-old", false},
		{"a.b", "/axb-10.10-04.43.32", false},
	}
	for _, c := range cases {
		if got := serviceNamePattern(c.srv).MatchString(c.name); got != c.want {
			t.Errorf("serviceNamePattern(%q).MatchString(%q) = %v, want %v", c.srv, c.name, got, c.want)
		}
	}
}
