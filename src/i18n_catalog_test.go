// 本文件对整个消息目录做一次跨包收口校验，是 i18n 的兜底门禁
// 文案的唯一来源是 i18n/locales 下的语言文件，键由各包以 Go 常量声明，本文件校验两者严格一一对应
// 校验项：三语键集合一致、每条三语齐备且格式符序列一致、每条带非空说明、语言文件里无重复键、声明的键都有条目、条目都有声明的键、键前缀等于包目录名
// 另校验迁移纪律：非测试源码里不剩未走目录的日志调用、没有用字符串字面量充当键、logx 未被别名或点导入
// 放在 package main 下是因为它需要按模块根目录扫描源码，而测试的工作目录就是该目录
//
// This file runs one cross-package check over the whole message catalogue and acts as the fallback gate for i18n
// The single source of the copy is the language files under i18n/locales, while the keys are declared as Go constants by each package, and this file checks that the two correspond one to one
// It checks that the three language files carry the same key set, that every entry has all three languages with the same verb sequence, that every entry carries a non-empty note, that no key repeats inside one language file, that every declared key has an entry, that every entry has a declared key, and that a key prefix equals its package directory
// It also checks the migration discipline: no non-test source keeps a log call that bypasses the catalogue, no string literal stands in for a key, and logx is imported under no alias
// It sits in package main because it has to walk the sources from the module root, which is the working directory of the test
package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	_ "github.com/MarchSnow-1/OptiRoute/agent"
	_ "github.com/MarchSnow-1/OptiRoute/auth"
	_ "github.com/MarchSnow-1/OptiRoute/center"
	_ "github.com/MarchSnow-1/OptiRoute/config"
	_ "github.com/MarchSnow-1/OptiRoute/edge"
	"github.com/MarchSnow-1/OptiRoute/i18n"
	_ "github.com/MarchSnow-1/OptiRoute/logx"
)

// localeDir 是语言文件相对模块根目录的路径
// localeDir is the path of the language files relative to the module root
const localeDir = "i18n/locales"

// keyDeclPattern 匹配形如 const KeyX i18n.Key = "pkg.key_name" 的声明
// 类型部分的 (?:i18n\.)? 覆盖 i18n 包自身可能出现的裸 Key，前导 const 覆盖单行声明
//
// keyDeclPattern matches a declaration shaped like const KeyX i18n.Key = "pkg.key_name"
// The optional i18n. qualifier covers a bare Key inside the i18n package itself, and the leading const covers a single-line declaration
var keyDeclPattern = regexp.MustCompile(`(?m)^\s*(?:const\s+)?[A-Za-z_][A-Za-z0-9_]*\s+(?:i18n\.)?Key\s*=\s*"([^"]+)"`)

// plainLogCallPattern 匹配未走消息目录的日志调用，带 f 后缀的带键方法不在其中
// 只锚定 logx. 前缀：加宽成任意接收者会把 err.Error() 与 func (l *Logger) Debug( 也算进来
// 别名导入这条逃逸口由 TestLogxIsImportedWithoutAlias 单独堵住
//
// plainLogCallPattern matches a log call that bypasses the message catalogue, while the keyed methods carrying an f suffix are not part of it
// It stays anchored on the logx. prefix: widening it to any receiver would also count err.Error() and a func (l *Logger) Debug( declaration
// The alias-import escape hatch is closed separately by TestLogxIsImportedWithoutAlias
var plainLogCallPattern = regexp.MustCompile(`logx\.(Debug|Info|Warn|Error)\(`)

// literalKeyCallPattern 匹配以一个字符串字面量代替键的带键调用，形如 logx.Infof("edge.foo", 1) 或 errf("config.foo")
// 未类型化的字符串常量可以赋给 i18n.Key 并直接作为 Key 参数传入，因此这种写法能编译，却绕过了「键必须是声明过的常量」这一前提
//
// literalKeyCallPattern matches a keyed call that passes a string literal instead of a key, shaped like logx.Infof("edge.foo", 1) or errf("config.foo")
// An untyped string constant may be passed where an i18n.Key is wanted, so this form compiles while skipping the premise that a key has to be a declared constant
var literalKeyCallPattern = regexp.MustCompile(`\b(?:logx\.(?:Debugf|Infof|Warnf|Errorf)|errf)\(\s*"`)

