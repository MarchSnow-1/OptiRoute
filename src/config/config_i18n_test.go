// 本文件覆盖 Phase 12 引入的语言配置：self.lang 的默认值、--lang= 的解析与优先级
// 覆盖点：defaults() 把空 lang 填为 i18n 的默认语言、显式取值不被改写、--lang= 覆盖配置文件、重复给出时保留首个取值
// 以及「取值不在支持范围内时不阻止启动」这一约定：非受支持的标签在 defaults() 与 Validate() 之后原样保留，交由 main 回退并提示
//
// This file covers the language configuration introduced in Phase 12: the default of self.lang, the parsing of --lang= and its precedence
// The cases cover defaults() filling an empty lang with the i18n default, an explicit value surviving untouched, --lang= overriding the configuration file, the first value winning when the argument repeats
// and the rule that an unsupported value cannot stop start-up: a tag outside what i18n supports survives defaults() and Validate() as it stands, leaving the fallback and the notice to main
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MarchSnow-1/OptiRoute/i18n"
)

// withArgs 在用例执行期间替换 os.Args，并在结束后还原
// withArgs replaces os.Args for the duration of a case and restores it afterwards
func withArgs(t *testing.T, args ...string) {
	t.Helper()
	saved := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = saved })
}

// writeConfigFile 把 JSON 内容写进临时目录并返回其路径
// writeConfigFile writes the JSON content into a temporary directory and returns its path
func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入临时配置失败 / writing the temporary configuration failed: %v", err)
	}
	return path
}

// TestLangDefaultsToI18nDefault 验证 lang 留空时由 defaults() 填为 i18n 的默认语言
// TestLangDefaultsToI18nDefault checks that an empty lang is filled with the i18n default by defaults()
func TestLangDefaultsToI18nDefault(t *testing.T) {
	c := &Config{}
	c.defaults()

	if c.Self.Lang == "" {
		t.Fatal("defaults() 应填充 lang / defaults() should fill lang")
	}
	if c.Self.Lang != string(i18n.Default()) {
		t.Fatalf("默认 lang = %q，期望 %q / default lang %q, want %q", c.Self.Lang, i18n.Default(), c.Self.Lang, i18n.Default())
	}
	if _, ok := i18n.Parse(c.Self.Lang); !ok {
		t.Fatalf("默认 lang %q 无法被 i18n 解析 / the default lang %q cannot be parsed by i18n", c.Self.Lang, c.Self.Lang)
	}
}

// TestLangExplicitValueSurvives 验证显式配置的语言不被默认值改写
// TestLangExplicitValueSurvives checks that an explicitly configured language is not replaced by the default
func TestLangExplicitValueSurvives(t *testing.T) {
	for _, tag := range []string{"zh-Hans", "zh-Hant", "en-US"} {
		c := &Config{}
		c.Self.Lang = tag
		c.defaults()

		if c.Self.Lang != tag {
			t.Fatalf("显式 lang %q 被改写为 %q / the explicit lang %q was replaced by %q", tag, c.Self.Lang, tag, c.Self.Lang)
		}
	}
}

// TestUnsupportedLangDoesNotFailValidation 验证不受支持的标签不阻止启动
// 该约定使语言标签打错时进程仍能起来，由 main 回退到 en-US 并打印提示
//
// TestUnsupportedLangDoesNotFailValidation checks that an unsupported tag cannot stop start-up
// The rule keeps the process starting when the language tag is mistyped, leaving main to fall back to en-US and print a notice
func TestUnsupportedLangDoesNotFailValidation(t *testing.T) {
	build := func(tag string) *Config {
		c := &Config{}
		c.Self.Lang = tag
		c.defaults()
		c.Self.Role = RoleCenter
		c.Self.ListenPort = 7000
		c.Self.CommSecret = "0123456789abcdef0123456789abcdef"
		return c
	}

	for _, tag := range []string{"ja-JP", "en_US", ""} {
		c := build(tag)
		if err := c.Validate(); err != nil {
			t.Fatalf("lang=%q 不应导致校验失败 / lang=%q should not fail validation: %v", tag, tag, err)
		}
	}

	// 非受支持的取值原样保留，未在 config 阶段被回退，回退点只有 main.resolveLang
	// The unsupported value survives as it stands and is not rolled back during the config stage, since main.resolveLang is the only fallback
	c := build("ja-JP")
	if c.Self.Lang != "ja-JP" {
		t.Fatalf("config 阶段改写了 lang: %q / the config stage rewrote lang: %q", c.Self.Lang, c.Self.Lang)
	}
}

