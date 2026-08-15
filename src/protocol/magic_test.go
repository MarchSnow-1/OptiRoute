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