// aliasedLogxImportPattern 匹配对 logx 的别名导入或点导入，含单行 import 与分组 import 两种写法
//
// aliasedLogxImportPattern matches an aliased or dot import of logx, covering both the single-line import and the grouped form
var aliasedLogxImportPattern = regexp.MustCompile(`(?m)^\s*(?:import\s+)?(?:[A-Za-z_][A-Za-z0-9_]*\s+|\.\s+)"github\.com/MarchSnow-1/OptiRoute/logx"`)

// sourceFile 记录一个被扫描到的源文件
// sourceFile records one source file that was scanned
type sourceFile struct {
	path    string // 相对模块根目录的路径，用斜杠分隔 / path relative to the module root, separated by slashes
	pkgDir  string // 该文件所在包的目录名，模块根目录下的文件记为 main / directory name of the file's package, with a file at the module root recorded as main
	isTest  bool
	content string
}

// scanSources 遍历模块根目录下的全部 .go 文件
// 遍历从模块根目录开始，而 go test 会把测试进程的工作目录设到该目录
//
// scanSources walks every .go file under the module root
// The walk starts at the module root, which is the working directory go test gives the test process
func scanSources(t *testing.T) []sourceFile {
	t.Helper()

	var files []sourceFile
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// 版本控制与忽略目录不参与扫描，但根目录自身不能被跳过，否则整个遍历会立刻结束
			// Version-control and ignored directories take no part in the walk, while the root itself must not be skipped or the whole walk ends right away
			if path != "." && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		slash := filepath.ToSlash(path)
		dir := filepath.ToSlash(filepath.Dir(path))
		pkgDir := filepath.Base(dir)
		if dir == "." {
			pkgDir = "main"
		}
		files = append(files, sourceFile{
			path:    slash,
			pkgDir:  pkgDir,
			isTest:  strings.HasSuffix(slash, "_test.go"),
			content: string(raw),
		})
		return nil
	})
	if err != nil {
		t.Fatalf("遍历源码失败 / walking the sources failed: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("没有扫描到任何 .go 文件，工作目录可能不是模块根目录 / no .go file was found, so the working directory is probably not the module root")
	}
	return files
}

// localeEntry 是一个语言文件里的一条记录
// localeEntry is one record inside a language file
type localeEntry struct {
	Text string `json:"text"`
	Note string `json:"note"`
}

// readLocale 读取一个语言文件，并显式检测重复键
// 用词法扫描而不是一次 Unmarshal，因为 encoding/json 对重复键静默取后者，那会让一个键的文案被另一条覆盖而不报错
//
// readLocale reads one language file and detects a duplicate key explicitly
// It scans tokens instead of a single Unmarshal, because encoding/json silently keeps the last duplicate and that would let one entry overwrite another without an error
func readLocale(t *testing.T, lang string) map[string]localeEntry {
	t.Helper()

	path := filepath.Join(localeDir, lang+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取语言文件失败 / reading the language file failed: %v", err)
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	if _, err := dec.Token(); err != nil {
		t.Fatalf("%s 顶层不是对象 / the top level is not an object: %v", path, err)
	}
	out := make(map[string]localeEntry)
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("%s 解析键失败 / decoding a key failed: %v", path, err)
		}
		key, ok := tok.(string)
		if !ok {
			t.Fatalf("%s 出现非字符串的键 / a key is not a string", path)
		}
		if _, dup := out[key]; dup {
			t.Errorf("%s 里键 %q 出现多次，后一条会静默覆盖前一条 / the key %q appears more than once and would silently overwrite the earlier one", path, key, key)
		}
		var e localeEntry
		if err := dec.Decode(&e); err != nil {
			t.Fatalf("%s 的条目 %q 解析失败 / decoding the entry %q failed: %v", path, key, key, err)
		}
		out[key] = e
	}
	if len(out) == 0 {
		t.Fatalf("%s 没有任何条目 / the file carries no entry", path)
	}
	return out
}

