// 本文件定义引导连接的固定首字节序列与分层判定函数，供 Edge 与 Client Agent 在共用端口上区分引导连接和业务连接
// 单独成文的原因：探测地址与业务转发共用端口，判定用的字节序列必须只有一份定义，否则两端会各自漂移
// This file defines the fixed leading byte sequence of the bootstrap connection and the layered matching helpers, so that the Edge and the Client Agent can tell a bootstrap connection from a service connection on the shared port
// It is a separate file because the probe address and the service forwarding share one port, and the byte sequence used for that decision must be defined once, otherwise the two ends would drift apart

package protocol

import "bytes"

// InitConnectMagic 是引导连接的固定首字节序列（16字节）
// 布局为 "ORTE"（4 字节）+ 版本 1（2 字节）+ DEADBEEF 后缀（4 字节）+ 预留位（4 字节，当前写 0）+ CRLF 终止符（2 字节），合计 16 字节
// 除前 4 字节被 IsMagicPrefix 用作初筛前缀外，其余字节没有独立读取方：IsMagic 对整 16 字节做等值比较，因此「版本」「预留位」与「CRLF 终止符」只是对常量布局的说明，协议层不解析这些段
//
// InitConnectMagic is the fixed leading byte sequence of the bootstrap connection (16 bytes)
// The layout is "ORTE" (4 bytes), version 1 (2 bytes), the DEADBEEF suffix (4 bytes), the reserved bytes (4 bytes, written as 0 today) and a CRLF terminator (2 bytes), 16 bytes in total
// Apart from the first 4 bytes serving as the filter prefix of IsMagicPrefix, the remaining bytes have no independent reader: IsMagic compares all 16 bytes for equality, so the version, the reserved bytes and the CRLF terminator only describe the layout of this constant and the protocol layer never parses those ranges
var InitConnectMagic = []byte{
	0x4F, 0x52, 0x54, 0x45, // "ORTE" 字面量 / the "ORTE" literal
	0x00, 0x01, // 版本 1 / version=1
	0xDE, 0xAD, 0xBE, 0xEF, // magic 后缀 / magic suffix
	0x00, 0x00, 0x00, 0x00, // 预留位 / reserved
	0x0D, 0x0A, // CRLF 终止符 / CRLF terminator
}

// MagicLen 是引导连接 Magic 的总长度（16 字节）
// MagicLen is the total length of the bootstrap Magic (16 bytes)
const MagicLen = 16

// MagicPrefixLen 是分层判定使用的 magic 前缀长度（4 字节）
// MagicPrefixLen is the length of the magic prefix used for the layered decision (4 bytes)
const MagicPrefixLen = 4

// IsMagic 检查 buf 的前 MagicLen 字节是否为引导连接 Magic
// buf 长度不足 MagicLen 时返回 false，因此调用方无需先做长度判断
// IsMagic checks whether the first MagicLen bytes of buf are the bootstrap Magic
// It reports false when buf is shorter than MagicLen, so callers do not have to check the length first
func IsMagic(buf []byte) bool {
	if len(buf) < MagicLen {
		return false
	}
	return bytes.Equal(buf[:MagicLen], InitConnectMagic)
}

// IsMagicPrefix 检查 buf 是否恰好等于 InitConnectMagic 的前 4 字节
// 仅用于连接初筛；完整 16 字节比对仍由 IsMagic 负责
// 严格限制 len == 4，因此把 16 字节实参传进来时返回 false 而不是命中前缀，避免两种语义混用
// IsMagicPrefix checks whether buf is exactly the first 4 bytes of InitConnectMagic
// It only serves as the connection pre-filter; the full 16-byte comparison stays with IsMagic
// The len == 4 requirement is strict, so passing a 16-byte argument reports false instead of matching the prefix, which keeps the two meanings apart
func IsMagicPrefix(buf []byte) bool {
	if len(buf) != MagicPrefixLen {
		return false
	}
	return bytes.Equal(InitConnectMagic[:MagicPrefixLen], buf)
}
