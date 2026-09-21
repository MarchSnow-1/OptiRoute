// 本文件验证 edge 侧的三条启动期与首帧报错在英文模式下不含中文字符
// 这些错误经 %w 被 main 的日志行嵌套，因此按真实调用路径触发一次再断言渲染结果
//
// This file checks that three start-up and first-packet errors of edge carry no Chinese character in English mode
// main nests these errors into a log line through %w, so one real call path is triggered before the rendered result is asserted

package edge

import (
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

// TestFirstPacketShortPrefixHasNoChineseInEnglishMode 验证英文模式下已读前缀不足 4 字节的报错不含中文字符
//
// TestFirstPacketShortPrefixHasNoChineseInEnglishMode checks that the short-prefix error carries no Chinese character in English mode
func TestFirstPacketShortPrefixHasNoChineseInEnglishMode(t *testing.T) {
	useEnglishForTest(t)

	_, err := readFirstPacketWithPrefix(nil, []byte{0x00, 0x01}, time.Time{})
	if err == nil {
		t.Fatal("不足 4 字节的前缀应按错误返回 / a prefix shorter than 4 bytes should be reported as an error")
	}
	assertNoChinese(t, err)
	if err.Error() != i18n.Text(i18n.LangEN, KeyFirstPacketShortPrefix) {
		t.Fatalf("报错未取自目录的英文文案: %q / the error does not carry the English text from the catalogue: %q", err.Error(), err.Error())
	}
}

// TestRegistrationFlowNotStartedHasNoChineseInEnglishMode 验证英文模式下注册流程未启动的报错不含中文字符
// 该分支在 Connect 未执行时成立，因此直接构造一个未连接过的 CenterClient 即可触发
//
// TestRegistrationFlowNotStartedHasNoChineseInEnglishMode checks that the error for a registration flow which has not started carries no Chinese character in English mode
// The branch holds while Connect has not run, so a CenterClient that never connected reaches it
func TestRegistrationFlowNotStartedHasNoChineseInEnglishMode(t *testing.T) {
	useEnglishForTest(t)

	c := &CenterClient{}
	err := c.WaitRegistration(0)
	if err == nil {
		t.Fatal("注册流程未启动时应返回错误 / an unstarted registration flow should fail")
	}
	assertNoChinese(t, err)
	if err.Error() != i18n.Text(i18n.LangEN, KeyRegistrationFlowNotStarted) {
		t.Fatalf("报错未取自目录的英文文案: %q / the error does not carry the English text from the catalogue: %q", err.Error(), err.Error())
	}
}

// assertNoChinese 断言错误文本不含中文字符，并回报实际文本
// assertNoChinese asserts that the error text carries no Chinese character and reports the actual text
func assertNoChinese(t *testing.T, err error) {
	t.Helper()
	if hasCJK(err.Error()) {
		t.Fatalf("英文模式下报错含中文: %q / the error carries Chinese text in English mode: %q", err.Error(), err.Error())
	}
	if strings.TrimSpace(err.Error()) == "" {
		t.Fatal("报错文本为空，断言无从成立 / the error text is empty, so the assertion cannot hold")
	}
}