// localeLangs 返回受支持语言，并核对每个语言文件都存在
// localeLangs returns the supported languages and confirms that each language file exists
func localeLangs(t *testing.T) []i18n.Lang {
	t.Helper()

	langs := i18n.Supported()
	if len(langs) == 0 {
		t.Fatal("i18n.Supported() 为空 / i18n.Supported() is empty")
	}
	for _, l := range langs {
		path := filepath.Join(localeDir, l.String()+".json")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("语言 %s 的条目文件缺失 / the file for language %s is missing: %v", l, l, err)
		}
	}
	return langs
}

// declaredKeys 返回源码里声明的全部键及其所在文件
// declaredKeys returns every key declared in the source together with the file that declares it
func declaredKeys(t *testing.T) map[string]string {
	t.Helper()

	declared := make(map[string]string)
	for _, f := range scanSources(t) {
		if f.isTest {
			continue
		}
		for _, m := range keyDeclPattern.FindAllStringSubmatch(f.content, -1) {
			key := m[1]
			if prev, dup := declared[key]; dup {
				t.Errorf("键 %q 在 %s 与 %s 中各声明了一次 / the key %q is declared in both %s and %s", key, prev, f.path, key, prev, f.path)
			}
			declared[key] = f.path
		}
	}
	if len(declared) == 0 {
		t.Fatal("没有扫描到任何键声明，正则或源码布局可能已变 / no key declaration was found, so the pattern or the layout has changed")
	}
	return declared
}

// verbsOf 返回文本中的格式符尾部字母序列，判定口径与 fmt 一致
// 用扫描而不是正则：正则的字符类若把空格算进标志位，%% 之后的空格加字母会被误判成一个格式符，使含转义百分号的文案判反
//
// verbsOf returns the sequence of trailing letters of the format verbs in a text, judged the way fmt does
// It scans rather than matching a regular expression: a character class that counts a space as a flag would read the space and letter after %% as a verb, which judges text carrying an escaped percent sign the wrong way round
func verbsOf(text string) []string {
	out := make([]string, 0, 4)
	for i := 0; i < len(text); i++ {
		if text[i] != '%' {
			continue
		}
		if i+1 < len(text) && text[i+1] == '%' {
			i++
			continue
		}
		j := i + 1
		for j < len(text) && strings.ContainsRune("+-# 0.123456789", rune(text[j])) {
			j++
		}
		if j < len(text) && isASCIILetter(text[j]) {
			out = append(out, string(text[j]))
			i = j
		}
	}
	return out
}

// isASCIILetter 判断 c 是否为 ASCII 字母
// isASCIILetter reports whether c is an ASCII letter
func isASCIILetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// TestLocaleKeySetsMatch 验证各语言文件的键集合完全一致
// TestLocaleKeySetsMatch checks that the language files carry exactly the same key set
func TestLocaleKeySetsMatch(t *testing.T) {
	langs := localeLangs(t)
	reference := readLocale(t, langs[0].String())
	referenceName := langs[0].String()

	for _, l := range langs[1:] {
		got := readLocale(t, l.String())
		for key := range reference {
			if _, ok := got[key]; !ok {
				t.Errorf("键 %q 在 %s 里有而 %s 里没有 / the key %q is in %s but not in %s", key, referenceName, l, key, referenceName, l)
			}
		}
		for key := range got {
			if _, ok := reference[key]; !ok {
				t.Errorf("键 %q 在 %s 里有而 %s 里没有 / the key %q is in %s but not in %s", key, l, referenceName, key, l, referenceName)
			}
		}
	}
	t.Logf("三语一致的键共 %d 个 / %d keys agree across the languages", len(reference), len(reference))
}

