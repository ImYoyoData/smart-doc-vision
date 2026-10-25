package api

import "testing"

func TestIsDevFrontendOrigin(t *testing.T) {
	cases := []struct {
		origin string
		want   bool
	}{
		{"http://2.168.31.254:5173", true},
		{"http://192.168.1.20:8080", true},
		{"https://10.0.0.8:5173", true},
		{"http://localhost:5173", false},
		{"http://example.com:5173", false},
		{"http://192.168.1.20:9000", false},
		{"not-an-origin", false},
	}
	for _, tc := range cases {
		if got := isDevFrontendOrigin(tc.origin); got != tc.want {
			t.Errorf("isDevFrontendOrigin(%q)=%v want %v", tc.origin, got, tc.want)
		}
	}
}
