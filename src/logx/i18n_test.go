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

// keyedTestKey 是本文件专用的一条目录消息，登记于包级目录并覆盖三种语言
// 该键只由本文件的用例使用，不与 logx 自己登记的键重名
//
// keyedTestKey is a catalogue message owned by this file, registered in the package-wide catalogue with all three languages
// The key is used by the cases in this file only and does not clash with the keys logx registers itself
const keyedTestKey i18n.Key = "logx.test.keyed_message"

// init 登记本文件专用消息，参数为被记录的次数
// init registers the message owned by this file, whose argument is the number of times something happened
func init() {
	i18n.Register(map[i18n.Key]i18n.Entry{
		keyedTestKey: i18n.New("event %d recorded", "已记录事件 %d 次", "已記錄事件 %d 次"),
	})
}

// TestKeyedMethodsFollowCurrentLanguage 验证带键方法的输出随当前语言变化
// TestKeyedMethodsFollowCurrentLanguage checks that the output of the keyed methods follows the current language
func TestKeyedMethodsFollowCurrentLanguage(t *testing.T) {
	cases := map[i18n.Lang]string{
		i18n.LangEN:     "event 7 recorded",
		i18n.LangZHHans: "已记录事件 7 次",
		i18n.LangZHant:  "已記錄事件 7 次",
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
	for _, want := range []string{"event 1 recorded", "event 2 recorded", "event 3 recorded", "event 4 recorded"} {
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
	if !strings.Contains(line, "event 42 recorded") {
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
	for _, want := range []string{"event 11 recorded", "event 12 recorded", "event 13 recorded", "event 14 recorded"} {
		if !strings.Contains(out, want) {
			t.Errorf("包级函数输出应包含 %q，实际 %q / the package-level helper output should contain %q, got %q", want, out, want, out)
		}
	}
}
