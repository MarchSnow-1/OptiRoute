// 本文件测试消息目录的加载与取用
// 覆盖三种语言的取用、参数格式化、无参数时保留百分号、键缺失的标记、缺某语言文案时退回默认语言、键列表有序、并发取用
// 另覆盖加载器对损坏语言文件的拒绝：重复键、空文案、顶层不是对象、条目不是对象，这四种都在加载期报出而不是静默取后者
//
// This file tests loading and lookup in the message catalogue
// It covers lookup in the three languages, argument formatting, percent signs kept when no argument is passed, the marker for a missing key, the fallback to the default language when one language has no text, the ordering of the key list and concurrent lookup
// It also covers the loader rejecting a broken language file: a duplicate key, empty copy, a top level that is not an object and an entry that is not an object, all of which are reported while loading rather than silently taking the last value
package i18n

import (
	"strings"
	"sync"
	"testing"
)

// installEntry 把一条测试条目放进目录，并在用例结束后移除
// 直接操作包级目录而不经过加载，是为了让取用路径的用例不依赖语言文件的内容
//
// installEntry puts one test entry into the catalogue and removes it when the case ends
// It touches the package-level catalogue instead of going through loading, which keeps the lookup cases independent of what the language files happen to hold
func installEntry(t *testing.T, key Key, texts map[Lang]string, note string) {
	t.Helper()

	catMu.Lock()
	catalog[key] = Entry{texts: texts, note: note}
	catMu.Unlock()
	t.Cleanup(func() {
		catMu.Lock()
		delete(catalog, key)
		catMu.Unlock()
	})
}

// resetCurrent 把当前语言还原为默认值，供各用例在结束时清理全局状态
// resetCurrent restores the current language to its default so that each case cleans up the shared state
func resetCurrent(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { Set(Default()) })
}

