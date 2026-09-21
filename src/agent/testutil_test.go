// 本文件提供 agent 包测试共用的辅助函数：把 net 地址中的端口字符串转成 int，转换失败即用 t.Fatal 结束用例
// 之所以单列一个文件：该辅助函数被 client_test.go 的两个临时服务构造函数共用，放在独立文件里便于其它测试文件复用
// This file holds the helper shared by the tests of the agent package: it converts the port string taken from a net address into an int and ends the case with t.Fatal when the conversion fails
// It lives in its own file because the two temporary-service constructors of client_test.go share this helper, and keeping it in a separate file makes it reusable by other test files

package agent

import (
	"strconv"
	"testing"
)

// mustPort 把端口字符串转成 int，转换失败时调用 t.Fatal 结束当前用例
// mustPort converts a port string into an int and calls t.Fatal to end the current case when the conversion fails
func mustPort(t *testing.T, s string) int {
	t.Helper()
	p, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
