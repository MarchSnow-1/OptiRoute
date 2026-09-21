// 本包负责把日志与启动期文案按语言渲染：解析语言标签、持有消息目录、按当前语言取文案
// 语言标签按 IETF BCP 47 的 language[-Script][-REGION] 形式解析，匹配大小写不敏感，返回值用规范大小写
// 本版本支持 en-US、zh-Hans 与 zh-Hant 三种取值，其余标签由调用方回退到 en-US 并给出提示
// 单字符子标签及其之后的扩展内容在解析时被丢弃，变体子标签不在识别范围内，因此带变体的标签同样走回退
// 消息目录由各包在自己的 messages.go 中登记，本包只做查找与格式化，不感知任何业务语义
//
// This package renders logs and start-up text in a language: it parses language tags, holds the message catalogue and looks text up for the current language
// A language tag is parsed as the language[-Script][-REGION] form of IETF BCP 47, matching is case-insensitive and a result uses canonical case
// This version supports the three values en-US, zh-Hans and zh-Hant, and every other tag is left to the caller to fall back to en-US with a notice
// A single-character subtag and the extension content after it are discarded during parsing, and variant subtags are outside what this version recognises, so a tag carrying a variant takes the fallback path as well
// The message catalogue is registered by each package in its own messages.go, and this package only looks entries up and formats them without knowing any business meaning
package i18n

import (
	"strings"
	"sync"
)

// Lang 是受支持的语言，取值为规范形式的语言标签
// Lang is a supported language, held as its canonical language tag
type Lang string

const (
	// LangEN 是英文，也是未指定语言时的取值
	// LangEN is English and is also the value used when no language is given
	LangEN Lang = "en-US"

	// LangZHHans 是简体中文
	// LangZHHans is Simplified Chinese
	LangZHHans Lang = "zh-Hans"

	// LangZHant 是繁体中文
	// LangZHant is Traditional Chinese
	LangZHant Lang = "zh-Hant"
)

// supported 列出受支持的语言，排序与 Supported 的返回顺序一致
// supported lists the supported languages in the order Supported returns them
var supported = []Lang{LangEN, LangZHHans, LangZHant}

// Supported 返回受支持语言的副本，调用方改动返回值不会影响本包
// Supported returns a copy of the supported languages, so a caller editing the result cannot affect this package
func Supported() []Lang {
	out := make([]Lang, len(supported))
	copy(out, supported)
	return out
}

// Default 返回未指定语言时使用的语言，即 en-US
// Default returns the language used when none is given, which is en-US
func Default() Lang { return LangEN }

// String 返回该语言的标准标签
// String returns the canonical tag of the language
func (l Lang) String() string { return string(l) }

// IsSupported 判断该语言是否受支持
// IsSupported reports whether the language is supported
func IsSupported(l Lang) bool {
	for _, s := range supported {
		if s == l {
			return true
		}
	}
	return false
}

var (
	// curMu 保护 current，使 Set 与 Current 可以跨 goroutine 调用
	// curMu guards current so that Set and Current are safe to call across goroutines
	curMu   sync.RWMutex
	current = Default()
)

// Current 返回当前语言
// Current returns the current language
func Current() Lang {
	curMu.RLock()
	defer curMu.RUnlock()
	return current
}

// Set 设置当前语言，取值不受支持时回落到 Default
// Set changes the current language and falls back to Default when the value is not supported
func Set(l Lang) {
	if !IsSupported(l) {
		l = Default()
	}
	curMu.Lock()
	current = l
	curMu.Unlock()
}

