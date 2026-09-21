// 本文件覆盖引导连接 Magic 的完整比对与 4 字节前缀初筛，重点验证前缀判定只接受长度恰好为 4 的实参
// This file covers the full comparison of the bootstrap Magic and the 4-byte prefix pre-filter, checking in particular that the prefix helper only accepts an argument of exactly 4 bytes

package protocol

import "testing"

func TestMagicRoundTrip(t *testing.T) {
	if len(InitConnectMagic) != MagicLen {
		t.Fatalf("magic len = %d, want %d", len(InitConnectMagic), MagicLen)
	}
	if !IsMagic(InitConnectMagic) {
		t.Fatal("valid magic was rejected")
	}
	if IsMagic(append([]byte(nil), InitConnectMagic[:MagicLen-1]...)) {
		t.Fatal("short magic should be rejected")
	}
}

func TestIsMagicPrefixStrictLength(t *testing.T) {
	if !IsMagicPrefix(InitConnectMagic[:MagicPrefixLen]) {
		t.Fatal("valid prefix was rejected")
	}
	if IsMagicPrefix(InitConnectMagic) {
		t.Fatal("full magic should not match strict prefix helper")
	}
	if IsMagicPrefix(InitConnectMagic[:MagicPrefixLen-1]) {
		t.Fatal("short prefix should be rejected")
	}
}
