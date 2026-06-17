package util

import (
	"net"
	"strconv"
	"strings"
)

// JoinHostPort 将 host 和 port 拼接为合法的 host:port 地址
// 自动处理已有方括号的 IPv6 地址，避免双重包裹
func JoinHostPort(host string, port int) string {
	h := strings.TrimPrefix(host, "[")
	h = strings.TrimSuffix(h, "]")
	return net.JoinHostPort(h, strconv.Itoa(port))
}