// Parse 把语言标签解析为受支持的语言
// 解析成功时返回该语言与 true，标签不合法或所指语言不在本版本支持范围内时返回 LangEN 与 false
// 成功返回的语言已归一为规范大小写，例如 zh-hans 与 ZH-HANS 都返回 zh-Hans
//
// Parse resolves a language tag into a supported language
// It returns the language and true on success, and returns LangEN with false when the tag is malformed or names a language outside what this version supports
// A successful result is normalised to canonical case, so zh-hans and ZH-HANS both resolve to zh-Hans
func Parse(tag string) (Lang, bool) {
	lang, script, region, ok := splitTag(tag)
	if !ok {
		return LangEN, false
	}

	switch lang {
	case "en":
		// 英文只有一套文案，区域与脚本子标签都指向同一条目
		// English carries a single set of text, so region and script subtags point at the same entries
		return LangEN, true

	case "zh":
		if script != "" {
			switch script {
			case "Hans":
				return LangZHHans, true
			case "Hant":
				return LangZHant, true
			default:
				// 汉字以外的书写系统没有对应文案，交给调用方回退
				// A writing system other than Han has no matching text, so the caller takes the fallback path
				return LangEN, false
			}
		}
		switch region {
		case "TW", "HK", "MO":
			return LangZHant, true
		case "CN", "SG", "MY":
			return LangZHHans, true
		}
		// 裸 zh 与上表未列出的区域按缺省脚本处理，即简体
		// A bare zh and a region outside the table above follow the default script, which is Simplified
		return LangZHHans, true

	default:
		return LangEN, false
	}
}

// splitTag 按 language[-Script][-REGION] 形式拆解标签
// 返回小写的语言、规范大小写的脚本与区域，以及标签是否合法
// 单字符子标签视为扩展的开始，该子标签与其后的全部子标签都不参与语言判定
// 扩展尾部只校验每个子标签的字符范围，不校验其内部语法，且要求单字符子标签之后至少还有一个子标签
// 变体子标签不属于上述形式，按不合法返回，因此调用方会走回退路径
//
// splitTag splits a tag in the language[-Script][-REGION] form
// It returns the language in lower case, the script and region in canonical case, and whether the tag is well formed
// A single-character subtag marks the start of an extension, and neither it nor the subtags after it take part in resolving the language
// The extension tail only has the character range of each subtag checked rather than its internal grammar, and a single-character subtag must be followed by at least one more subtag
// A variant subtag falls outside that form and is reported as malformed, which sends the caller down the fallback path
func splitTag(tag string) (lang, script, region string, ok bool) {
	subs := strings.Split(tag, "-")
	for i, sub := range subs {
		if !isAlnum(sub) {
			return "", "", "", false
		}
		if len(sub) == 1 {
			// 扩展开始，其后至少要有一个子标签，本版本不再解析尾部内容
			// An extension starts here, at least one subtag has to follow it, and this version parses no further into the tail
			if i+1 == len(subs) {
				return "", "", "", false
			}
			for _, tail := range subs[i+1:] {
				if !isAlnum(tail) {
					return "", "", "", false
				}
			}
			break
		}
		switch {
		case i == 0:
			if len(sub) < 2 || len(sub) > 3 || !isAlpha(sub) {
				return "", "", "", false
			}
			lang = strings.ToLower(sub)
		case len(sub) == 4 && isAlpha(sub) && script == "" && region == "":
			script = titleCase(sub)
		case (len(sub) == 2 && isAlpha(sub) || len(sub) == 3 && isDigits(sub)) && region == "":
			region = strings.ToUpper(sub)
		default:
			return "", "", "", false
		}
	}
	if lang == "" {
		return "", "", "", false
	}
	return lang, script, region, true
}

// isAlpha 判断 s 非空且全部由 ASCII 字母组成
// isAlpha reports whether s is non-empty and made of ASCII letters only
func isAlpha(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; !isAlphaByte(c) {
			return false
		}
	}
	return true
}

// isDigits 判断 s 非空且全部由 ASCII 数字组成
// isDigits reports whether s is non-empty and made of ASCII digits only
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isAlnum 判断 s 非空且全部由 ASCII 字母与数字组成，长度不超过 8 位
// isAlnum reports whether s is non-empty, made of ASCII letters and digits, and no longer than eight characters
func isAlnum(s string) bool {
	if s == "" || len(s) > 8 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !isAlphaByte(c) && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// isAlphaByte 判断 c 是否为 ASCII 字母
// isAlphaByte reports whether c is an ASCII letter
func isAlphaByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// titleCase 把 s 的首字符转成大写、其余转成小写，用于脚本子标签的规范形式
// titleCase upper-cases the first character of s and lower-cases the rest, producing the canonical form of a script subtag
func titleCase(s string) string {
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}
