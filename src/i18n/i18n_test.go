// 本文件测试语言标签的解析与当前语言的设置
// 覆盖规范取值、大小写不敏感、缺省脚本与区域推导、脚本优先于区域、扩展子标签被丢弃、非法与未支持标签的回退
//
// This file tests language tag parsing and the current-language setting
// It covers canonical values, case-insensitivity, script and region defaults, script taking precedence over region, discarded extension subtags, and the fallback for malformed and unsupported tags
package i18n

import "testing"

// TestDefaultIsEnglish 验证未指定语言时的取值为 en-US
// TestDefaultIsEnglish checks that the value used when no language is given is en-US
func TestDefaultIsEnglish(t *testing.T) {
	if got := Default(); got != LangEN {
		t.Fatalf("Default() = %q, 期望 %q / want %q", got, LangEN, LangEN)
	}
	if Default().String() != "en-US" {
		t.Fatalf("Default().String() = %q, 期望 en-US / want en-US", Default().String())
	}
}

// TestParseCanonicalTags 验证三种受支持标签的原样解析
// TestParseCanonicalTags checks that the three supported tags resolve as they stand
func TestParseCanonicalTags(t *testing.T) {
	cases := map[string]Lang{
		"en-US":   LangEN,
		"zh-Hans": LangZHHans,
		"zh-Hant": LangZHant,
	}
	for tag, want := range cases {
		got, ok := Parse(tag)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %q, %v，期望 %q, true / want %q, true", tag, got, ok, want, want)
		}
	}
}

// TestParseIsCaseInsensitive 验证大小写混合的标签被归一为规范形式
// TestParseIsCaseInsensitive checks that a tag in mixed case is normalised to the canonical form
func TestParseIsCaseInsensitive(t *testing.T) {
	cases := map[string]Lang{
		"EN-us":   LangEN,
		"EN-US":   LangEN,
		"ZH-HANS": LangZHHans,
		"zh-hans": LangZHHans,
		"Zh-HaNs": LangZHHans,
		"zh-hant": LangZHant,
		"ZH-HANT": LangZHant,
	}
	for tag, want := range cases {
		got, ok := Parse(tag)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %q, %v，期望 %q, true / want %q, true", tag, got, ok, want, want)
		}
	}
}

// TestParseResolvesEnglishRegionAndScript 验证各种英文变体都指向同一套英文文案
// TestParseResolvesEnglishRegionAndScript checks that the English variants all point at the single set of English text
func TestParseResolvesEnglishRegionAndScript(t *testing.T) {
	for _, tag := range []string{"en", "en-GB", "en-AU", "en-Latn-US", "en-US-u-ca-gregory"} {
		got, ok := Parse(tag)
		if !ok || got != LangEN {
			t.Errorf("Parse(%q) = %q, %v，期望 %q, true / want %q, true", tag, got, ok, LangEN, LangEN)
		}
	}
}

// TestParseDerivesChineseScriptFromRegion 验证区域推导出的中文书写系统
// TestParseDerivesChineseScriptFromRegion checks the Chinese writing system derived from a region
func TestParseDerivesChineseScriptFromRegion(t *testing.T) {
	hans := []string{"zh", "zh-CN", "zh-SG", "zh-MY", "ZH-cn"}
	hant := []string{"zh-TW", "zh-HK", "zh-MO", "ZH-tw"}

	for _, tag := range hans {
		got, ok := Parse(tag)
		if !ok || got != LangZHHans {
			t.Errorf("Parse(%q) = %q, %v，期望 %q, true / want %q, true", tag, got, ok, LangZHHans, LangZHHans)
		}
	}
	for _, tag := range hant {
		got, ok := Parse(tag)
		if !ok || got != LangZHant {
			t.Errorf("Parse(%q) = %q, %v，期望 %q, true / want %q, true", tag, got, ok, LangZHant, LangZHant)
		}
	}
}

// TestParsePrefersScriptOverRegion 验证脚本子标签优先于区域子标签
// TestParsePrefersScriptOverRegion checks that a script subtag takes precedence over a region subtag
func TestParsePrefersScriptOverRegion(t *testing.T) {
	cases := map[string]Lang{
		"zh-Hant-CN": LangZHant,
		"zh-Hans-TW": LangZHHans,
		"zh-Hant-HK": LangZHant,
		"zh-Hans-HK": LangZHHans,
	}
	for tag, want := range cases {
		got, ok := Parse(tag)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %q, %v，期望 %q, true / want %q, true", tag, got, ok, want, want)
		}
	}
}

