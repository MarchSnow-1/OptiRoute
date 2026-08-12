package protocol

import "bytes"

// InitConnectMagic 是引导连接的固定首字节序列（16字节）
var InitConnectMagic = []byte{
	0x4F, 0x52, 0x54, 0x45, // "ORTE"
	0x00, 0x01,             // version=1
	0xDE, 0xAD, 0xBE, 0xEF, // magic suffix
	0x00, 0x00, 0x00, 0x00, // reserved
	0x0D, 0x0A,             // CRLF terminator
}

const MagicLen = 16

// MagicPrefixLen 是分层判定使用的 magic 前缀长度（4 字节）
const MagicPrefixLen = 4

// IsMagic 检查 buf 的前 MagicLen 字节是否为引导连接 Magic
func IsMagic(buf []byte) bool {
	if len(buf) < MagicLen {
		return false
	}
	return bytes.Equal(buf[:MagicLen], InitConnectMagic)
}

// IsMagicPrefix 检查 buf 是否恰好等于 InitConnectMagic 的前 4 字节。
// 仅用于连接初筛；完整 16 字节比对仍由 IsMagic 负责。
// 严格限制 len == 4，防止误把 16 字节实参传入造成语义混淆。
func IsMagicPrefix(buf []byte) bool {
	if len(buf) != MagicPrefixLen {
		return false
	}
	return bytes.Equal(InitConnectMagic[:MagicPrefixLen], buf)
}