// TestParseArgsLang 验证 --lang= 的解析
// TestParseArgsLang checks the parsing of --lang=
func TestParseArgsLang(t *testing.T) {
	opts := parseArgs([]string{"--config-path=a.json", "--lang=zh-Hans"})
	if !opts.langSet || opts.lang != "zh-Hans" {
		t.Fatalf("--lang=zh-Hans 解析结果 lang=%q langSet=%v / parsed lang=%q langSet=%v", opts.lang, opts.langSet, opts.lang, opts.langSet)
	}

	// 未给出该参数时 langSet 为 false，配置文件中的取值因此生效
	// Without the argument langSet stays false, so the value in the configuration file takes effect
	opts = parseArgs([]string{"--config-path=a.json"})
	if opts.langSet || opts.lang != "" {
		t.Fatalf("未给出 --lang 时 lang=%q langSet=%v / without --lang lang=%q langSet=%v", opts.lang, opts.langSet, opts.lang, opts.langSet)
	}

	// 显式空串算「给出了」，否则无法区分未配置与 --lang=
	// An explicit empty value counts as given, since otherwise it could not be told apart from an omitted argument
	opts = parseArgs([]string{"--lang="})
	if !opts.langSet || opts.lang != "" {
		t.Fatalf("--lang= 应记为已给出: lang=%q langSet=%v / --lang= should count as given: lang=%q langSet=%v", opts.lang, opts.langSet, opts.lang, opts.langSet)
	}

	// 重复给出时保留首个取值，与配置来源参数的规则一致
	// A repeated argument keeps the first value, matching the rule for the configuration-source arguments
	opts = parseArgs([]string{"--lang=zh-Hant", "--lang=zh-Hans"})
	if opts.lang != "zh-Hant" {
		t.Fatalf("重复 --lang 时应保留首个取值，得到 %q / a repeated --lang should keep the first value, got %q", opts.lang, opts.lang)
	}

	// 大小写与缺省脚本由 i18n.Parse 处理，parseArgs 只做原样取值
	// Case and missing scripts are handled by i18n.Parse, while parseArgs only takes the value as it stands
	opts = parseArgs([]string{"--lang=ZH-hans"})
	if opts.lang != "ZH-hans" {
		t.Fatalf("parseArgs 应原样保留取值，得到 %q / parseArgs should keep the value as it stands, got %q", opts.lang, opts.lang)
	}
}

// TestLoadLangPrecedence 验证命令行 --lang= 优先于配置文件中的 self.lang
// TestLoadLangPrecedence checks that the command-line --lang= takes precedence over self.lang in the configuration file
func TestLoadLangPrecedence(t *testing.T) {
	path := writeConfigFile(t, `{"self":{"role":"center","lang":"zh-Hant"}}`)

	// 只给配置文件时取其取值
	// With the configuration file alone its value is used
	withArgs(t, "optiroute", "--config-path="+path)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 失败 / Load failed: %v", err)
	}
	if cfg.Self.Lang != "zh-Hant" {
		t.Fatalf("配置文件 lang = %q，期望 zh-Hant / configuration lang %q, want zh-Hant", cfg.Self.Lang, cfg.Self.Lang)
	}

	// 命令行同时给出时以命令行为准
	// With both given the command line wins
	withArgs(t, "optiroute", "--config-path="+path, "--lang=zh-Hans")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load 失败 / Load failed: %v", err)
	}
	if cfg.Self.Lang != "zh-Hans" {
		t.Fatalf("命令行 lang = %q，期望 zh-Hans / command-line lang %q, want zh-Hans", cfg.Self.Lang, cfg.Self.Lang)
	}

	// --lang= 给出空串时按给出处理并覆盖配置文件，由 main 回退到 en-US 并提示
	// An empty --lang= counts as given and overrides the configuration, leaving main to fall back to en-US with a notice
	withArgs(t, "optiroute", "--config-path="+path, "--lang=")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load 失败 / Load failed: %v", err)
	}
	if cfg.Self.Lang != "" {
		t.Fatalf("--lang= 应覆盖配置文件，得到 %q / an empty --lang= should override the configuration, got %q", cfg.Self.Lang, cfg.Self.Lang)
	}
}

// TestLoadLangDefaultsWhenAbsent 验证两个来源都未给出语言时填默认值
// TestLoadLangDefaultsWhenAbsent checks that the default is filled when neither source gives a language
func TestLoadLangDefaultsWhenAbsent(t *testing.T) {
	path := writeConfigFile(t, `{"self":{"role":"center"}}`)

	withArgs(t, "optiroute", "--config-path="+path)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load 失败 / Load failed: %v", err)
	}
	if cfg.Self.Lang != string(i18n.Default()) {
		t.Fatalf("lang = %q，期望默认值 %q / lang %q, want the default %q", cfg.Self.Lang, i18n.Default(), cfg.Self.Lang, i18n.Default())
	}
}

// TestCLILangReturnsRawValue 验证 CLILang 原样返回命令行取值与是否给出
// TestCLILangReturnsRawValue checks that CLILang returns the command-line value as it stands together with whether it was given
func TestCLILangReturnsRawValue(t *testing.T) {
	withArgs(t, "optiroute", "--lang=ZH-hans")
	tag, set := CLILang()
	if !set || tag != "ZH-hans" {
		t.Fatalf("CLILang() = %q, %v，期望 ZH-hans, true / want ZH-hans, true", tag, set)
	}

	withArgs(t, "optiroute")
	tag, set = CLILang()
	if set || tag != "" {
		t.Fatalf("未给出参数时 CLILang() = %q, %v，期望空值, false / without the argument want an empty value, false", tag, set)
	}
}