// TestEveryEntryCarriesThreeLanguagesAndANote 验证每条消息三语齐备、文案非空，且带非空说明
// 说明字段是 JSON 无法承载注释的补偿，缺了它下一位维护者就看不到触发条件与参数含义
//
// TestEveryEntryCarriesThreeLanguagesAndANote checks that every entry carries all three languages with non-empty copy and a non-empty note
// The note compensates for JSON carrying no comments, and without it the next maintainer cannot see the trigger condition or what the arguments mean
func TestEveryEntryCarriesThreeLanguagesAndANote(t *testing.T) {
	langs := localeLangs(t)
	for _, l := range langs {
		entries := readLocale(t, l.String())
		for key, e := range entries {
			if strings.TrimSpace(e.Text) == "" {
				t.Errorf("[%s] %s 的文案为空 / the copy is empty", l, key)
			}
			if strings.TrimSpace(e.Note) == "" {
				t.Errorf("[%s] %s 缺少说明 / the entry has no note", l, key)
			}
		}
	}
}

// TestEveryEntryKeepsTheSameVerbSequence 验证一条消息在三种语言下使用相同的格式符序列
// 格式符数量或类型不一致时，占位会错位或渲染出格式符残留，因此这条断言是目录可用性的底线
//
// TestEveryEntryKeepsTheSameVerbSequence checks that one message uses the same verb sequence in all three languages
// A differing count or kind of verb misplaces the arguments or renders a leftover verb, so this assertion is the baseline of a usable catalogue
func TestEveryEntryKeepsTheSameVerbSequence(t *testing.T) {
	for _, key := range i18n.Keys() {
		entry, ok := i18n.Lookup(key)
		if !ok {
			continue
		}
		var reference []string
		var referenceLang string
		first := true
		for _, lang := range i18n.Supported() {
			verbs := verbsOf(entry.All()[lang])
			if first {
				reference, referenceLang, first = verbs, string(lang), false
				continue
			}
			if len(verbs) != len(reference) {
				t.Errorf("键 %q 的 %q 有 %d 个格式符，%q 有 %d 个: %v 对 %v / the %q text of key %q carries %d verbs while %q carries %d", key, lang, len(verbs), referenceLang, len(reference), verbs, reference, lang, key, len(verbs), referenceLang, len(reference))
				continue
			}
			for i := range verbs {
				if verbs[i] != reference[i] {
					t.Errorf("键 %q 第 %d 个格式符在 %q 是 %%%s，在 %q 是 %%%s / the %d-th verb of key %q is %%%s in %q and %%%s in %q", key, i+1, lang, verbs[i], referenceLang, reference[i], i+1, key, verbs[i], lang, reference[i], referenceLang)
					break
				}
			}
		}
	}
}

// TestDeclaredKeysAndEntriesMatchOneToOne 验证声明的键与语言文件的条目严格一一对应
// 这是「JSON 承载文案、Go 常量承载键」这一设计的守门条件：少一边就会在运行时输出 !键! 标记，多一边则是无人引用的死条目
//
// TestDeclaredKeysAndEntriesMatchOneToOne checks that the declared keys and the catalogue entries correspond one to one
// It guards the design of copy in JSON and keys in Go constants: missing either side leads to a !key! marker at run time or leaves an entry nothing refers to
func TestDeclaredKeysAndEntriesMatchOneToOne(t *testing.T) {
	registered := make(map[string]bool)
	for _, key := range i18n.Keys() {
		registered[string(key)] = true
	}

	declared := declaredKeys(t)
	for key, path := range declared {
		if !registered[key] {
			t.Errorf("%s 声明的键 %q 没有条目，日志会输出 !%s! 标记 / the key %q declared in %s has no entry, so a log line would print the marker !%s!", path, key, key, key, path, key)
		}
	}
	for _, key := range i18n.Keys() {
		if _, ok := declared[string(key)]; !ok {
			t.Errorf("条目 %q 在源码中找不到键声明 / the entry %q has no key declaration in the source", key, key)
		}
	}
	t.Logf("已声明且有条目的键共 %d 个 / %d declared keys carry an entry", len(declared), len(declared))
}

