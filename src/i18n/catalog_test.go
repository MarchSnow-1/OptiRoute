// 本文件测试消息目录的登记与取用
// 覆盖三种语言的取用、参数格式化、无参数时保留百分号、键缺失的标记、缺某语言文案时退回英文、重复键 panic、键列表有序与并发读写
//
// This file tests registration and lookup in the message catalogue
// It covers lookup in the three languages, argument formatting, percent signs kept when no argument is passed, the marker for a missing key, the fallback to English when one language has no text, the panic on a duplicate key, the ordering of the key list and concurrent access
package i18n

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TestTextReturnsEachLanguage 验证同一键在三种语言下返回各自的文案
// TestTextReturnsEachLanguage checks that one key returns its own text in each of the three languages
func TestTextReturnsEachLanguage(t *testing.T) {
	key := Key("catalog.test.three_languages")
	Register(map[Key]Entry{
		key: New("hello", "你好", "你好"),
	})

	cases := map[Lang]string{
		LangEN:     "hello",
		LangZHHans: "你好",
		LangZHant:  "你好",
	}
	for l, want := range cases {
		if got := Text(l, key); got != want {
			t.Errorf("Text(%q) = %q，期望 %q / want %q", l, got, want, want)
		}
	}
}

// TestTextFormatsArguments 验证参数按格式符填充
// TestTextFormatsArguments checks that arguments fill the format verbs
func TestTextFormatsArguments(t *testing.T) {
	key := Key("catalog.test.arguments")
	Register(map[Key]Entry{
		key: New("connect %s failed: %v", "连接 %s 失败: %v", "連線 %s 失敗: %v"),
	})

	if got := Text(LangEN, key, "example.com:80", "refused"); got != "connect example.com:80 failed: refused" {
		t.Errorf("Text(en-US) = %q / want %q", got, "connect example.com:80 failed: refused")
	}
	if got := Text(LangZHHans, key, "example.com:80", "refused"); got != "连接 example.com:80 失败: refused" {
		t.Errorf("Text(zh-Hans) = %q / want %q", got, "连接 example.com:80 失败: refused")
	}
}

// TestTextWithoutArgumentsKeepsPercentSign 验证无参数调用不会把文案里的百分号当成格式符
// TestTextWithoutArgumentsKeepsPercentSign checks that a call without arguments cannot read a percent sign in the text as a format verb
func TestTextWithoutArgumentsKeepsPercentSign(t *testing.T) {
	key := Key("catalog.test.percent")
	Register(map[Key]Entry{
		key: New("usage at 100% load", "负载 100% 时", "負載 100% 時"),
	})

	if got := Text(LangEN, key); got != "usage at 100% load" {
		t.Errorf("Text(en-US) = %q，期望百分号原样保留 / want the percent sign kept as it stands", got)
	}
}

// TestTextMarksMissingKey 验证未登记的键返回可见标记而不是空串
// TestTextMarksMissingKey checks that an unregistered key returns a visible marker rather than an empty string
func TestTextMarksMissingKey(t *testing.T) {
	unknown := Key("catalog.test.not_registered")
	if _, ok := Lookup(unknown); ok {
		t.Fatalf("键 %q 不应已登记 / key %q should not be registered", unknown, unknown)
	}

	want := "!catalog.test.not_registered!"
	if got := Text(LangEN, unknown); got != want {
		t.Fatalf("Text() = %q，期望 %q / want %q", got, want, want)
	}
}

// TestTextFallsBackToEnglishForEmptyLanguage 验证某语言缺文案时退回英文
// TestTextFallsBackToEnglishForEmptyLanguage checks that a language without text falls back to English
func TestTextFallsBackToEnglishForEmptyLanguage(t *testing.T) {
	key := Key("catalog.test.partial")
	Register(map[Key]Entry{
		key: New("english only", "", ""),
	})

	for _, l := range []Lang{LangZHHans, LangZHant} {
		if got := Text(l, key); got != "english only" {
			t.Errorf("Text(%q) = %q，期望退回英文 / want the English fallback", l, got)
		}
	}
}

