// 本文件覆盖 logx 带消息键的四个方法：文案随当前语言取、被级别过滤时不查目录、禁用时零输出、debug 级别下调用点仍指向调用方
// 这四个性质共同使 i18n 接入后热路径的开销与异步化之前一致，同时行格式不变
//
// This file covers the four message-key methods of logx: the text follows the current language, a filtered call touches no catalogue entry, a disabled logger emits nothing and the call site still names the caller at the debug level
// Together these properties keep the cost on a hot path the same as it was before the asynchronous rewrite, while the line format stays unchanged
package logx

import (
	"os"
	"strings"
	"testing"

	"github.com/MarchSnow-1/OptiRoute/i18n"
)

// keyedTestKey 是本包真实存在的那个键，直接用它可以省掉为测试单独登记一条消息
// 文案由 i18n/locales 下的语言文件提供，参数为被丢弃的记录条数
//
// keyedTestKey is the key this package really has, and using it directly removes the need to register a message for the test alone
// Its copy comes from the language files under i18n/locales and its argument is the number of records dropped
const keyedTestKey = KeyQueueFull

// TestKeyedMethodsFollowCurrentLanguage 验证带键方法的输出随当前语言变化
// TestKeyedMethodsFollowCurrentLanguage checks that the output of the keyed methods follows the current language
func TestKeyedMethodsFollowCurrentLanguage(t *testing.T) {
	cases := map[i18n.Lang]string{
		i18n.LangEN:     "log queue full, 7 records dropped",
		i18n.LangZHHans: "日志队列已满，丢弃 7 条",
		i18n.LangZHant:  "日誌佇列已滿，丟棄 7 條",
	}

	for lang, want := range cases {
		t.Run(string(lang), func(t *testing.T) {
			setLangForTest(t, lang)

			l, read := newCaptureLogger(t, LevelInfo, false)
			l.Infof(keyedTestKey, 7)
			l.Flush()

			if out := read(); !strings.Contains(out, want) {
				t.Fatalf("Infof 输出应包含 %q，实际 %q / Infof should contain %q, got %q", want, out, want, out)
			}
		})
	}
}

// TestKeyedMethodsCoverEveryLevel 验证四个级别的方法都输出各自级别的标签
// TestKeyedMethodsCoverEveryLevel checks that the four level methods each emit their own level tag
func TestKeyedMethodsCoverEveryLevel(t *testing.T) {
	setLangForTest(t, i18n.LangEN)

	l, read := newCaptureLogger(t, LevelDebug, false)
	l.Debugf(keyedTestKey, 1)
	l.Infof(keyedTestKey, 2)
	l.Warnf(keyedTestKey, 3)
	l.Errorf(keyedTestKey, 4)
	l.Flush()

	out := read()
	for _, want := range []string{"[DEBUG]", "[INFO] ", "[WARN] ", "[ERROR]"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出应包含级别标签 %q，实际 %q / the output should carry the level tag %q, got %q", want, out, want, out)
		}
	}
	for _, want := range []string{"records dropped", "records dropped", "records dropped", "records dropped"} {
		if !strings.Contains(out, want) {
			t.Errorf("输出应包含 %q，实际 %q / the output should contain %q, got %q", want, out, want, out)
		}
	}
}

