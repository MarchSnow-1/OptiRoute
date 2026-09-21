// 本文件对整个消息目录做一次跨包收口校验，是 i18n 的兜底门禁
// 校验四件事：源码里声明的每个键都已登记、登记表里没有多余键、每条消息三种语言齐备且格式符序列一致、非测试源码里不剩未走目录的日志调用
// 键的前缀还必须与其所在包的目录名一致，使日志里出现一个键就能看出它属于哪个包
// 放在 package main 下是因为它需要 import 全部角色包，从而触发各包 init 里的登记
//
// This file runs one cross-package check over the whole message catalogue and acts as the fallback gate for i18n
// It checks four things: every key declared in the source is registered, the registry holds no extra key, every message carries all three languages with the same verb sequence, and no non-test source keeps a log call that bypasses the catalogue
// A key's prefix also has to match the directory name of its package, so seeing a key in a log line already shows which package owns it
// It sits in package main because it imports every role package and therefore triggers the registration in each init
package main

import (
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

// keyDeclPattern 匹配形如 const KeyX i18n.Key = "pkg.key_name" 的声明
// keyDeclPattern matches a declaration shaped like const KeyX i18n.Key = "pkg.key_name"
var keyDeclPattern = regexp.MustCompile(`i18n\.Key\s*=\s*"([^"]+)"`)

// verbPattern 匹配一个格式符并捕获其尾部字母，%% 不会被匹配，因此转义后的百分号不计入
// verbPattern matches one format verb and captures its trailing letter, while %% is not matched so an escaped percent sign is not counted
var verbPattern = regexp.MustCompile(`%[-+# 0-9.]*([a-zA-Z])`)

// plainLogCallPattern 匹配未走消息目录的日志调用，带 f 后缀的带键方法不在其中
// plainLogCallPattern matches a log call that bypasses the message catalogue, while the keyed methods carrying an f suffix are not part of it
var plainLogCallPattern = regexp.MustCompile(`logx\.(Debug|Info|Warn|Error)\(`)

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

// verbsOf 返回文本中的格式符尾部字母序列
// verbsOf returns the sequence of trailing letters of the format verbs in a text
func verbsOf(text string) []string {
	matches := verbPattern.FindAllStringSubmatch(text, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// TestEveryDeclaredKeyIsRegistered 验证源码里声明的每个键都已进入登记表
// TestEveryDeclaredKeyIsRegistered checks that every key declared in the source has reached the registry
func TestEveryDeclaredKeyIsRegistered(t *testing.T) {
	declared := make(map[string]string)
	for _, f := range scanSources(t) {
		if f.isTest {
			continue
		}
		for _, m := range keyDeclPattern.FindAllStringSubmatch(f.content, -1) {
			key := m[1]
			if prev, dup := declared[key]; dup {
				t.Errorf("键 %q 在 %s 与 %s 中各声明了一次 / key %q is declared in both %s and %s", key, prev, f.path, key, prev, f.path)
			}
			declared[key] = f.path
		}
	}
	if len(declared) == 0 {
		t.Fatal("没有扫描到任何键声明，正则或源码布局可能已变 / no key declaration was found, so the pattern or the layout has changed")
	}

	registered := make(map[string]bool)
	for _, key := range i18n.Keys() {
		registered[string(key)] = true
	}

	for key, path := range declared {
		if !registered[key] {
			t.Errorf("%s 声明的键 %q 未登记到目录，日志会输出 !%s! 标记 / the key %q declared in %s is not registered, so a log line would print the marker !%s!", path, key, key, key, path, key)
		}
	}
	t.Logf("已声明并登记的键共 %d 个 / %d declared keys are registered", len(declared), len(declared))
}

// TestRegistryHasNoExtraKey 验证登记表里没有源码中查不到的键
// 多余的键通常来自改名或复制后忘记删除的常量，会让目录与实际输出不一致
//
// TestRegistryHasNoExtraKey checks that the registry holds no key the source does not declare
// An extra key usually comes from a constant left behind after a rename or a copy, and it would put the catalogue out of step with the output
func TestRegistryHasNoExtraKey(t *testing.T) {
	declared := make(map[string]bool)
	for _, f := range scanSources(t) {
		if f.isTest {
			continue
		}
		for _, m := range keyDeclPattern.FindAllStringSubmatch(f.content, -1) {
			declared[m[1]] = true
		}
	}

	for _, key := range i18n.Keys() {
		if !declared[string(key)] {
			t.Errorf("登记表里的键 %q 在源码中找不到声明 / the key %q in the registry has no declaration in the source", key, key)
		}
	}
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

// TestEveryMessageCarriesThreeLanguages 验证每条消息三种语言都有非空文案
// TestEveryMessageCarriesThreeLanguages checks that every message carries non-empty text in all three languages
func TestEveryMessageCarriesThreeLanguages(t *testing.T) {
	for _, key := range i18n.Keys() {
		entry, ok := i18n.Lookup(key)
		if !ok {
			t.Errorf("键 %q 查不到条目 / the key %q has no entry", key, key)
			continue
		}
		for lang, text := range entry.All() {
			if text == "" {
				t.Errorf("键 %q 缺少 %q 文案 / the key %q has no text for %q", key, lang, key, lang)
			}
		}
	}
}

// TestEveryMessageKeepsTheSameVerbSequence 验证一条消息在三种语言下使用相同的格式符序列
// 格式符数量或类型不一致时，占位会错位或渲染出格式符残留，因此这条断言是目录可用性的底线
//
// TestEveryMessageKeepsTheSameVerbSequence checks that one message uses the same verb sequence in all three languages
// A differing count or kind of verb misplaces the arguments or renders a leftover verb, so this assertion is the baseline of a usable catalogue
func TestEveryMessageKeepsTheSameVerbSequence(t *testing.T) {
	for _, key := range i18n.Keys() {
		entry, ok := i18n.Lookup(key)
		if !ok {
			continue
		}

		var reference []string
		var referenceLang string
		first := true
		for _, lang := range i18n.Supported() {
			text := entry.All()[lang]
			verbs := verbsOf(text)
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

// TestNoLogCallBypassesCatalogue 验证非测试源码里不剩未走消息目录的日志调用
// 带 f 后缀的方法才从目录取文案，因此任何 logx.Info、logx.Warn、logx.Error、logx.Debug 都说明有一行日志不会随语言切换
//
// TestNoLogCallBypassesCatalogue checks that no non-test source keeps a log call bypassing the message catalogue
// Only the methods carrying an f suffix take their text from the catalogue, so any logx.Info, logx.Warn, logx.Error or logx.Debug marks a log line that cannot follow the language
func TestNoLogCallBypassesCatalogue(t *testing.T) {
	for _, f := range scanSources(t) {
		if f.isTest {
			continue
		}
		for i, line := range strings.Split(f.content, "\n") {
			if plainLogCallPattern.MatchString(line) {
				t.Errorf("%s 第 %d 行仍在用未走目录的日志调用: %s / the line still logs outside the catalogue: %s", f.path, i+1, strings.TrimSpace(line), strings.TrimSpace(line))
			}
		}
	}
}
