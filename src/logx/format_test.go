// 本文件证明 logx 替换 go-logger 后行格式不退化，分三层验证
// 第一层把两边的输出归一化后逐字符比较：时间戳与调用点掩码掉，其余部分必须完全相同，从而锁住字段顺序与分隔符
// 第二层单独断言调用点字段指向真正的调用文件而不是 logx 自身，这是异步化最容易丢掉的信息
// 第三层用一行实测捕获的真实输出作为金标，锁住时间戳的布局（四位年、两位月日、斜杠与冒号）
// go-logger 在这里作为参考实现生成期望格式，生产路径由 logx 独立渲染
//
// This file proves that replacing go-logger with logx does not degrade the line format, in three layers
// The first layer normalises both outputs and compares them character by character with timestamps and call sites masked, which pins the field order and the separators
// The second layer asserts on its own that the call-site field points at the real calling file rather than at logx itself, the piece of information asynchronous logging most easily loses
// The third layer pins the timestamp layout (four-digit year, two-digit month and day, slashes and colons) with a line captured from real output
// go-logger serves as a reference implementation here to produce the expected format, while the production path renders independently through logx
package logx

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	logger "github.com/donnie4w/go-logger/logger"
)

// fixedInstant 是全量对比与金标行使用的固定时刻
// fixedInstant is the pinned instant used by the whole-file comparison and the golden line
var fixedInstant = time.Date(2026, 9, 21, 14, 47, 17, 0, time.Local)

// timestampPattern 匹配日志行中的时间戳，用于归一化时掩码
// timestampPattern matches the timestamp inside a log line and is masked during normalisation
var timestampPattern = regexp.MustCompile(`\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}`)

// callSitePattern 匹配日志行中的调用点，形如 file.go:123
// callSitePattern matches the call site inside a log line, shaped like file.go:123
var callSitePattern = regexp.MustCompile(`[^\s]+\.go:\d+`)

// stdoutCaptureMu 串行化对 os.Stdout 的替换，避免并行用例互相干扰
// stdoutCaptureMu serialises the swaps of os.Stdout so parallel cases cannot disturb each other
var stdoutCaptureMu sync.Mutex

// captureStdout 在回调期间把 os.Stdout 换成管道并返回回调写入的字节
// captureStdout swaps os.Stdout for a pipe during the callback and returns the bytes the callback wrote
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	stdoutCaptureMu.Lock()
	defer stdoutCaptureMu.Unlock()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	_ = w.Close()
	os.Stdout = old
	out := <-done
	_ = r.Close()
	return out
}

// applyLibraryOption 按 main.initLogger 的方式配置 go-logger，使参考实现的格式与生产配置一致
// applyLibraryOption configures go-logger the way main.initLogger does, so the reference format matches the production configuration
func applyLibraryOption(level string, debug bool) {
	logLevel := logger.LEVEL_INFO
	switch level {
	case "debug":
		logLevel = logger.LEVEL_DEBUG
	case "warn":
		logLevel = logger.LEVEL_WARN
	case "error":
		logLevel = logger.LEVEL_ERROR
	}

	levelFmt := func(l logger.LEVELTYPE) string {
		switch l {
		case logger.LEVEL_DEBUG:
			return "[DEBUG]"
		case logger.LEVEL_INFO:
			return "[INFO] "
		case logger.LEVEL_WARN:
			return "[WARN] "
		case logger.LEVEL_ERROR:
			return "[ERROR]"
		case logger.LEVEL_FATAL:
			return "[FATAL]"
		default:
			return "[?????]"
		}
	}

	format := logger.FORMAT_LEVELFLAG | logger.FORMAT_DATE | logger.FORMAT_TIME
	formatter := "{level} {time} {message}\n"
	if debug {
		format |= logger.FORMAT_SHORTFILENAME
		formatter = "{level} {time} {file} {message}\n"
	}

	logger.SetOption(&logger.Option{
		Level:     logLevel,
		Console:   true,
		Format:    format,
		Formatter: formatter,
		AttrFormat: &logger.AttrFormat{
			SetLevelFmt: levelFmt,
		},
	})
}

// normalizeLine 把时间戳与调用点替换为占位符，使两边的行可以在内容上逐字符比较
// normalizeLine replaces the timestamp and the call site with placeholders so the two sides compare character by character on content
func normalizeLine(s string) string {
	s = timestampPattern.ReplaceAllString(s, "<TS>")
	return callSitePattern.ReplaceAllString(s, "<FILE>")
}

// newTestLogger 构造一个写入 io.Discard 的日志器，输出目标由用例在采集回调内改为当前的管道
// newTestLogger builds a logger writing to io.Discard, and each case points the output at the current pipe inside its capture callback
func newTestLogger(t *testing.T, level Level) *Logger {
	t.Helper()
	l := newLogger(level, false, io.Discard)
	l.SetClock(func() time.Time { return fixedInstant })
	t.Cleanup(l.Close)
	return l
}

