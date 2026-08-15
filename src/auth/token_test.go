package auth

import (
	"bytes"
	"testing"
	"time"
)

func TestUpdateSecretVersionOrdering(t *testing.T) {
	a := NewAuthManager()
	s1 := bytes.Repeat([]byte{0x01}, 32)
	s2 := bytes.Repeat([]byte{0x02}, 32)
	s3 := bytes.Repeat([]byte{0x03}, 32)

	a.UpdateSecret(s1, 30, 1)
	a.UpdateSecret(s2, 30, 2)

	a.UpdateSecret(s1, 30, 1)
	if !bytes.Equal(a.current, s2) {
		t.Fatalf("older secret was accepted: current=%x", a.current[:4])
	}
	if a.currentVersion != 2 {
		t.Fatalf("currentVersion = %d, want 2", a.currentVersion)
	}

	a.UpdateSecret(s3, 30, 2)
	if !bytes.Equal(a.current, s2) {
		t.Fatalf("duplicate version replaced secret: current=%x", a.current[:4])
	}

	// 不兼容旧 Center：version=0 必须忽略。
	a.UpdateSecret(s3, 30, 0)
	if !bytes.Equal(a.current, s2) {
		t.Fatal("legacy version=0 push should be ignored")
	}
}

func TestUpdateSecretAfterResetVersion(t *testing.T) {
	a := NewAuthManager()
	a.UpdateSecret(bytes.Repeat([]byte{0x01}, 32), 30, 5)

	a.ResetVersion()
	a.UpdateSecret(bytes.Repeat([]byte{0x02}, 32), 30, 1)
	if a.currentVersion != 1 {
		t.Fatalf("currentVersion = %d, want 1 after reset", a.currentVersion)
	}
	if !bytes.Equal(a.current, bytes.Repeat([]byte{0x02}, 32)) {
		t.Fatal("new center secret was not accepted after reset")
	}
}

func TestCommSecretHeaderRoundTrip(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	header := BuildCommSecretHeader(secret)
	if !VerifyCommSecretHeader(header, secret) {
		t.Fatal("valid Authorization header was rejected")
	}
	if VerifyCommSecretHeader(header, "ffffffffffffffffffffffffffffffff") {
		t.Fatal("wrong secret should be rejected")
	}
	if VerifyCommSecretHeader("", secret) {
		t.Fatal("missing header should be rejected")
	}
	if VerifyCommSecretHeader("Bearer zzzz", secret) {
		t.Fatal("malformed hex should be rejected")
	}
	if VerifyCommSecretHeader("Bearer "+header, secret) {
		t.Fatal("duplicate Bearer prefix should be rejected")
	}
}

func TestSecretFingerprint(t *testing.T) {
	if got := secretFingerprint(nil); got != "<empty>" {
		t.Fatalf("empty fingerprint = %q", got)
	}
	fp := secretFingerprint(bytes.Repeat([]byte{0xab}, 32))
	if len(fp) != 11 || fp[8:] != "..." {
		t.Fatalf("unexpected fingerprint: %q", fp)
	}
}

func TestGenerateAndVerifyRouteToken(t *testing.T) {
	a := NewAuthManager()
	a.UpdateSecret(bytes.Repeat([]byte{0x33}, 32), 30, 1)

	token, ts, err := a.GenerateRouteToken("target-edge", "bootstrap-edge", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	if !a.VerifyRouteToken(token, ts, "target-edge", "192.0.2.10", 30) {
		t.Fatal("valid route token should verify")
	}
	if a.VerifyRouteToken(token, ts, "target-edge", "192.0.2.11", 30) {
		t.Fatal("client IP mismatch should be rejected")
	}
	if a.VerifyRouteToken(token, ts, "other-edge", "192.0.2.10", 30) {
		t.Fatal("target UUID mismatch should be rejected")
	}
	if a.VerifyRouteToken(token, ts+1, "target-edge", "192.0.2.10", 30) {
		t.Fatal("timestamp mismatch should be rejected")
	}
	if a.VerifyRouteToken(token, ts, "target-edge", "192.0.2.10", 30) {
		t.Fatal("replayed route token should be rejected")
	}
}

func TestGenerateRouteTokenWithoutClientIPBinding(t *testing.T) {
	a := NewAuthManager()
	a.UpdateSecret(bytes.Repeat([]byte{0x33}, 32), 30, 1)

	token, ts, err := a.GenerateRouteToken("target-edge", "bootstrap-edge", "")
	if err != nil {
		t.Fatal(err)
	}
	if !a.VerifyRouteToken(token, ts, "target-edge", "", 30) {
		t.Fatal("token without client IP binding should verify with empty IP")
	}
}

func TestVerifyRouteTokenMalformed(t *testing.T) {
	a := NewAuthManager()
	a.UpdateSecret(bytes.Repeat([]byte{0x33}, 32), 30, 1)

	if a.VerifyRouteToken("not-a-token", time.Now().Unix(), "target-edge", "192.0.2.10", 30) {
		t.Fatal("missing v2 prefix should be rejected")
	}
	if a.VerifyRouteToken("v2:!!!not-base64!!!", time.Now().Unix(), "target-edge", "192.0.2.10", 30) {
		t.Fatal("bad base64 should be rejected")
	}
}

func TestRouteTokenPreviousSecretTransition(t *testing.T) {
	a := NewAuthManager()
	secret1 := bytes.Repeat([]byte{0x01}, 32)
	secret2 := bytes.Repeat([]byte{0x02}, 32)
	a.UpdateSecret(secret1, 30, 1)

	token, ts, err := a.GenerateRouteToken("target-edge", "bootstrap-edge", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}

	a.UpdateSecret(secret2, 30, 2)
	if !a.VerifyRouteToken(token, ts, "target-edge", "192.0.2.10", 30) {
		t.Fatal("route token signed with previous secret should verify during transition")
	}
}