// TestTFollowsCurrentLanguage 验证 T 使用当前语言
// TestTFollowsCurrentLanguage checks that T uses the current language
func TestTFollowsCurrentLanguage(t *testing.T) {
	resetCurrent(t)

	key := Key("catalog.test.current")
	Register(map[Key]Entry{
		key: New("started", "已启动", "已啟動"),
	})

	Set(LangEN)
	if got := T(key); got != "started" {
		t.Fatalf("T() en-US = %q，期望 %q / want %q", got, "started", "started")
	}
	Set(LangZHHans)
	if got := T(key); got != "已启动" {
		t.Fatalf("T() zh-Hans = %q，期望 %q / want %q", got, "已启动", "已启动")
	}
	Set(LangZHant)
	if got := T(key); got != "已啟動" {
		t.Fatalf("T() zh-Hant = %q，期望 %q / want %q", got, "已啟動", "已啟動")
	}
}

// TestRegisterDuplicateKeyPanics 验证同一个键登记两次会 panic
// TestRegisterDuplicateKeyPanics checks that registering the same key twice panics
func TestRegisterDuplicateKeyPanics(t *testing.T) {
	key := Key("catalog.test.duplicate")
	Register(map[Key]Entry{key: New("first", "第一", "第一")})

	defer func() {
		if recover() == nil {
			t.Fatal("重复登记同一个键时应 panic / registering the same key twice should panic")
		}
	}()
	Register(map[Key]Entry{key: New("second", "第二", "第二")})
}

// TestKeysAreSortedAndComplete 验证键列表按字典序排列且包含已登记的键
// TestKeysAreSortedAndComplete checks that the key list is in ascending order and holds the registered keys
func TestKeysAreSortedAndComplete(t *testing.T) {
	key := Key("catalog.test.sorted")
	Register(map[Key]Entry{key: New("a", "甲", "甲")})

	keys := Keys()
	if len(keys) == 0 {
		t.Fatal("Keys() 返回空列表 / Keys() returned an empty list")
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			t.Fatalf("Keys() 未按字典序排列: %q 在 %q 之前 / Keys() is not in ascending order: %q before %q", keys[i-1], keys[i], keys[i-1], keys[i])
		}
	}

	found := false
	for _, k := range keys {
		if k == key {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Keys() 未包含 %q / Keys() does not hold %q", key, key)
	}
}

// TestEntryAllReturnsEveryLanguage 验证 All 返回三种语言的文案
// TestEntryAllReturnsEveryLanguage checks that All returns the text in the three languages
func TestEntryAllReturnsEveryLanguage(t *testing.T) {
	all := New("hello", "你好", "你好").All()
	if len(all) != len(Supported()) {
		t.Fatalf("All() 返回 %d 项，期望 %d 项 / All() returned %d entries, want %d", len(all), len(Supported()), len(all), len(Supported()))
	}
	for _, l := range Supported() {
		if _, ok := all[l]; !ok {
			t.Errorf("All() 缺少语言 %q / All() is missing language %q", l, l)
		}
	}
}

// TestConcurrentTextAndRegisterIsSafe 验证并发取用与登记不会破坏目录
// TestConcurrentTextAndRegisterIsSafe checks that concurrent lookup and registration cannot corrupt the catalogue
func TestConcurrentTextAndRegisterIsSafe(t *testing.T) {
	shared := Key("catalog.test.concurrent")
	Register(map[Key]Entry{shared: New("body %d", "正文 %d", "正文 %d")})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := Key(fmt.Sprintf("catalog.test.concurrent.%d", n))
			Register(map[Key]Entry{key: New("x", "甲", "甲")})
			if got := Text(LangEN, shared, n); !strings.HasPrefix(got, "body ") {
				t.Errorf("并发取用返回 %q / concurrent lookup returned %q", got, got)
			}
		}(i)
	}
	wg.Wait()
}