// TestParseIgnoresExtensionSubtags 验证扩展子标签被丢弃而不影响结果
// TestParseIgnoresExtensionSubtags checks that extension subtags are discarded without changing the result
func TestParseIgnoresExtensionSubtags(t *testing.T) {
	cases := map[string]Lang{
		"zh-Hans-CN-u-nu-hanidec": LangZHHans,
		"zh-TW-x-private":         LangZHant,
		"zh-Hant-u-co-stroke":     LangZHant,
	}
	for tag, want := range cases {
		got, ok := Parse(tag)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %q, %v，期望 %q, true / want %q, true", tag, got, ok, want, want)
		}
	}
}

// TestParseRejectsMalformedTags 验证格式不合法的标签返回 false，由调用方回退
// TestParseRejectsMalformedTags checks that a malformed tag returns false and leaves the fallback to the caller
func TestParseRejectsMalformedTags(t *testing.T) {
	malformed := []string{
		"", "zh-", "-zh", "zh--Hans", "en_US", "en US", "e", "engl",
		"123", "zh-Han", "zh-12345", "zh-CN-Hant", "zh-Hans-Hant",
		"en-US-u", "zh-台湾", "zh-Hans-", "e1-US", "en-USA",
	}
	for _, tag := range malformed {
		got, ok := Parse(tag)
		if ok {
			t.Errorf("Parse(%q) = %q, true，期望回退 / want the fallback path", tag, got)
		}
		if got != LangEN {
			t.Errorf("Parse(%q) 回退值 = %q，期望 %q / fallback value %q, want %q", tag, got, LangEN, got, LangEN)
		}
	}
}

// TestParseRejectsUnsupportedLanguages 验证合法但本版本未支持的标签返回 false
// TestParseRejectsUnsupportedLanguages checks that a well-formed tag this version does not support returns false
func TestParseRejectsUnsupportedLanguages(t *testing.T) {
	unsupported := []string{
		"ja-JP", "de", "fr-FR", "ru-RU", "zh-Latn", "zh-hakka", "de-DE-1996",
	}
	for _, tag := range unsupported {
		got, ok := Parse(tag)
		if ok {
			t.Errorf("Parse(%q) = %q, true，期望未被识别 / want it unrecognised", tag, got)
		}
	}
}

// TestSetAndCurrent 验证当前语言的设置与读取
// TestSetAndCurrent checks setting and reading the current language
func TestSetAndCurrent(t *testing.T) {
	resetCurrent(t)

	for _, l := range Supported() {
		Set(l)
		if got := Current(); got != l {
			t.Fatalf("Set(%q) 之后 Current() = %q / after Set(%q) Current() = %q", l, got, l, got)
		}
	}
}

// TestSetFallsBackForUnsupportedValue 验证不受支持的取值不会成为当前语言
// TestSetFallsBackForUnsupportedValue checks that an unsupported value cannot become the current language
func TestSetFallsBackForUnsupportedValue(t *testing.T) {
	resetCurrent(t)

	Set(Lang("ja-JP"))
	if got := Current(); got != LangEN {
		t.Fatalf("Set(ja-JP) 之后 Current() = %q，期望回落到 %q / after Set(ja-JP) Current() = %q, want the fallback %q", got, LangEN, got, LangEN)
	}
}

// TestSupportedAndIsSupported 验证受支持语言的列表与判断
// TestSupportedAndIsSupported checks the supported-language list and the membership test
func TestSupportedAndIsSupported(t *testing.T) {
	list := Supported()
	if len(list) != 3 {
		t.Fatalf("Supported() 长度 = %d，期望 3 / Supported() length %d, want 3", len(list), len(list))
	}
	if list[0] != LangEN {
		t.Errorf("Supported() 首项 = %q，期望默认语言在最前 / Supported() starts with %q, want the default first", list[0], list[0])
	}
	for _, l := range list {
		if !IsSupported(l) {
			t.Errorf("IsSupported(%q) = false / want true", l)
		}
	}
	if IsSupported(Lang("ja-JP")) {
		t.Error("IsSupported(ja-JP) = true / want false")
	}
}

// TestSupportedReturnsCopy 验证调用方改动返回的切片不会影响本包的列表
// TestSupportedReturnsCopy checks that a caller editing the returned slice cannot affect the list in this package
func TestSupportedReturnsCopy(t *testing.T) {
	list := Supported()
	list[0] = Lang("ja-JP")

	if got := Supported()[0]; got != LangEN {
		t.Fatalf("改动返回值之后 Supported()[0] = %q，期望 %q / after editing the result Supported()[0] = %q, want %q", got, LangEN, got, LangEN)
	}
}
