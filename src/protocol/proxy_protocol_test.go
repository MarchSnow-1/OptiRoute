package protocol

import (
	"bytes"
	"net"
	"testing"
)

func TestBuildAndParsePPv2IPv4(t *testing.T) {
	src := net.ParseIP("192.0.2.10").To4()
	dst := net.ParseIP("198.51.100.20").To4()
	hdr := BuildPPv2Header(src, 12345, dst, 443)

	if len(hdr) != PPv2HeaderLenIPv4 {
		t.Fatalf("IPv4 header len = %d, want %d", len(hdr), PPv2HeaderLenIPv4)
	}
	gotIP, gotPort, err := ParsePPv2Header(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if !gotIP.Equal(src) || gotPort != 12345 {
		t.Fatalf("parsed src = %s:%d", gotIP, gotPort)
	}
}

func TestBuildAndParsePPv2IPv6(t *testing.T) {
	src := net.ParseIP("2001:db8::1")
	dst := net.ParseIP("2001:db8::2")
	hdr := BuildPPv2Header(src, 12345, dst, 443)

	if len(hdr) != PPv2HeaderLenIPv6 {
		t.Fatalf("IPv6 header len = %d, want %d", len(hdr), PPv2HeaderLenIPv6)
	}
	gotIP, gotPort, err := ParsePPv2Header(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if !gotIP.Equal(src) || gotPort != 12345 {
		t.Fatalf("parsed src = %s:%d", gotIP, gotPort)
	}
}

func TestBuildPPv2WithEmptyDstUsesSameFamilyZero(t *testing.T) {
	hdr := BuildPPv2Header(net.ParseIP("192.0.2.10"), 1111, net.IP{}, 0)
	if len(hdr) != PPv2HeaderLenIPv4 {
		t.Fatalf("IPv4 + empty dst should build IPv4 header, got %d", len(hdr))
	}
	got, _, err := ParsePPv2Header(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(net.ParseIP("192.0.2.10")) {
		t.Fatalf("src = %s", got)
	}
}

func TestParsePPv2HeaderRejectsMalformed(t *testing.T) {
	valid := BuildPPv2Header(net.ParseIP("192.0.2.10"), 1111, net.ParseIP("198.51.100.20"), 443)

	badSig := append([]byte(nil), valid...)
	badSig[0] = 0
	if _, _, err := ParsePPv2Header(badSig); err == nil {
		t.Fatal("bad signature should be rejected")
	}

	badCmd := append([]byte(nil), valid...)
	badCmd[12] = 0x20
	if _, _, err := ParsePPv2Header(badCmd); err == nil {
		t.Fatal("bad command should be rejected")
	}

	if _, _, err := ParsePPv2Header(valid[:PPv2MinHeaderLen-1]); err == nil {
		t.Fatal("truncated header should be rejected")
	}
}

func TestPPv2SignatureBytes(t *testing.T) {
	if !bytes.Equal(ppv2Signature, []byte{0x0D, 0x0A, 0x0D, 0x0A, 0x00, 0x0D, 0x0A, 0x51, 0x55, 0x49, 0x54, 0x0A}) {
		t.Fatal("unexpected PPv2 signature")
	}
}