// TestFilteredKeyedCallFetchesNoText 验证低于当前级别的调用不去查目录
// 未登记的键会渲染成 !消息键! 标记，因此只要输出里没有该标记，就说明取文案发生在级别过滤之后
//
// TestFilteredKeyedCallFetchesNoText checks that a call below the current level looks nothing up in the catalogue
// An unregistered key renders as the marker !message.key!, so the absence of that marker in the output shows the lookup happens after the level check
func TestFilteredKeyedCallFetchesNoText(t *testing.T) {
	setLangForTest(t, i18n.LangEN)

	const unregistered i18n.Key = "logx.test.never_registered"
	if _, ok := i18n.Lookup(unregistered); ok {
		t.Fatalf("键 %q 不应已登记 / key %q should not be registered", unregistered, unregistered)
	}
	marker := "!" + string(unregistered) + "!"

	l, read := newCaptureLogger(t, LevelInfo, false)

	l.Debugf(unregistered, "ignored")
	l.Flush()
	if out := read(); strings.Contains(out, marker) {
		t.Fatalf("低于当前级别的调用不应取文案，实际输出 %q / a call below the current level should fetch no text, got %q", out, out)
	}

	// 同一条消息在级别放行时必须出现标记，用来证明上面的断言不是因为它本身不产生输出
	// The same message has to show the marker once the level admits it, which shows the assertion above is not passing merely because the call emits nothing
	l.Infof(unregistered, "kept")
	l.Flush()
	if out := read(); !strings.Contains(out, marker) {
		t.Fatalf("级别放行时应出现缺失标记 %q，实际输出 %q / the missing-text marker %q should appear once the level admits the call, got %q", marker, out, marker, out)
	}
}

// TestDisabledKeyedCallIsSilent 验证禁用时带键方法零输出
// TestDisabledKeyedCallIsSilent checks that a disabled logger emits nothing through the keyed methods
func TestDisabledKeyedCallIsSilent(t *testing.T) {
	setLangForTest(t, i18n.LangEN)

	l, read := newCaptureLogger(t, LevelDebug, true)
	l.Debugf(keyedTestKey, 1)
	l.Infof(keyedTestKey, 2)
	l.Warnf(keyedTestKey, 3)
	l.Errorf(keyedTestKey, 4)
	l.Flush()

	if out := read(); out != "" {
		t.Fatalf("禁用时应零输出，实际 %q / a disabled logger should stay silent, got %q", out, out)
	}
}

// TestKeyedDebugLineReportsCaller 验证 debug 级别下带键方法的调用点指向调用方而不是 logx 源码
// 该断言挡住调用点采集层数写错这一退化，与 format_test.go 中固定文案的同类断言成对
//
// TestKeyedDebugLineReportsCaller checks that at the debug level the call site of a keyed method names the caller rather than a logx source file
// The assertion guards against a wrong frame count in the call-site helper and pairs with the matching assertion for fixed text in format_test.go
func TestKeyedDebugLineReportsCaller(t *testing.T) {
	setLangForTest(t, i18n.LangEN)

	l := newTestLogger(t, LevelDebug)
	gotOut := captureStdout(t, func() {
		l.SetOutput(os.Stdout)
		l.Debugf(keyedTestKey, 42)
		l.Flush()
	})

	line := strings.TrimRight(gotOut, "\n")
	site := callSitePattern.FindString(line)
	if site == "" {
		t.Fatalf("debug 行未带调用点: %q / the debug line carries no call site: %q", line, line)
	}
	if !strings.HasPrefix(site, "i18n_test.go:") {
		t.Fatalf("调用点应指向本文件，实际 %q，整行 %q / the call site should name this file, got %q in line %q", site, line, site, line)
	}
	if !strings.Contains(line, "records dropped") {
		t.Fatalf("debug 行丢失文案: %q / the debug line lost its message: %q", line, line)
	}
}

// TestPackageLevelKeyedHelpersLog 验证包级函数转发到默认日志器
// TestPackageLevelKeyedHelpersLog checks that the package-level helpers forward to the default logger
func TestPackageLevelKeyedHelpersLog(t *testing.T) {
	setLangForTest(t, i18n.LangEN)

	// 默认日志器的级别、开关与输出在此临时改写，结束后由 Cleanup 还原为构造时的取值
	// The level, switch and output of the default logger are changed here and restored by the cleanup to the values it was built with
	sink := &lockedBuffer{}
	Init("debug", false)
	std.SetOutput(sink)
	t.Cleanup(func() {
		std.SetOutput(os.Stdout)
		Init("info", false)
	})

	Debugf(keyedTestKey, 11)
	Infof(keyedTestKey, 12)
	Warnf(keyedTestKey, 13)
	Errorf(keyedTestKey, 14)
	Flush()

	out := sink.String()
	for _, want := range []string{"records dropped", "records dropped", "records dropped", "records dropped"} {
		if !strings.Contains(out, want) {
			t.Errorf("包级函数输出应包含 %q，实际 %q / the package-level helper output should contain %q, got %q", want, out, want, out)
		}
	}
}

