// 本文件验证 auth 签发路径的报错在英文模式下不含中文字符
// 这些错误由 Edge 引导流程经 %w 嵌进日志行，因此按真实调用路径触发一次再断言渲染结果
//
// This file checks that the errors of the auth issuing path carry no Chinese character in English mode
// The Edge bootstrap flow nests these errors into a log line through %w, so one real call path is triggered before the rendered result is asserted

package auth

import (
	"testing"

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

// TestGenerateRouteTokenErrorHasNoChineseInEnglishMode 验证英文模式下密钥未初始化的签发报错不含中文字符
// 回归点：该文案此前以 fmt.Errorf 写死为中文，英文模式下会把中文片段带进引导流程的日志行
//
// TestGenerateRouteTokenErrorHasNoChineseInEnglishMode checks that the error of issuing without a secret carries no Chinese character in English mode
// The regression it pins: the text used to be a hard-coded Chinese fmt.Errorf, which in English mode put a Chinese fragment into the log line of the bootstrap flow
func TestGenerateRouteTokenErrorHasNoChineseInEnglishMode(t *testing.T) {
	useEnglishForTest(t)

	// 新建的 AuthManager 尚未接受过 secret 推送，因此签发停在密钥为空的检查上，random 与签名路径都走不到
	// A freshly built AuthManager has taken no secret push, so issuing stops at the empty-secret check and reaches neither the random nor the signing path
	a := NewAuthManager()
	_, _, err := a.GenerateRouteToken("uuid-example-target", "uuid-example-issuer", "192.0.2.10")
	if err == nil {
		t.Fatal("密钥未初始化时签发应报错 / issuing should fail while the secret is unset")
	}
	if hasCJK(err.Error()) {
		t.Fatalf("英文模式下报错含中文: %q / the error carries Chinese text in English mode: %q", err.Error(), err.Error())
	}
	if err.Error() != i18n.Text(i18n.LangEN, KeyTokenSecretUninitialized) {
		t.Fatalf("报错未取自目录的英文文案: %q / the error does not carry the English text from the catalogue: %q", err.Error(), err.Error())
	}
}
