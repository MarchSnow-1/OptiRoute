package protocol

import (
	"encoding/binary"
	"fmt"
	"net"
)

const (
	PPv2MinHeaderLen  = 16 // 签名(12) + 命令(1) + 族(1) + 长度(2)
	PPv2HeaderLenIPv4 = 28 // IPv4/TCP 完整包头
	PPv2HeaderLenIPv6 = 52 // IPv6/TCP 完整包头
)

var ppv2Signature = []byte{
	0x0D, 0x0A, 0x0D, 0x0A, 0x00, 0x0D, 0x0A, 0x51, 0x55, 0x49, 0x54, 0x0A,
}

// BuildPPv2Header 构造 Proxy Protocol v2 包头，自动检测 IPv4/IPv6
func BuildPPv2Header(srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16) []byte {
	src4 := srcIP.To4()

	// dstIP 为空（nil 或 net.IP{}）时按 srcIP 地址族补全，避免误判为 IPv6。
	if len(dstIP) == 0 {
		if src4 != nil {
			dstIP = net.IPv4zero
		} else {
			dstIP = net.IPv6zero
		}
	}
	dst4 := dstIP.To4()

	if src4 != nil && dst4 != nil {
		// IPv4/TCP: 28 字节
		hdr := make([]byte, PPv2HeaderLenIPv4)
		copy(hdr[0:12], ppv2Signature)
		hdr[12] = 0x21                             // version=2, command=PROXY
		hdr[13] = 0x11                             // family=IPv4, protocol=TCP
		binary.BigEndian.PutUint16(hdr[14:16], 12) // 4+4+2+2
		copy(hdr[16:20], src4)
		copy(hdr[20:24], dst4)
		binary.BigEndian.PutUint16(hdr[24:26], srcPort)
		binary.BigEndian.PutUint16(hdr[26:28], dstPort)
		return hdr
	}

	// IPv6/TCP: 52 字节
	src16 := srcIP.To16()
	dst16 := dstIP.To16()
	hdr := make([]byte, PPv2HeaderLenIPv6)
	copy(hdr[0:12], ppv2Signature)
	hdr[12] = 0x21                             // version=2, command=PROXY
	hdr[13] = 0x21                             // family=IPv6, protocol=TCP
	binary.BigEndian.PutUint16(hdr[14:16], 36) // 16+16+2+2
	copy(hdr[16:32], src16)
	copy(hdr[32:48], dst16)
	binary.BigEndian.PutUint16(hdr[48:50], srcPort)
	binary.BigEndian.PutUint16(hdr[50:52], dstPort)
	return hdr
}

// ParsePPv2Header 从包头中解析出客户端真实 IP 和端口，支持 IPv4 和 IPv6
func ParsePPv2Header(hdr []byte) (srcIP net.IP, srcPort uint16, err error) {
	if len(hdr) < PPv2MinHeaderLen {
		return nil, 0, fmt.Errorf("包头长度不足: %d < %d", len(hdr), PPv2MinHeaderLen)
	}
	for i, b := range ppv2Signature {
		if hdr[i] != b {
			return nil, 0, fmt.Errorf("PPv2 签名校验失败，位置 %d", i)
		}
	}
	if hdr[12] != 0x21 {
		return nil, 0, fmt.Errorf("不支持的命令字节: 0x%02X", hdr[12])
	}

	switch hdr[13] {
	case 0x11: // IPv4/TCP
		if binary.BigEndian.Uint16(hdr[14:16]) != 12 {
			return nil, 0, fmt.Errorf("IPv4 包头长度字段不匹配: %d != 12", binary.BigEndian.Uint16(hdr[14:16]))
		}
		if len(hdr) < PPv2HeaderLenIPv4 {
			return nil, 0, fmt.Errorf("IPv4 包头长度不足: %d < %d", len(hdr), PPv2HeaderLenIPv4)
		}
		srcIP = make(net.IP, 4)
		copy(srcIP, hdr[16:20])
		srcPort = binary.BigEndian.Uint16(hdr[24:26])
	case 0x21: // IPv6/TCP
		if binary.BigEndian.Uint16(hdr[14:16]) != 36 {
			return nil, 0, fmt.Errorf("IPv6 包头长度字段不匹配: %d != 36", binary.BigEndian.Uint16(hdr[14:16]))
		}
		if len(hdr) < PPv2HeaderLenIPv6 {
			return nil, 0, fmt.Errorf("IPv6 包头长度不足: %d < %d", len(hdr), PPv2HeaderLenIPv6)
		}
		srcIP = make(net.IP, 16)
		copy(srcIP, hdr[16:32])
		srcPort = binary.BigEndian.Uint16(hdr[48:50])
	default:
		return nil, 0, fmt.Errorf("不支持的协议族/类型: 0x%02X", hdr[13])
	}

	return srcIP, srcPort, nil
}
