package util

import (
	"net"
	"strconv"
)

// JoinHostPort 将 host 和 port 拼接为合法的 host:port 地址
// IPv6 地址自动加方括号：[::1]:8080
func JoinHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}
