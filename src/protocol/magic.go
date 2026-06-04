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

// IsMagic 检查 buf 的前 MagicLen 字节是否为引导连接 Magic
func IsMagic(buf []byte) bool {
	if len(buf) < MagicLen {
		return false
	}
	return bytes.Equal(buf[:MagicLen], InitConnectMagic)
}
