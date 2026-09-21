// 本文件验证帧读写的报错在英文模式下不含中文字符
// 这些错误会经 %w 被调用方的日志行嵌套，因此只看目录条目不够，要按真实调用路径触发一次再断言渲染结果
//
// This file checks that the frame reader and writer report no Chinese character in English mode
// These errors are nested into a caller's log line through %w, so looking at the catalogue entries alone is not enough and one real call path is triggered before the rendered result is asserted

package util

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/MarchSnow-1/OptiRoute/i18n"
)

// useEnglishForTest 把当前语言设为英文，并在用例结束后还原为默认值
// useEnglishForTest sets the current language to English and restores the default once the test ends
func useEnglishForTest(t *testing.T) {
	t.Helper()
	i18n.Set(i18n.LangEN)
	t.Cleanup(func() { i18n.Set(i18n.Default()) })
}

// hasCJK 判断文本中是否出现 CJK 统一表意文字
// hasCJK reports whether the text carries a CJK unified ideograph
func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// TestReadFrameErrorHasNoChineseInEnglishMode 验证英文模式下帧长超限的报错不含中文字符
// 回归点：该文案此前以 fmt.Errorf 写死为中文，英文模式下会把中文片段带进调用方的日志行
//
// TestReadFrameErrorHasNoChineseInEnglishMode checks that the frame length limit error carries no Chinese character in English mode
// The regression it pins: the text used to be a hard-coded Chinese fmt.Errorf, which in English mode put a Chinese fragment into the caller's log line
func TestReadFrameErrorHasNoChineseInEnglishMode(t *testing.T) {
	useEnglishForTest(t)

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	go func() {
		hdr := make([]byte, 4)
		binary.BigEndian.PutUint32(hdr, MaxFrameSize+1)
		client.Write(hdr)
	}()

	_, err := ReadFrame(server, time.Second)
	if err == nil {
		t.Fatal("超限帧长应按错误返回 / an oversize frame length should be reported as an error")
	}
	if hasCJK(err.Error()) {
		t.Fatalf("英文模式下报错含中文: %q / the error carries Chinese text in English mode: %q", err.Error(), err.Error())
	}
	if !strings.Contains(err.Error(), i18n.Text(i18n.LangEN, KeyFrameLengthExceeded, MaxFrameSize+1)) {
		t.Fatalf("报错未取自目录的英文文案: %q / the error does not carry the English text from the catalogue: %q", err.Error(), err.Error())
	}
}
