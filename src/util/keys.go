// 本文件声明 util 包用到的消息键，各语言的文案放在 i18n/locales 下的语言文件里
// 覆盖帧读写两个方向对长度上限的报错与被调方未写入任何字节的报错，使这些错误经 %w 嵌入调用方日志后仍随语言切换
// 文案是未填充的格式串，由 util.errf 交给 fmt.Errorf，使 %w 仍能保留错误链
//
// This file declares the message keys util uses, and the copy of each language lives in the language files under i18n/locales
// It covers the length-limit errors of the frame reader and writer together with a callee that wrote no byte, so those errors still follow the language once a caller embeds them in a log line through %w
// A message is an unfilled format string handed to fmt.Errorf by util.errf, which lets a %w verb keep the error chain intact
package util

import (
	"fmt"

	"github.com/MarchSnow-1/OptiRoute/i18n"
)

const (
	// KeyFrameTooLarge 是待写入帧超过 MaxFrameSize 时的键，参数为实参字节数
	// 该检查在写头部之前完成，因此连接上不留下任何字节
	//
	// KeyFrameTooLarge is the key used when the frame to write exceeds MaxFrameSize, whose argument is the byte count of the argument
	// The check runs before the header is written, so no byte reaches the connection
	KeyFrameTooLarge i18n.Key = "util.frame_too_large"

	// KeyWriteZeroBytes 是单次写返回 0 字节时的键，不带参数
	// 把它当错误处理使写入循环不会空转
	//
	// KeyWriteZeroBytes is the key used when a single write reports 0 bytes, and it takes no argument
	// Treating it as an error keeps the write loop from spinning
	KeyWriteZeroBytes i18n.Key = "util.write_zero_bytes"

	// KeyFrameLengthExceeded 是读到帧长超过 MaxFrameSize 时的键，参数为帧长度字段的取值
	// 该检查在读载荷之前完成，因此为单帧分配的缓冲区不超过 1 MB
	//
	// KeyFrameLengthExceeded is the key used when the frame length read from the connection exceeds MaxFrameSize, whose argument is the value of that length field
	// The check runs before the payload is read, so the buffer allocated for one frame stays within 1 MB
	KeyFrameLengthExceeded i18n.Key = "util.frame_length_exceeded"
)

// errf 按消息键取当前语言的格式串并交给 fmt.Errorf
// 取的是未填充的格式串而不是已渲染的文本，使 %w 仍由 fmt.Errorf 处理，错误链得以保留
//
// errf takes the format string of a message key in the current language and hands it to fmt.Errorf
// The unfilled format string is used rather than rendered text, so a %w verb is still handled by fmt.Errorf and the error chain survives
func errf(key i18n.Key, args ...any) error {
	return fmt.Errorf(i18n.T(key), args...)
}