// TestTextReturnsEachLanguage 验证同一键在三种语言下返回各自的文案
// TestTextReturnsEachLanguage checks that one key returns its own text in each of the three languages
func TestTextReturnsEachLanguage(t *testing.T) {
	key := Key("catalog.test.three_languages")
	installEntry(t, key, map[Lang]string{
		LangEN:     "hello",
		LangZHHans: "你好",
		LangZHant:  "妳好",
	}, "note")

	cases := map[Lang]string{
		LangEN:     "hello",
		LangZHHans: "你好",
		LangZHant:  "妳好",
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
	installEntry(t, key, map[Lang]string{
		LangEN:     "connect %s failed: %v",
		LangZHHans: "连接 %s 失败: %v",
		LangZHant:  "連線 %s 失敗: %v",
	}, "note")

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
	installEntry(t, key, map[Lang]string{
		LangEN:     "usage at 100% load",
		LangZHHans: "负载 100% 时",
		LangZHant:  "負載 100% 時",
	}, "note")

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

// TestTextFallsBackToDefaultLanguage 验证某语言缺文案时退回默认语言
// TestTextFallsBackToDefaultLanguage checks that a language without text falls back to the default one
func TestTextFallsBackToDefaultLanguage(t *testing.T) {
	key := Key("catalog.test.partial")
	installEntry(t, key, map[Lang]string{LangEN: "english only"}, "note")

	for _, l := range []Lang{LangZHHans, LangZHant} {
		if got := Text(l, key); got != "english only" {
			t.Errorf("Text(%q) = %q，期望退回默认语言 / want the default-language fallback", l, got)
		}
	}
}

// TestTFollowsCurrentLanguage 验证 T 使用当前语言
// TestTFollowsCurrentLanguage checks that T uses the current language
func TestTFollowsCurrentLanguage(t *testing.T) {
	resetCurrent(t)

	key := Key("catalog.test.current")
	installEntry(t, key, map[Lang]string{
		LangEN:     "started",
		LangZHHans: "已启动",
		LangZHant:  "已啟動",
	}, "note")

	Set(LangEN)
	if got := T(key); got != "started" {
		t.Fatalf("T() en-US = %q / want %q", got, "started")
	}
	Set(LangZHHans)
	if got := T(key); got != "已启动" {
		t.Fatalf("T() zh-Hans = %q / want %q", got, "已启动")
	}
	Set(LangZHant)
	if got := T(key); got != "已啟動" {
		t.Fatalf("T() zh-Hant = %q / want %q", got, "已啟動")
	}
}

// TestEntryAllAndNote 验证 All 返回各语言文案、Note 返回说明
// TestEntryAllAndNote checks that All returns the text per language and Note returns the note
func TestEntryAllAndNote(t *testing.T) {
	key := Key("catalog.test.entry")
	installEntry(t, key, map[Lang]string{
		LangEN:     "en",
		LangZHHans: "简",
		LangZHant:  "繁",
	}, "触发条件与参数说明")

	entry, ok := Lookup(key)
	if !ok {
		t.Fatal("条目未登记 / the entry is not registered")
	}
	all := entry.All()
	if len(all) != 3 {
		t.Fatalf("All() 返回 %d 项，期望 3 项 / All() returned %d entries, want 3", len(all), len(all))
	}
	for _, l := range []Lang{LangEN, LangZHHans, LangZHant} {
		if _, ok := all[l]; !ok {
			t.Errorf("All() 缺少语言 %q / All() is missing language %q", l, l)
		}
	}
	if entry.Note() != "触发条件与参数说明" {
		t.Errorf("Note() = %q / want the installed note", entry.Note())
	}

	// 调用方改动 All 的返回值不得影响目录
	// A caller editing the result of All must not affect the catalogue
	all[LangEN] = "changed"
	if again, _ := Lookup(key); again.text(LangEN) != "en" {
		t.Error("改动 All() 的返回值污染了目录 / editing the result of All polluted the catalogue")
	}
}

// TestKeysAreSorted 验证键列表按字典序排列且包含已安装的键
// TestKeysAreSorted checks that the key list is in ascending order and holds the installed key
func TestKeysAreSorted(t *testing.T) {
	key := Key("catalog.test.sorted")
	installEntry(t, key, map[Lang]string{LangEN: "a"}, "note")

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

// TestConcurrentLookupIsSafe 验证并发取用与安装不会破坏目录
// TestConcurrentLookupIsSafe checks that concurrent lookup and installation cannot corrupt the catalogue
func TestConcurrentLookupIsSafe(t *testing.T) {
	shared := Key("catalog.test.concurrent")
	installEntry(t, shared, map[Lang]string{LangEN: "body %d"}, "note")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := Key("catalog.test.concurrent." + string(rune('a'+n)))
			catMu.Lock()
			catalog[key] = Entry{texts: map[Lang]string{LangEN: "x"}}
			catMu.Unlock()
			defer func() {
				catMu.Lock()
				delete(catalog, key)
				catMu.Unlock()
			}()
			if got := Text(LangEN, shared, n); !strings.HasPrefix(got, "body ") {
				t.Errorf("并发取用返回 %q / concurrent lookup returned %q", got, got)
			}
		}(i)
	}
	wg.Wait()
}

// TestDecodeLocaleRejectsBrokenInput 验证加载器拒绝四种损坏的语言文件
// 重复键这一项尤其重要：encoding/json 对重复键静默取后者，若只做一次 Unmarshal，一个键的文案会被另一条覆盖而毫无提示
//
// TestDecodeLocaleRejectsBrokenInput checks that the loader rejects four kinds of broken language file
// The duplicate-key case matters most: encoding/json silently keeps the last duplicate, so a single Unmarshal would let one entry overwrite another with no signal at all
func TestDecodeLocaleRejectsBrokenInput(t *testing.T) {
	cases := map[string]string{
		"重复键":    `{"util.a":{"text":"x","note":"n"},"util.a":{"text":"y","note":"n"}}`,
		"空文案":    `{"util.a":{"text":"  ","note":"n"}}`,
		"顶层不是对象": `[{"util.a":{"text":"x","note":"n"}}]`,
		"条目不是对象": `{"util.a":"just a string"}`,
	}

	for name, body := range cases {
		if _, err := decodeLocale(LangEN, []byte(body)); err == nil {
			t.Errorf("%s 的输入应被拒绝 / the %s input should have been rejected", name, name)
		}
	}
}

// TestDecodeLocaleAcceptsAValidFile 验证合法语言文件被正确解析
// TestDecodeLocaleAcceptsAValidFile checks that a valid language file is parsed correctly
func TestDecodeLocaleAcceptsAValidFile(t *testing.T) {
	entries, err := decodeLocale(LangZHHans, []byte(`{"util.a":{"text":"甲 %d","note":"说明"}}`))
	if err != nil {
		t.Fatalf("合法输入被拒绝 / a valid input was rejected: %v", err)
	}
	e, ok := entries[Key("util.a")]
	if !ok {
		t.Fatal("条目缺失 / the entry is missing")
	}
	if e.text(LangZHHans) != "甲 %d" {
		t.Errorf("文案 = %q / want %q", e.text(LangZHHans), "甲 %d")
	}
	if e.Note() != "说明" {
		t.Errorf("说明 = %q / want %q", e.Note(), "说明")
	}
}

// TestLoadKeepsTheCatalogueOnFailure 验证加载失败时目录保持原样
// 加载失败发生在二进制启动期，若它先把目录清空再报错，调用方拿到的错误就无法与一份可用目录并存
//
// TestLoadKeepsTheCatalogueOnFailure checks that a failed load leaves the catalogue as it was
// Loading happens while the binary starts, and a loader that empties the catalogue before reporting would leave the caller unable to pair that error with a usable catalogue
func TestLoadKeepsTheCatalogueOnFailure(t *testing.T) {
	before := len(Keys())
	if before == 0 {
		t.Fatal("测试前提不成立：目录为空 / the precondition fails: the catalogue is empty")
	}
	if err := Load(); err != nil {
		t.Fatalf("加载内嵌语言文件失败 / loading the embedded language files failed: %v", err)
	}
	if after := len(Keys()); after != before {
		t.Fatalf("加载后键数从 %d 变为 %d / the key count moved from %d to %d", before, after, before, after)
	}
}
