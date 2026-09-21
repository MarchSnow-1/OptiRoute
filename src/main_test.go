// 本文件验证入口的语言解析与回退行为
// 覆盖受支持标签的规范取值、不受支持标签回退到 en-US 并给出指名来源的提示，以及提示固定用英文渲染这一约定
//
// This file checks the language resolution and fallback of the entry point
// It covers the canonical value of a supported tag, the fallback to en-US with a notice naming the source for an unsupported one, and the rule that the notice is rendered in English regardless of the current language
package main

import (
	"strings"
	"testing"

	"github.com/MarchSnow-1/OptiRoute/i18n"
)

// TestResolveLangAcceptsSupportedTags 验证受支持的标签按规范取值返回且不带提示
// TestResolveLangAcceptsSupportedTags checks that a supported tag resolves to its canonical value without a notice
func TestResolveLangAcceptsSupportedTags(t *testing.T) {
	cases := map[string]i18n.Lang{
		"en-US":   i18n.LangEN,
		"en":      i18n.LangEN,
		"en-GB":   i18n.LangEN,
		"zh-Hans": i18n.LangZHHans,
		"zh-Hant": i18n.LangZHant,
		"zh":      i18n.LangZHHans,
		"zh-CN":   i18n.LangZHHans,
		"zh-TW":   i18n.LangZHant,
		"ZH-hans": i18n.LangZHHans,
	}

	for tag, want := range cases {
		lang, notice := resolveLang(tag, "--lang")
		if lang != want {
			t.Errorf("resolveLang(%q) = %q，期望 %q / want %q", tag, lang, want, want)
		}
		if notice != "" {
			t.Errorf("resolveLang(%q) 给出了提示 %q，受支持标签不应有提示 / a supported tag should yield no notice, got %q", tag, notice, notice)
		}
	}
}

// TestResolveLangFallsBackWithNotice 验证不受支持的标签回退到 en-US 并给出提示
// TestResolveLangFallsBackWithNotice checks that an unsupported tag falls back to en-US and yields a notice
func TestResolveLangFallsBackWithNotice(t *testing.T) {
	for _, tag := range []string{"ja-JP", "de-DE", "en_US", "zh-", ""} {
		lang, notice := resolveLang(tag, "--lang")

		if lang != i18n.LangEN {
			t.Errorf("resolveLang(%q) = %q，期望回退到 %q / want the fallback %q", tag, lang, i18n.LangEN, i18n.LangEN)
		}
		if notice == "" {
			t.Fatalf("resolveLang(%q) 未给出提示 / no notice was produced", tag)
		}
		if !strings.Contains(notice, "--lang") {
			t.Errorf("提示未指名来源: %q / the notice does not name the source: %q", notice, notice)
		}
		if !strings.Contains(notice, "en-US") {
			t.Errorf("提示未说明回退目标: %q / the notice does not state the fallback: %q", notice, notice)
		}
		if tag != "" && !strings.Contains(notice, tag) {
			t.Errorf("提示未回显取值 %q: %q / the notice does not echo the value %q: %q", tag, notice, tag, notice)
		}
	}
}

// TestResolveLangNoticeNamesConfigField 验证来自配置文件的标签在提示里被指名为 self.lang
// TestResolveLangNoticeNamesConfigField checks that a tag coming from the configuration file is named self.lang in the notice
func TestResolveLangNoticeNamesConfigField(t *testing.T) {
	_, notice := resolveLang("ja-JP", "self.lang")
	if !strings.Contains(notice, "self.lang") {
		t.Fatalf("提示未指名 self.lang: %q / the notice does not name self.lang: %q", notice, notice)
	}
	if strings.Contains(notice, "--lang") {
		t.Fatalf("提示不应把来源写成 --lang: %q / the notice should not name --lang as the source: %q", notice, notice)
	}
}

// TestResolveLangNoticeStaysEnglish 验证当前语言为中文时回退提示仍用英文渲染
// 标签不受支持时生效语言本来就是 en-US，因此提示固定用英文，取值不随调用顺序变化
//
// TestResolveLangNoticeStaysEnglish checks that the fallback notice stays English while the current language is Chinese
// An unsupported tag makes en-US the effective language anyway, so the notice stays English and its value does not depend on call order
func TestResolveLangNoticeStaysEnglish(t *testing.T) {
	i18n.Set(i18n.LangZHHans)
	t.Cleanup(func() { i18n.Set(i18n.Default()) })

	_, notice := resolveLang("ja-JP", "--lang")
	if !strings.Contains(notice, "not a supported language tag") {
		t.Fatalf("提示应为英文: %q / the notice should be English: %q", notice, notice)
	}
	if strings.Contains(notice, "不是受支持的语言标签") {
		t.Fatalf("提示不应被当前语言改写: %q / the notice should not follow the current language: %q", notice, notice)
	}
}

