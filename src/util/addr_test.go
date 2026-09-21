// 本文件覆盖 util.JoinHostPort 的地址拼接，重点验证带方括号与不带方括号的 IPv6 字面量得到同一个结果
// This file covers the address joining of util.JoinHostPort, checking in particular that an IPv6 literal with and without square brackets gives the same result

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
