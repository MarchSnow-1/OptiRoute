// 本文件提供 host 与 port 的拼接工具，供各角色生成监听地址、探测地址与 WebSocket URL
// 单独成文的原因：仓库里拼接地址的地方很多，若各处直接写 host + ":" + port，带方括号的 IPv6 字面量会被拼成双重包裹的形式
// This file provides the host-and-port join helper used by every role to build listen addresses, probe addresses and WebSocket URLs
// It is a separate file because addresses are joined in many places, and writing host + ":" + port inline would double-wrap an IPv6 literal that already carries square brackets

package util

import (
	"net"
	"strconv"
	"strings"
)

// JoinHostPort 将 host 和 port 拼接为合法的 host:port 地址
// 先去掉 host 两端已有的方括号，再交给 net.JoinHostPort，因此 "[::1]" 与 "::1" 得到同一个结果；port 不在此处做范围校验，按十进制原样写入
// JoinHostPort joins host and port into a valid host:port address
// It strips existing square brackets from both ends of host before calling net.JoinHostPort, so "[::1]" and "::1" give the same result; port is not range-checked here and is written in decimal as given
func JoinHostPort(host string, port int) string {
	h := strings.TrimPrefix(host, "[")
	h = strings.TrimSuffix(h, "]")
	return net.JoinHostPort(h, strconv.Itoa(port))
}