// TestKeyPrefixMatchesPackage 验证键的前缀与其所在包的目录名一致
// TestKeyPrefixMatchesPackage checks that a key's prefix matches the directory name of the package that declares it
func TestKeyPrefixMatchesPackage(t *testing.T) {
	for _, f := range scanSources(t) {
		if f.isTest {
			continue
		}
		for _, m := range keyDeclPattern.FindAllStringSubmatch(f.content, -1) {
			key := m[1]
			prefix, _, ok := strings.Cut(key, ".")
			if !ok {
				t.Errorf("%s 的键 %q 没有前缀 / the key %q declared in %s carries no prefix", f.path, key, key, f.path)
				continue
			}
			if prefix != f.pkgDir {
				t.Errorf("%s 属于包 %s，但键 %q 的前缀是 %q / the file belongs to package %s while the key %q carries the prefix %q", f.path, f.pkgDir, key, prefix, f.pkgDir, key, prefix)
			}
		}
	}
}

// TestLocaleDirectoryIsTheOnlyCopySource 验证非测试源码里不再出现内联的三语文案
// 改造之后文案只应存在于语言文件里，源码中若仍出现 i18n.New 形式的三元组，说明有包没有切过来
//
// TestLocaleDirectoryIsTheOnlyCopySource checks that no non-test source still carries inline copy in three languages
// After the change the copy belongs to the language files alone, so an i18n.New triple in the source means a package did not switch over
func TestLocaleDirectoryIsTheOnlyCopySource(t *testing.T) {
	pattern := regexp.MustCompile(`i18n\.New\(|i18n\.Register\(`)
	for _, f := range scanSources(t) {
		if f.isTest {
			continue
		}
		for i, line := range strings.Split(f.content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if pattern.MatchString(line) {
				t.Errorf("%s 第 %d 行仍在源码里内联文案: %s / the line still carries inline copy in the source: %s", f.path, i+1, trimmed, trimmed)
			}
		}
	}
}

// TestNoLogCallBypassesCatalogue 验证非测试源码里不剩未走消息目录的日志调用
// 带 f 后缀的方法才从目录取文案，因此任何 logx.Info、logx.Warn、logx.Error、logx.Debug 都说明有一行日志不会随语言切换
// 整行注释被跳过：注释里可能举出这类调用来说明为什么不该用
//
// TestNoLogCallBypassesCatalogue checks that no non-test source keeps a log call bypassing the message catalogue
// Only the methods carrying an f suffix take their text from the catalogue, so any logx.Info, logx.Warn, logx.Error or logx.Debug marks a log line that cannot follow the language
// A comments-only line is skipped, because a comment may name such a call while explaining why one is not used
func TestNoLogCallBypassesCatalogue(t *testing.T) {
	for _, f := range scanSources(t) {
		if f.isTest {
			continue
		}
		for i, line := range strings.Split(f.content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if plainLogCallPattern.MatchString(line) {
				t.Errorf("%s 第 %d 行仍在用未走目录的日志调用: %s / the line still logs outside the catalogue: %s", f.path, i+1, trimmed, trimmed)
			}
		}
	}
}

// TestNoLiteralKeyAtCallSite 验证调用点不直接用字符串字面量充当消息键
// 未类型化的字符串常量可以赋给 i18n.Key，因此这种写法能编译，却绕过了「键必须声明为常量」这一前提
// 它既不会进入声明集合，也不会被条目校验发现，运行时只会打印 !键! 标记
//
// TestNoLiteralKeyAtCallSite checks that no call site passes a bare string literal where a message key is wanted
// An untyped string constant may be passed where an i18n.Key is wanted, so the form compiles while skipping the premise that a key has to be declared as a constant
// It enters neither the declaration set nor the entry check, and at run time it only prints the !key! marker
func TestNoLiteralKeyAtCallSite(t *testing.T) {
	for _, f := range scanSources(t) {
		if f.isTest {
			continue
		}
		for i, line := range strings.Split(f.content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if literalKeyCallPattern.MatchString(line) {
				t.Errorf("%s 第 %d 行用字符串字面量充当消息键，应改为声明的常量: %s / the line passes a string literal as a message key instead of a declared constant: %s", f.path, i+1, trimmed, trimmed)
			}
		}
	}
}