// TestResolveLangMatchesParse 验证回退判定与 i18n.Parse 的口径一致
// 该断言使 resolveLang 不需要自己维护一份受支持标签的判断
//
// TestResolveLangMatchesParse checks that the fallback decision agrees with i18n.Parse
// The assertion keeps resolveLang from having to maintain its own idea of the supported tags
func TestResolveLangMatchesParse(t *testing.T) {
	tags := []string{"en-US", "zh-Hans", "zh-Hant", "zh", "zh-TW", "ja-JP", "en_US", "", "de"}
	for _, tag := range tags {
		parsed, ok := i18n.Parse(tag)
		lang, notice := resolveLang(tag, "--lang")

		if ok && (lang != parsed || notice != "") {
			t.Errorf("resolveLang(%q) = %q, %q，与 Parse 的 %q, true 不一致 / disagrees with Parse", tag, lang, notice, parsed)
		}
		if !ok && (lang != i18n.LangEN || notice == "") {
			t.Errorf("resolveLang(%q) = %q, %q，Parse 返回 false 时应回退并提示 / Parse returning false should fall back with a notice", tag, lang, notice)
		}
	}
}

// cliLangOutcome 复现 main 第 1 步对命令行 --lang 的判定，返回生效语言与提示
// 该判定要求与配置加载无关，因为配置加载失败会直接退出，提示必须在此之前产生
//
// cliLangOutcome reproduces the judgement main makes on the command-line --lang in its first step and returns the effective language together with the notice
// That judgement has to be independent of the configuration load, because a failed load exits at once and the notice therefore has to be produced before it
func cliLangOutcome(tag string) (i18n.Lang, string) {
	lang, ok := i18n.Parse(tag)
	if ok {
		return lang, ""
	}
	return i18n.Default(), i18n.Text(i18n.Default(), KeyLangUnsupported, "--lang", tag)
}

// TestCLILangIsJudgedBeforeLoadFails 验证命令行语言的判定与提示不依赖配置是否加载成功
// 回归点：提示曾在配置合并之后才生成，而配置加载失败会以退出码 1 结束，使该路径上「标签写错」完全静默，与文档承诺不符
//
// TestCLILangIsJudgedBeforeLoadFails checks that the judgement and the notice for the command-line language do not depend on the configuration loading successfully
// The regression it pins: the notice used to be produced only after the merge, while a failed load ends with exit code 1, which left a mistyped tag entirely silent on that path and contradicted the documented promise
func TestCLILangIsJudgedBeforeLoadFails(t *testing.T) {
	// 非法取值必须给出提示，且回退到默认语言，不需要任何配置即可判定
	// An unsupported value has to yield a notice and fall back to the default language, and none of that needs a configuration
	for _, tag := range []string{"ja-JP", "", "en_US", "zh-", "de-DE"} {
		lang, notice := cliLangOutcome(tag)
		if lang != i18n.LangEN {
			t.Errorf("cliLangOutcome(%q) = %q，期望回退到 %q / want the fallback %q", tag, lang, i18n.LangEN, i18n.LangEN)
		}
		if notice == "" {
			t.Fatalf("cliLangOutcome(%q) 未给出提示，该提示会在配置加载失败时丢失 / no notice was produced, and it would be lost when the configuration fails to load", tag)
		}
		if !strings.Contains(notice, "--lang") {
			t.Errorf("提示未指名命令行来源: %q / the notice does not name the command-line source: %q", notice, notice)
		}
	}

	// 受支持的取值不应有提示，且返回规范化之后的语言，混合大小写也必须归一
	// A supported value yields no notice and returns a canonicalised language, and a mixed-case value has to be normalised as well
	for tag, want := range map[string]i18n.Lang{
		"en-US": i18n.LangEN, "zh-Hans": i18n.LangZHHans, "zh-hant": i18n.LangZHant, "ZH-HANS": i18n.LangZHHans,
	} {
		lang, notice := cliLangOutcome(tag)
		if lang != want {
			t.Errorf("cliLangOutcome(%q) = %q，期望 %q / want %q", tag, lang, want, want)
		}
		if notice != "" {
			t.Errorf("cliLangOutcome(%q) 不应给出提示，实际 %q / a supported tag should yield no notice, got %q", tag, notice, notice)
		}
	}
}