// TestLineStructureMatchesLibrary 用归一化后的逐字符比较锁住字段顺序与分隔符
// 两边的调用点行号不同，因此调用点位置只比较占位符；时间戳同样以占位符比较，其布局由金标用例单独锁定
//
// TestLineStructureMatchesLibrary pins the field order and the separators with a character-by-character comparison of normalised lines
// The two sides differ in the call-site line number, so that position compares as a placeholder, and the timestamp compares as a placeholder too while the golden case pins its layout on its own
func TestLineStructureMatchesLibrary(t *testing.T) {
	const (
		infoMsg = "OptiRoute v0.4.0 启动 role:edge"
		warnMsg = "连接中心节点失败 err:dial tcp: refused"
	)

	applyLibraryOption("info", false)
	refOut := captureStdout(t, func() {
		logger.Info(infoMsg)
		logger.Warn(warnMsg)
	})

	l := newTestLogger(t, LevelInfo)
	gotOut := captureStdout(t, func() {
		// 输出目标在管道换好之后再指向它，使写 goroutine 写入被采集的管道
		// The output target is pointed at the pipe after the swap, so the writer goroutine writes into the captured pipe
		l.SetOutput(os.Stdout)
		l.Info(infoMsg)
		l.Warn(warnMsg)
		l.Flush()
	})

	refLines := strings.Split(strings.TrimRight(refOut, "\n"), "\n")
	gotLines := strings.Split(strings.TrimRight(gotOut, "\n"), "\n")
	if len(refLines) != 2 || len(gotLines) != 2 {
		t.Fatalf("expected two lines from each side, got ref=%d got=%d\nref=%q\ngot=%q", len(refLines), len(gotLines), refOut, gotOut)
	}

	for i := range refLines {
		want := normalizeLine(refLines[i])
		got := normalizeLine(gotLines[i])
		if want != got {
			t.Fatalf("line %d differs after normalisation\nref =%q\ngot =%q\nrawRef=%q\nrawGot=%q", i, want, got, refLines[i], gotLines[i])
		}
	}
}

// TestDebugLineReportsCallerNotLogx 断言 debug 行里的调用点是被测文件本身而不是 logx 的源码文件
// 异步化把渲染搬到写 goroutine，若调用点在那里采集就会全部变成 logx 的源码位置，本用例用于挡住该退化
//
// TestDebugLineReportsCallerNotLogx asserts that the call site in a debug line names the file under test rather than a logx source file
// Going asynchronous moves rendering to the writer goroutine, and collecting the call site there would turn every site into a logx source location, so this case guards against that regression
func TestDebugLineReportsCallerNotLogx(t *testing.T) {
	const msg = "拓扑已更新 nodes:2"

	l := newTestLogger(t, LevelDebug)
	gotOut := captureStdout(t, func() {
		l.SetOutput(os.Stdout)
		l.Debug(msg)
		l.Flush()
	})

	line := strings.TrimRight(gotOut, "\n")
	site := callSitePattern.FindString(line)
	if site == "" {
		t.Fatalf("the debug line carries no call site: %q", line)
	}
	if strings.HasPrefix(site, "logx.go:") || strings.HasPrefix(site, "format_test.go:0") {
		t.Fatalf("the call site should name the caller, got %q in line %q", site, line)
	}
	if !strings.HasPrefix(site, "format_test.go:") {
		t.Fatalf("the call site should name this test file, got %q", site)
	}
	if !strings.Contains(line, msg) {
		t.Fatalf("the debug line lost its message: %q", line)
	}
}

// TestDebugLineMatchesLibraryShape 对比 debug 配置下两边的归一化行，此时两侧都带调用点
// TestDebugLineMatchesLibraryShape compares the normalised lines of both sides under the debug configuration, where both carry a call site
func TestDebugLineMatchesLibraryShape(t *testing.T) {
	const msg = "拓扑已更新 nodes:2"

	applyLibraryOption("debug", true)
	refOut := captureStdout(t, func() {
		logger.Debug(msg)
	})

	l := newTestLogger(t, LevelDebug)
	gotOut := captureStdout(t, func() {
		l.SetOutput(os.Stdout)
		l.Debug(msg)
		l.Flush()
	})

	want := normalizeLine(strings.TrimRight(refOut, "\n"))
	got := normalizeLine(strings.TrimRight(gotOut, "\n"))
	if want != got {
		t.Fatalf("debug line differs after normalisation\nref =%q\ngot =%q", want, got)
	}
}

// TestRenderMatchesCapturedGoldenLine 用实测捕获的一行作为金标，锁住时间戳布局与分隔空格
// 该行取自 go-logger 在 info 配置下的真实输出，因此时间戳格式或分隔方式发生变化时本用例会失败
//
// TestRenderMatchesCapturedGoldenLine pins the timestamp layout and the separating spaces with a line captured from real output
// The line comes from the actual output of go-logger under the info configuration, so a change of timestamp layout or separator fails this case
func TestRenderMatchesCapturedGoldenLine(t *testing.T) {
	const golden = "[INFO]  2026/09/21 14:47:17 OptiRoute v0.4.0 启动 role:edge\n"

	l := newLogger(LevelInfo, false, io.Discard)
	defer l.Close()

	got := l.render(LevelInfo, "", "OptiRoute v0.4.0 启动 role:edge", fixedInstant)
	if got != golden {
		t.Fatalf("rendered line differs from the golden line\ngot   =%q\ngolden=%q", got, golden)
	}
}