// TestLogxIsImportedWithoutAlias 验证 logx 未被别名导入或点导入
// 别名导入会让以 logx. 为锚点的检查全部失效，点导入使调用点看起来像普通函数，两者都会让迁移后的回归长期不被发现
//
// TestLogxIsImportedWithoutAlias checks that logx is imported without an alias and not dot-imported
// An aliased import defeats every check anchored on logx. and a dot import makes a call site look like a plain function, so either one would let a regression stay hidden long after the migration
func TestLogxIsImportedWithoutAlias(t *testing.T) {
	for _, f := range scanSources(t) {
		if f.isTest {
			continue
		}
		if m := aliasedLogxImportPattern.FindString(f.content); m != "" {
			t.Errorf("%s 以别名或点导入方式引用 logx，会使门禁锚点失效: %s / the file imports logx under an alias or as a dot import, which defeats the gate anchor: %s", f.path, strings.TrimSpace(m), strings.TrimSpace(m))
		}
	}
}

// TestVerbsOfHandlesEscapedPercent 验证格式符扫描把 %% 当作转义处理，且能识别带宽度与精度的格式符
// 回归点：早先的正则把空格算进标志位，使 50%% now 被读出一个并不存在的格式符，含转义百分号的文案会被判反
//
// TestVerbsOfHandlesEscapedPercent checks that the verb scan treats %% as an escape and still recognises a verb carrying width and precision
// The regression it pins: the earlier regular expression counted a space as a flag, so 50%% now produced a verb that does not exist and text carrying an escaped percent sign was judged the wrong way round
func TestVerbsOfHandlesEscapedPercent(t *testing.T) {
	cases := map[string][]string{
		"":                        nil,
		"no verbs here":           nil,
		"load 50%% now":           nil,
		"progress 100%% done":     nil,
		"已完成 50%% 的负载":            nil,
		"%d files":                {"d"},
		"%s and %d":               {"s", "d"},
		"%d files, 50%% done":     {"d"},
		"%.2f ms":                 {"f"},
		"%6.3f ms":                {"f"},
		"%q":                      {"q"},
		"%%d should not count":    nil,
		"%s%%":                    {"s"},
		"50%% then %s":            {"s"},
		"%v after %% and a space": {"v"},
	}

	for text, want := range cases {
		got := verbsOf(text)
		if len(got) != len(want) {
			t.Errorf("verbsOf(%q) = %v，期望 %v / want %v", text, got, want, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("verbsOf(%q)[%d] = %q，期望 %q / want %q", text, i, got[i], want[i], want[i])
				break
			}
		}
	}
}

// TestLocaleFilesLoadCleanly 验证内嵌语言文件能被加载器完整读取
// 三语键集、格式符序列与说明齐备由本文件其余用例校验，这里只确认加载本身没有报错且目录非空
//
// TestLocaleFilesLoadCleanly checks that the embedded language files load completely
// The key sets, the verb sequences and the notes are checked by the other cases in this file, so this one only confirms that loading itself reports no error and leaves a non-empty catalogue
func TestLocaleFilesLoadCleanly(t *testing.T) {
	if err := i18n.Load(); err != nil {
		t.Fatalf("加载内嵌语言文件失败 / loading the embedded language files failed: %v", err)
	}
	if len(i18n.Keys()) == 0 {
		t.Fatal("加载之后目录为空 / the catalogue is empty after loading")
	}
	t.Logf("目录共 %d 个键 / the catalogue holds %d keys", len(i18n.Keys()), len(i18n.Keys()))
}
