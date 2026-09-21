// 本文件实现 Proxy Protocol v2 包头的构造与解析，供 Edge 向源站注入真实客户端地址、Server Agent 在接入时取出该地址
// 单独成文的原因：PPv2 的字节布局由外部规范固定，注入端与解析端必须共用同一份布局和长度校验，否则两侧会各自演进到不兼容
// This file implements building and parsing of the Proxy Protocol v2 header, so that the Edge can inject the real client address towards the origin and the Server Agent can read that address back on accept
// It is a separate file because the PPv2 byte layout is fixed by an external specification, and the injecting side and the parsing side must share one layout and one set of length checks, otherwise the two would evolve into incompatible versions

package protocol

import (
	"encoding/binary"
	"net"
)

const (
	PPv2MinHeaderLen  = 16 // 签名(12) + 命令(1) + 族(1) + 长度(2) / signature(12) + command(1) + family(1) + length(2)
	PPv2HeaderLenIPv4 = 28 // IPv4/TCP 完整包头 / the complete IPv4/TCP header
	PPv2HeaderLenIPv6 = 52 // IPv6/TCP 完整包头 / the complete IPv6/TCP header
)

// ppv2Signature 是 PPv2 规范的 12 字节签名：前 4 字节为 CRLF CRLF，随后 0x00、CRLF、ASCII 串 QUIT 与一个换行字节
// ppv2Signature is the 12-byte signature of the PPv2 specification: the first 4 bytes are CRLF CRLF, followed by 0x00, CRLF, the ASCII string QUIT and one newline byte
var ppv2Signature = []byte{
	0x0D, 0x0A, 0x0D, 0x0A, 0x00, 0x0D, 0x0A, 0x51, 0x55, 0x49, 0x54, 0x0A,
}

// BuildPPv2Header 构造 Proxy Protocol v2 包头，自动检测 IPv4/IPv6
// 地址族由 srcIP 与 dstIP 共同决定：两者都能转成 4 字节时输出 28 字节的 IPv4 包头，否则输出 52 字节的 IPv6 包头
// BuildPPv2Header builds a Proxy Protocol v2 header and detects IPv4/IPv6 automatically
// The address family follows srcIP and dstIP together: when both convert to 4 bytes the result is the 28-byte IPv4 header, otherwise it is the 52-byte IPv6 header
func BuildPPv2Header(srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16) []byte {
	src4 := srcIP.To4()

	// dstIP 为空（nil 或 net.IP{}）时按 srcIP 地址族补全，避免误判为 IPv6
	// An empty dstIP (nil or net.IP{}) is filled in from the address family of srcIP, so it is not mistaken for IPv6
	if len(dstIP) == 0 {
		if src4 != nil {
			dstIP = net.IPv4zero
		} else {
			dstIP = net.IPv6zero
		}
	}
	dst4 := dstIP.To4()

	if src4 != nil && dst4 != nil {
		// IPv4/TCP: 28 字节 / IPv4/TCP: 28 bytes
		hdr := make([]byte, PPv2HeaderLenIPv4)
		copy(hdr[0:12], ppv2Signature)
		hdr[12] = 0x21                             // 版本 2、命令 PROXY / version=2, command=PROXY
		hdr[13] = 0x11                             // 地址族 IPv4、传输层 TCP / family=IPv4, protocol=TCP
		binary.BigEndian.PutUint16(hdr[14:16], 12) // 长度字段：4+4+2+2 / the length field: 4+4+2+2
		copy(hdr[16:20], src4)
		copy(hdr[20:24], dst4)
		binary.BigEndian.PutUint16(hdr[24:26], srcPort)
		binary.BigEndian.PutUint16(hdr[26:28], dstPort)
		return hdr
	}

	// IPv6/TCP: 52 字节 / IPv6/TCP: 52 bytes
	src16 := srcIP.To16()
	dst16 := dstIP.To16()
	hdr := make([]byte, PPv2HeaderLenIPv6)
	copy(hdr[0:12], ppv2Signature)
	hdr[12] = 0x21                             // 版本 2、命令 PROXY / version=2, command=PROXY
	hdr[13] = 0x21                             // 地址族 IPv6、传输层 TCP / family=IPv6, protocol=TCP
	binary.BigEndian.PutUint16(hdr[14:16], 36) // 长度字段：16+16+2+2 / the length field: 16+16+2+2
	copy(hdr[16:32], src16)
	copy(hdr[32:48], dst16)
	binary.BigEndian.PutUint16(hdr[48:50], srcPort)
	binary.BigEndian.PutUint16(hdr[50:52], dstPort)
	return hdr
}

// ParsePPv2Header 从包头中解析出客户端真实 IP 和端口，支持 IPv4 和 IPv6
// 依次校验最小 16 字节长度、12 字节签名与命令字节 0x21，再按地址族字节分支：长度字段与该族应有值不符，或实参长度不足该族完整包头时返回错误
// ParsePPv2Header parses the real client IP and port out of the header and supports IPv4 and IPv6
// It checks the 16-byte minimum length, the 12-byte signature and the 0x21 command byte in that order, then branches on the family byte: a length field that disagrees with the value expected for that family, or an argument shorter than the complete header of that family, is an error
func ParsePPv2Header(hdr []byte) (srcIP net.IP, srcPort uint16, err error) {
	if len(hdr) < PPv2MinHeaderLen {
		return nil, 0, errf(KeyPPv2HeaderTooShort, len(hdr), PPv2MinHeaderLen)
	}
	for i, b := range ppv2Signature {
		if hdr[i] != b {
			return nil, 0, errf(KeyPPv2SignatureMismatch, i)
		}
	}
	if hdr[12] != 0x21 {
		return nil, 0, errf(KeyPPv2CommandUnsupported, hdr[12])
	}

	switch hdr[13] {
	case 0x11: // IPv4/TCP / 地址族 IPv4、传输层 TCP
		if binary.BigEndian.Uint16(hdr[14:16]) != 12 {
			return nil, 0, errf(KeyPPv2IPv4LengthMismatch, binary.BigEndian.Uint16(hdr[14:16]))
		}
		if len(hdr) < PPv2HeaderLenIPv4 {
			return nil, 0, errf(KeyPPv2IPv4HeaderTooShort, len(hdr), PPv2HeaderLenIPv4)
		}
		srcIP = make(net.IP, 4)
		copy(srcIP, hdr[16:20])
		srcPort = binary.BigEndian.Uint16(hdr[24:26])
	case 0x21: // IPv6/TCP / 地址族 IPv6、传输层 TCP
		if binary.BigEndian.Uint16(hdr[14:16]) != 36 {
			return nil, 0, errf(KeyPPv2IPv6LengthMismatch, binary.BigEndian.Uint16(hdr[14:16]))
		}
		if len(hdr) < PPv2HeaderLenIPv6 {
			return nil, 0, errf(KeyPPv2IPv6HeaderTooShort, len(hdr), PPv2HeaderLenIPv6)
		}
		srcIP = make(net.IP, 16)
		copy(srcIP, hdr[16:32])
		srcPort = binary.BigEndian.Uint16(hdr[48:50])
	default:
		return nil, 0, errf(KeyPPv2FamilyUnsupported, hdr[13])
	}

	return srcIP, srcPort, nil
}