// useDefaultLogger 把包级默认日志器接到内存缓冲并切到 debug 级别，返回读取输出的函数
// 该函数用于覆盖「通过包级函数调用」这条路径，它与通过 Logger 实例调用相比多一层栈帧
//
// useDefaultLogger wires the package-level default logger to a memory buffer at the debug level and returns a reader for the output
// It exists to cover the path through a package-level function, which carries one more stack frame than a call on a Logger instance
func useDefaultLogger(t *testing.T) func() string {
	t.Helper()

	sink := &lockedBuffer{}
	Init("debug", false)
	std.SetOutput(sink)
	t.Cleanup(func() {
		std.SetOutput(os.Stdout)
		Init("info", false)
	})
	return func() string { return sink.String() }
}

// TestPackageLevelDebugLineReportsCaller 验证通过包级函数输出时 debug 行的调用点指向调用方
// 包级函数若转发到 Logger 上的同名方法而不是直接调用内部方法，调用链会多一层，调用点会变成 logx.go 里包装函数的行号
//
// TestPackageLevelDebugLineReportsCaller checks that a debug line written through a package-level function names the caller
// A package-level function that forwards to the same-named method on Logger instead of calling the internal one directly adds a frame, and the call site then becomes the line of that wrapper in logx.go
func TestPackageLevelDebugLineReportsCaller(t *testing.T) {
	setLangForTest(t, i18n.LangEN)
	read := useDefaultLogger(t)

	Debug("fixed text from a package-level call")
	Debugf(keyedTestKey, 21)
	Flush()

	for _, line := range strings.Split(strings.TrimRight(read(), "\n"), "\n") {
		site := callSitePattern.FindString(line)
		if site == "" {
			t.Fatalf("debug 行未带调用点: %q / the debug line carries no call site: %q", line, line)
		}
		if !strings.HasPrefix(site, "i18n_test.go:") {
			t.Fatalf("包级调用的调用点应指向本文件，实际 %q，整行 %q / the call site of a package-level call should name this file, got %q in line %q", site, line, site, line)
		}
	}
}

// TestEveryPackageLevelHelperReportsCaller 验证八个包级函数的调用点都指向调用方而不是 logx.go
// 逐个函数覆盖，避免只测其中一个而漏掉其余七个的 skip 写错
//
// TestEveryPackageLevelHelperReportsCaller checks that the call site of all eight package-level functions names the caller rather than logx.go
// Every function is covered one by one, so that a wrong skip in any single one of them cannot slip through
func TestEveryPackageLevelHelperReportsCaller(t *testing.T) {
	setLangForTest(t, i18n.LangEN)
	read := useDefaultLogger(t)

	Debug("d")
	Info("i")
	Warn("w")
	Error("e")
	Debugf(keyedTestKey, 1)
	Infof(keyedTestKey, 2)
	Warnf(keyedTestKey, 3)
	Errorf(keyedTestKey, 4)
	Flush()

	lines := strings.Split(strings.TrimRight(read(), "\n"), "\n")
	if len(lines) != 8 {
		t.Fatalf("应有 8 行输出，实际 %d 行: %q / 8 lines were expected, got %d: %q", len(lines), lines, len(lines), lines)
	}
	for _, line := range lines {
		site := callSitePattern.FindString(line)
		if site == "" {
			t.Fatalf("debug 行未带调用点: %q / the debug line carries no call site: %q", line, line)
		}
		if strings.HasPrefix(site, "logx.go:") {
			t.Fatalf("调用点退化成了 logx 源码位置: %q / the call site degraded to a logx source location: %q", site, line)
		}
		if !strings.HasPrefix(site, "i18n_test.go:") {
			t.Fatalf("调用点应指向本文件，实际 %q，整行 %q / the call site should name this file, got %q in line %q", site, line, site, line)
		}
	}
}
