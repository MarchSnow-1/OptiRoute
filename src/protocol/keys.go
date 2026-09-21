// 本文件声明 protocol 包用到的消息键，各语言的文案放在 i18n/locales 下的语言文件里
// 覆盖 PPv2 包头解析各分支的报错，使这些错误经 %w 嵌入 Server Agent 的日志后仍随语言切换
// 文案是未填充的格式串，由 protocol.errf 交给 fmt.Errorf，使 %w 仍能保留错误链
//
// This file declares the message keys protocol uses, and the copy of each language lives in the language files under i18n/locales
// It covers the errors of each ParsePPv2Header branch, so those errors still follow the language once the Server Agent embeds them in a log line through %w
// A message is an unfilled format string handed to fmt.Errorf by protocol.errf, which lets a %w verb keep the error chain intact
package protocol

import (
	"fmt"

	"github.com/MarchSnow-1/OptiRoute/i18n"
)

const (
	// KeyPPv2HeaderTooShort 是实参短于 PPv2MinHeaderLen 时的键，参数依次为实参长度与该下限
	// 该检查排在最前，使后续按下标取字节的分支都能读到完整的最小包头
	//
	// KeyPPv2HeaderTooShort is the key used when the argument is shorter than PPv2MinHeaderLen, with the argument length and that lower bound as arguments
	// The check runs first, which lets every later branch that indexes into the header read the complete minimum header
	KeyPPv2HeaderTooShort i18n.Key = "protocol.ppv2_header_too_short"

	// KeyPPv2SignatureMismatch 是 12 字节签名比对失败时的键，参数为第一个不匹配的下标
	// KeyPPv2SignatureMismatch is the key used when the 12-byte signature comparison fails, whose argument is the first index that differs
	KeyPPv2SignatureMismatch i18n.Key = "protocol.ppv2_signature_mismatch"

	// KeyPPv2CommandUnsupported 是命令字节不是 0x21 时的键，参数为该字节
	// KeyPPv2CommandUnsupported is the key used when the command byte is not 0x21, whose argument is that byte
	KeyPPv2CommandUnsupported i18n.Key = "protocol.ppv2_command_unsupported"

	// KeyPPv2IPv4LengthMismatch 是 IPv4 包的地址长度字段不是 12 时的键，参数为该字段的取值
	// KeyPPv2IPv4LengthMismatch is the key used when the address length field of an IPv4 header is not 12, whose argument is the value of that field
	KeyPPv2IPv4LengthMismatch i18n.Key = "protocol.ppv2_ipv4_length_mismatch"

	// KeyPPv2IPv4HeaderTooShort 是实参短于 PPv2HeaderLenIPv4 时的键，参数依次为实参长度与该完整包头长度
	// KeyPPv2IPv4HeaderTooShort is the key used when the argument is shorter than PPv2HeaderLenIPv4, with the argument length and that complete header length as arguments
	KeyPPv2IPv4HeaderTooShort i18n.Key = "protocol.ppv2_ipv4_header_too_short"

	// KeyPPv2IPv6LengthMismatch 是 IPv6 包的地址长度字段不是 36 时的键，参数为该字段的取值
	// KeyPPv2IPv6LengthMismatch is the key used when the address length field of an IPv6 header is not 36, whose argument is the value of that field
	KeyPPv2IPv6LengthMismatch i18n.Key = "protocol.ppv2_ipv6_length_mismatch"

	// KeyPPv2IPv6HeaderTooShort 是实参短于 PPv2HeaderLenIPv6 时的键，参数依次为实参长度与该完整包头长度
	// KeyPPv2IPv6HeaderTooShort is the key used when the argument is shorter than PPv2HeaderLenIPv6, with the argument length and that complete header length as arguments
	KeyPPv2IPv6HeaderTooShort i18n.Key = "protocol.ppv2_ipv6_header_too_short"

	// KeyPPv2FamilyUnsupported 是地址族与传输层字节不是 0x11 或 0x21 时的键，参数为该字节
	// KeyPPv2FamilyUnsupported is the key used when the address family and transport byte is neither 0x11 nor 0x21, whose argument is that byte
	KeyPPv2FamilyUnsupported i18n.Key = "protocol.ppv2_family_unsupported"
)

// errf 按消息键取当前语言的格式串并交给 fmt.Errorf
// 取的是未填充的格式串而不是已渲染的文本，使 %w 仍由 fmt.Errorf 处理，错误链得以保留
//
// errf takes the format string of a message key in the current language and hands it to fmt.Errorf
// The unfilled format string is used rather than rendered text, so a %w verb is still handled by fmt.Errorf and the error chain survives
func errf(key i18n.Key, args ...any) error {
	return fmt.Errorf(i18n.T(key), args...)
}
