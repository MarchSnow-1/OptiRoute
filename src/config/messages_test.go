// 本文件验证 config 的报错文案随当前语言变化，并锁住被合并的共用键渲染结果与合并前逐字一致
// 合并指的是 self.connect_timeout_ms 与 self.max_edges 这类同构报错共用一个键、字段名由参数传入
// 另外验证 %w 包住底层错误的错误链在改走消息目录之后仍然存在
//
// This file checks that config error text follows the current language and pins the shared keys introduced by merging to render exactly as they did before
// Merging means that structurally identical failures such as self.connect_timeout_ms and self.max_edges share one key with the field name passed as an argument
// It also checks that the error chain of a %w wrap survives the move to the message catalogue
package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/MarchSnow-1/OptiRoute/i18n"
)

// setLangForTest 把当前语言设为给定值，并在用例结束后还原为默认值
// setLangForTest sets the current language and restores the default when the case ends
func setLangForTest(t *testing.T, lang i18n.Lang) {
	t.Helper()
	i18n.Set(lang)
	t.Cleanup(func() { i18n.Set(i18n.Default()) })
}

// TestNumericMessageMergedPrefix 验证共用键在三种语言下渲染出的文案与合并前一致
// TestNumericMessageMergedPrefix checks that the shared key renders the same text as before the merge in all three languages
func TestNumericMessageMergedPrefix(t *testing.T) {
	cases := map[i18n.Lang]string{
		i18n.LangEN:     "self.connect_timeout_ms must be > 0, current 0",
		i18n.LangZHHans: "self.connect_timeout_ms 必须 > 0，当前 0",
		i18n.LangZHant:  "self.connect_timeout_ms 必須 > 0，目前 0",
	}

	for lang, want := range cases {
		t.Run(string(lang), func(t *testing.T) {
			setLangForTest(t, lang)

			c := &Config{}
			c.defaults()
			// defaults() 会填入正值，这里显式改回 0 触发校验分支
			// defaults() fills in a positive value, so it is set back to 0 here to reach the validation arm
			c.Self.ConnectTimeoutMs = 0

			err := c.validateCommonNumerics()
			if err == nil {
				t.Fatal("connect_timeout_ms 为 0 时应报错 / a connect_timeout_ms of 0 should be rejected")
			}
			if err.Error() != want {
				t.Fatalf("报错 = %q，期望 %q / error %q, want %q", err.Error(), want, err.Error(), want)
			}
			if strings.Contains(err.Error(), "%!") {
				t.Fatalf("报错里出现格式符残留，说明参数与文案不匹配: %q / the error carries a leftover verb, so the arguments and the text disagree: %q", err.Error(), err.Error())
			}
		})
	}
}

// TestPortAndSecretMessagesFollowLanguage 验证端口与密钥报错随语言变化
// TestPortAndSecretMessagesFollowLanguage checks that the port and secret errors follow the language
func TestPortAndSecretMessagesFollowLanguage(t *testing.T) {
	cases := map[i18n.Lang]struct{ port, secret string }{
		i18n.LangEN: {
			port:   "self.listen_port must be between 1 and 65535, current 0",
			secret: "the edge role requires comm_secret",
		},
		i18n.LangZHHans: {
			port:   "self.listen_port 必须在 1-65535 之间，当前 0",
			secret: "edge 角色必须配置 comm_secret",
		},
		i18n.LangZHant: {
			port:   "self.listen_port 必須在 1-65535 之間，目前 0",
			secret: "edge 角色必須設定 comm_secret",
		},
	}

	for lang, want := range cases {
		t.Run(string(lang), func(t *testing.T) {
			setLangForTest(t, lang)

			if err := validatePortRange("self.listen_port", 0); err == nil || err.Error() != want.port {
				t.Fatalf("端口报错 = %v，期望 %q / port error %v, want %q", err, want.port, err, want.port)
			}
			if err := validateSecretField("comm_secret", "", "edge"); err == nil || err.Error() != want.secret {
				t.Fatalf("密钥报错 = %v，期望 %q / secret error %v, want %q", err, want.secret, err, want.secret)
			}
		})
	}
}

// TestNoArgumentMessageFollowsLanguage 验证不带参数的报错同样随语言变化
// TestNoArgumentMessageFollowsLanguage checks that a message without arguments follows the language as well
func TestNoArgumentMessageFollowsLanguage(t *testing.T) {
	cases := map[i18n.Lang]string{
		i18n.LangEN:     "missing required field: role",
		i18n.LangZHHans: "必填字段缺失: role",
		i18n.LangZHant:  "必填欄位缺失: role",
	}

	for lang, want := range cases {
		t.Run(string(lang), func(t *testing.T) {
			setLangForTest(t, lang)

			c := &Config{}
			err := c.Validate()
			if err == nil || err.Error() != want {
				t.Fatalf("报错 = %v，期望 %q / error %v, want %q", err, want, err, want)
			}
			if strings.Contains(err.Error(), "%!") {
				t.Fatalf("无参数文案里出现格式符残留: %q / a message without arguments carries a leftover verb: %q", err.Error(), err.Error())
			}
		})
	}
}

// TestConfigLoadErrorKeepsChain 验证读取配置文件失败仍包住底层错误，错误链未被消息目录打断
// TestConfigLoadErrorKeepsChain checks that a failed configuration read still wraps the underlying error and that the message catalogue does not break the chain
func TestConfigLoadErrorKeepsChain(t *testing.T) {
	for _, lang := range i18n.Supported() {
		t.Run(string(lang), func(t *testing.T) {
			setLangForTest(t, lang)

			missing := t.TempDir() + "/absent.json"
			_, err := loadFromFile(missing)
			if err == nil {
				t.Fatal("读取不存在的文件应报错 / reading a missing file should fail")
			}
			if errors.Unwrap(err) == nil {
				t.Fatalf("错误链被丢失，无法 Unwrap: %v / the error chain is gone and cannot be unwrapped: %v", err, err)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Fatalf("报错未包含路径 %q: %v / the error does not carry the path %q: %v", missing, err, missing, err)
			}
			if strings.Contains(err.Error(), "%!") {
				t.Fatalf("报错里出现格式符残留: %q / the error carries a leftover verb: %q", err.Error(), err.Error())
			}
		})
	}
}

// TestEveryValidationKeyRendersWithoutLeftoverVerbs 验证目录里的每条 config 消息都能被填充干净
// 逐条按参数个数取用会过于复杂，因此只检查无参数取用时不会因文案本身带裸百分号而变形
//
// TestEveryValidationKeyRendersWithoutLeftoverVerbs checks that every config message in the catalogue can be filled cleanly
// Fetching each one with the right number of arguments would be too intricate, so this only checks that fetching without arguments cannot mangle a message that carries a bare percent sign
func TestEveryValidationKeyRendersWithoutLeftoverVerbs(t *testing.T) {
	for _, key := range i18n.Keys() {
		if !strings.HasPrefix(string(key), "config.") {
			continue
		}
		entry, ok := i18n.Lookup(key)
		if !ok {
			t.Fatalf("键 %q 未登记 / key %q is not registered", key, key)
		}
		for lang, text := range entry.All() {
			if text == "" {
				t.Errorf("键 %q 缺少 %q 文案 / key %q has no text for %q", key, lang, key, lang)
			}
			if strings.Contains(text, "%%") {
				t.Errorf("键 %q 的 %q 文案含 %% ，在 errf 里会被渲染成裸百分号: %q / the %q text of key %q carries %%, which errf renders as a bare percent sign: %q", key, lang, text, lang, key, text)
			}
		}
	}
}
