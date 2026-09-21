// 本文件覆盖引导连接 Magic 的完整比对与 4 字节前缀初筛，重点验证前缀判定只接受长度恰好为 4 的实参
// 另外锁住逐段字节布局，以及「除前 4 字节外没有独立读取方」这一描述：改动任一字节都会让完整比对失败，说明这些字节参与等值比较而不是被单独解析
//
// This file covers the full comparison of the bootstrap Magic and the 4-byte prefix pre-filter, checking in particular that the prefix helper only accepts an argument of exactly 4 bytes
// It also pins the per-segment byte layout together with the statement that no reader looks at the bytes beyond the first 4: changing any single byte breaks the full comparison, which shows those bytes take part in the equality check rather than being parsed on their own

package protocol

import (
	"bytes"
	"testing"
)

// TestMagicLayout 覆盖 Magic 的固定布局：总长 16，且各段字节与 magic.go 的注释描述一致
// TestMagicLayout covers the fixed layout of the Magic: 16 bytes in total with each segment matching the description in magic.go
func TestMagicLayout(t *testing.T) {
	if len(InitConnectMagic) != MagicLen {
		t.Fatalf("InitConnectMagic length = %d, want %d", len(InitConnectMagic), MagicLen)
	}
	if MagicLen != 16 {
		t.Fatalf("MagicLen = %d, want 16", MagicLen)
	}
	if MagicPrefixLen != 4 {
		t.Fatalf("MagicPrefixLen = %d, want 4", MagicPrefixLen)
	}

	// 段的划分与 magic.go 的注释一致，逐段核对字节值
	// The segmentation matches the comment in magic.go, and each segment is checked byte by byte
	segments := []struct {
		name string
		from int
		to   int
		want []byte
	}{
		{"ORTE literal", 0, 4, []byte{0x4F, 0x52, 0x54, 0x45}},
		{"version", 4, 6, []byte{0x00, 0x01}},
		{"DEADBEEF suffix", 6, 10, []byte{0xDE, 0xAD, 0xBE, 0xEF}},
		{"reserved", 10, 14, []byte{0x00, 0x00, 0x00, 0x00}},
		{"CRLF terminator", 14, 16, []byte{0x0D, 0x0A}},
	}
	for _, s := range segments {
		if got := InitConnectMagic[s.from:s.to]; !bytes.Equal(got, s.want) {
			t.Fatalf("%s = % x, want % x", s.name, got, s.want)
		}
	}
}

// TestMagicRoundTrip 覆盖完整比对的基本语义：常量本身命中，少一字节即不命中
// TestMagicRoundTrip covers the basic semantics of the full comparison: the constant itself matches while one byte short does not
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

	// 末尾附加业务字节不影响判定，因为只比较前 16 字节
	// Trailing payload bytes do not affect the check, because only the first 16 bytes are compared
	if !IsMagic(append(append([]byte(nil), InitConnectMagic...), 0xFF, 0xFE)) {
		t.Fatal("trailing bytes after the magic should not affect the check")
	}

	// 逐字节改动，确认每个位置都参与等值比较
	// Every single byte is flipped to confirm that each position takes part in the equality comparison
	for i := 0; i < MagicLen; i++ {
		mutated := append([]byte(nil), InitConnectMagic...)
		mutated[i] ^= 0xFF
		if IsMagic(mutated) {
			t.Fatalf("flipping byte %d should break the full comparison", i)
		}
	}

	// 长度不足时返回 false，调用方因此不必先判断长度
	// A short buffer reports false, so a caller does not have to check the length first
	for n := 0; n < MagicLen; n++ {
		if IsMagic(InitConnectMagic[:n]) {
			t.Fatalf("a %d-byte buffer should not match", n)
		}
	}
}

// TestIsMagicPrefixStrictLength 覆盖前缀判定的严格长度要求
// TestIsMagicPrefixStrictLength covers the strict length requirement of the prefix check
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
	for n := 0; n < MagicLen; n++ {
		if n == MagicPrefixLen {
			continue
		}
		if IsMagicPrefix(InitConnectMagic[:n]) {
			t.Fatalf("a %d-byte buffer should not match the prefix check", n)
		}
	}

	// 前缀命中但整体不命中：这种组合说明分层判定的两层各自独立
	// A prefix hit without a full hit, which shows the two layers of the decision stay independent
	prefixOnly := append([]byte(nil), InitConnectMagic...)
	prefixOnly[MagicPrefixLen] ^= 0xFF
	if !IsMagicPrefix(prefixOnly[:MagicPrefixLen]) {
		t.Fatal("the prefix of a mutated magic should still match the prefix check")
	}
	if IsMagic(prefixOnly) {
		t.Fatal("a mutated magic should not pass the full comparison")
	}
}
