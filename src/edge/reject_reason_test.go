// 本文件覆盖 Center 拒绝注册时原因码在两端的处理
// 跨进程字段只传机器可读的原因码，由接收方按自己的语言渲染，因此这里断言：已知码渲染成本机语言、未知码原样回显
//
// This file covers how a reason code for a rejected registration is handled at both ends
// Only a machine-readable reason code crosses the process boundary and the receiver renders it in its own language, so the cases here assert that a known code renders in the local language while an unknown one is echoed as it stands
package edge

import (
	"strings"
	"testing"

	"github.com/MarchSnow-1/OptiRoute/i18n"
	"github.com/MarchSnow-1/OptiRoute/protocol"
)

// setLangForTest 把当前语言设为给定值，并在用例结束后还原为默认值
// setLangForTest sets the current language and restores the default when the case ends
func setLangForTest(t *testing.T, lang i18n.Lang) {
	t.Helper()
	i18n.Set(lang)
	t.Cleanup(func() { i18n.Set(i18n.Default()) })
}

// TestRejectReasonTextFollowsLocalLanguage 验证同一个原因码在三种本机语言下渲染出各自的文本
// TestRejectReasonTextFollowsLocalLanguage checks that one reason code renders its own text under each of the three local languages
func TestRejectReasonTextFollowsLocalLanguage(t *testing.T) {
	cases := map[protocol.RegisterRejectReason]map[i18n.Lang]string{
		protocol.RejectReasonFieldsInvalid: {
			i18n.LangEN:     "a registration field failed its range check",
			i18n.LangZHHans: "注册字段未通过边界校验",
			i18n.LangZHant:  "註冊欄位未通過邊界校驗",
		},
		protocol.RejectReasonCapacityReached: {
			i18n.LangEN:     "the number of online Edges has reached its cap",
			i18n.LangZHHans: "在线 Edge 数量已达上限",
			i18n.LangZHant:  "線上 Edge 數量已達上限",
		},
	}

	for reason, perLang := range cases {
		for lang, want := range perLang {
			setLangForTest(t, lang)
			if got := rejectReasonText(reason); got != want {
				t.Errorf("rejectReasonText(%q) 在 %q 下 = %q，期望 %q / under %q the text is %q, want %q", reason, lang, got, want, lang, got, want)
			}
		}
	}
}

// TestRejectReasonTextCoversEveryDefinedCode 验证 protocol 定义的每个原因码都有本机文案
// 该断言使 Center 新增一个码而本机未同步时，本包的测试先失败，而不是让运维在日志里看到码
//
// TestRejectReasonTextCoversEveryDefinedCode checks that every reason code defined by protocol has local text
// The assertion makes a code added on the Center without a local follow-up fail this package's tests, instead of leaving an operator to read a bare code in the log
func TestRejectReasonTextCoversEveryDefinedCode(t *testing.T) {
	defined := []protocol.RegisterRejectReason{
		protocol.RejectReasonPayloadInvalid,
		protocol.RejectReasonFieldsInvalid,
		protocol.RejectReasonRateLimited,
		protocol.RejectReasonCapacityReached,
	}

	setLangForTest(t, i18n.LangZHHans)
	for _, reason := range defined {
		text := rejectReasonText(reason)

		// 未定义的码会走未知分支并回显该码，因此文本里出现码本身就说明该码没有被本机认识
		// An undefined code takes the unknown arm and echoes itself, so the code appearing in the text means this build does not know it
		if text == "" || strings.Contains(text, string(reason)) {
			t.Errorf("原因码 %q 没有本机文案，渲染结果 %q / the reason code %q has no local text, rendered %q", reason, text, reason, text)
		}
		if text == i18n.T(KeyRejectReasonUnknown, reason) {
			t.Errorf("原因码 %q 落到了未知分支 / the reason code %q fell through to the unknown arm", reason, reason)
		}
	}
}

// TestRejectReasonTextUnknownCodeIsEchoed 验证本机不认识的原因码被原样回显
// 回显使 Center 新增码时本机不必同时升级，运维仍能在日志里看到 Center 下发的那个码
//
// TestRejectReasonTextUnknownCodeIsEchoed checks that a reason code this build does not know is echoed as it stands
// The echo keeps a code added on the Center from requiring a simultaneous local upgrade, and an operator still sees the code the Center sent
func TestRejectReasonTextUnknownCodeIsEchoed(t *testing.T) {
	setLangForTest(t, i18n.LangZHHans)

	for _, reason := range []protocol.RegisterRejectReason{"some_future_code", "rate_limited_v2"} {
		got := rejectReasonText(reason)
		if got == "" {
			t.Fatalf("未知原因码 %q 渲染为空串 / the unknown reason code %q rendered as an empty string", reason, reason)
		}
		if !strings.Contains(got, string(reason)) {
			t.Errorf("未知原因码 %q 未被回显，实际 %q / the unknown reason code %q was not echoed, got %q", reason, got, reason, got)
		}
	}
}
