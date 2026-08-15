package util

import "testing"

func TestJoinHostPort(t *testing.T) {
	cases := []struct {
		host string
		port int
		want string
	}{
		{"127.0.0.1", 80, "127.0.0.1:80"},
		{"example.com", 443, "example.com:443"},
		{"[::1]", 443, "[::1]:443"},
		{"::1", 443, "[::1]:443"},
	}
	for _, c := range cases {
		if got := JoinHostPort(c.host, c.port); got != c.want {
			t.Fatalf("JoinHostPort(%q, %d) = %q, want %q", c.host, c.port, got, c.want)
		}
	}
}
