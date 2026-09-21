// 本包提供有界异步日志：调用方只把日志投进内存队列，由单独的写 goroutine 落到标准输出
// 建立原因：原先每次 logger.Info 都在调用方 goroutine 内同步写 os.Stdout，标准输出接到满的管道或不读的采集端时该写会一直阻塞
// 数据面建连路径上的日志调用点因此可能把连接卡死，异步化把写动作从调用方 goroutine 移开
// 渲染由本包自己完成而非复用 go-logger：go-logger 的渲染与写盘是同一步，无法只取字节不写出，而异步写需要先在调用方生成完整行
// 应用代码用带消息键的方法输出（Debugf、Infof、Warnf、Errorf），文案由 i18n 目录按当前语言提供，级别过滤发生在取文案之前
// 保留 go-logger 作为参考实现：format_test.go 用它与本包逐字段对比输出，确保替换后行格式不变
// 队列满时丢弃日志并计数，不阻塞调用方；退出前用 Flush 冲刷，崩溃时队列中未写出的日志会丢失
//
// This package provides bounded asynchronous logging: a caller only queues a record in memory, and a dedicated writer goroutine writes it to standard output
// Reason for its existence: every logger.Info used to write os.Stdout synchronously inside the calling goroutine, and that write blocks while standard output is a full pipe or a collector that stopped reading
// A log call on the connection setup path of the data plane could therefore stall a connection, and going asynchronous moves the write off the calling goroutine
// Rendering is done here rather than by reusing go-logger, because go-logger renders and writes in the same step and offers no way to obtain the bytes alone, while an asynchronous write needs the complete line built in the caller
// Application code logs through the keyed methods Debugf, Infof, Warnf and Errorf, whose text comes from the i18n catalogue in the current language, and the level check runs before that text is fetched
// go-logger is kept as a reference implementation: format_test.go compares its output field by field with this package so that replacing it cannot change the line format
// A full queue drops records and counts them instead of blocking the caller; Flush drains the queue before exit, while records still queued at a crash are lost
package logx

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MarchSnow-1/OptiRoute/i18n"
)

// Level 是日志级别，数值越大越严重
// Level is the log level, where a larger value is more severe
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// ParseLevel 把配置中的级别名转成 Level，无法识别时返回 LevelInfo 与 false
// ParseLevel turns a configured level name into a Level and returns LevelInfo with false when the name is unknown
func ParseLevel(name string) (Level, bool) {
	switch name {
	case "debug":
		return LevelDebug, true
	case "info":
		return LevelInfo, true
	case "warn":
		return LevelWarn, true
	case "error":
		return LevelError, true
	default:
		return LevelInfo, false
	}
}

// levelTag 返回与日志库一致的水平标签，注意 info 与 warn 的标签自带一个尾随空格
// levelTag returns the level tag matching the logging library, and the tags of info and warn carry a trailing space on purpose
func levelTag(l Level) string {
	switch l {
	case LevelDebug:
		return "[DEBUG]"
	case LevelInfo:
		return "[INFO] "
	case LevelWarn:
		return "[WARN] "
	case LevelError:
		return "[ERROR]"
	default:
		return "[?????]"
	}
}

// queueSize 是内存队列容量，达到该容量后新日志被丢弃并计数
// queueSize is the in-memory queue capacity, beyond which new records are dropped and counted
const queueSize = 4096

// flushTimeout 是 Flush 等待队列排空的最长时间
// flushTimeout is the longest a Flush waits for the queue to drain
const flushTimeout = 2 * time.Second

// entry 是一条排队中的日志，msg 在调用方就已经渲染成型，因此队列里保存的是不可变字符串
// entry is one queued record whose msg was already built by the caller, so the queue holds an immutable string
type entry struct {
	level   Level
	file    string // 仅 debug 级别填充，形如 file.go:123 / filled for the debug level only, shaped like file.go:123
	msg     string
	barrier chan struct{} // 非 nil 时表示这是一次 Flush 的屏障 / a non-nil value marks a Flush barrier
}

// Logger 是有界异步日志器
// Logger is a bounded asynchronous logger
type Logger struct {
	level    Level
	withFile bool // 是否在每行中输出调用点，对应日志库的 SHORTFILENAME / whether each line carries the call site, matching SHORTFILENAME of the logging library
	disabled bool

	ch        chan entry
	dropped   atomic.Int64
	writeWG   sync.WaitGroup
	closeOnce sync.Once
	done      chan struct{}

	mu  sync.Mutex
	out io.Writer
	now func() time.Time
}

// newLogger 构造日志器并启动唯一的写 goroutine
// newLogger builds a logger and starts its single writer goroutine
func newLogger(level Level, disable bool, out io.Writer) *Logger {
	l := &Logger{
		level:    level,
		withFile: level == LevelDebug,
		disabled: disable,
		ch:       make(chan entry, queueSize),
		done:     make(chan struct{}),
		out:      out,
		now:      time.Now,
	}
	l.writeWG.Add(1)
	go l.run()
	return l
}

// run 是唯一的写 goroutine，收到关闭信号后把队列中剩余的日志写完再退出
// run is the single writer goroutine, and it drains the remaining records after a close signal before it returns
func (l *Logger) run() {
	defer l.writeWG.Done()
	for {
		select {
		case e := <-l.ch:
			l.write(e)
		case <-l.done:
			for {
				select {
				case e := <-l.ch:
					l.write(e)
				default:
					return
				}
			}
		}
	}
}

// write 输出一条日志，并在本批存在丢弃时先补一条提示
// write emits one record and first adds a notice when this batch saw dropped ones
func (l *Logger) write(e entry) {
	if e.barrier != nil {
		close(e.barrier)
		return
	}

	l.mu.Lock()
	out, now := l.out, l.now
	l.mu.Unlock()

	if dropped := l.dropped.Swap(0); dropped > 0 {
		notice := i18n.T(KeyQueueFull, dropped)
		_, _ = io.WriteString(out, l.render(LevelWarn, "", notice, now()))
	}
	_, _ = io.WriteString(out, l.render(e.level, e.file, e.msg, now()))
}

// render 生成完整的一行，格式与日志库在 main.initLogger 配置下的输出一致：
// 级别标签 + 空格 + 年/月/日 时:分:秒 + 空格 +（debug 时 调用点 + 空格）+ 消息 + 换行
//
// render builds one complete line in the format the logging library produces under the configuration of main.initLogger:
// level tag + space + date, space, time + space + (call site and a space at the debug level) + message + newline
func (l *Logger) render(level Level, file, msg string, t time.Time) string {
	var b strings.Builder
	b.Grow(len(msg) + 48)

	b.WriteString(levelTag(level))
	b.WriteByte(' ')
	year, month, day := t.Date()
	hour, minute, second := t.Clock()
	fmt.Fprintf(&b, "%04d/%02d/%02d %02d:%02d:%02d", year, month, day, hour, minute, second)
	b.WriteByte(' ')
	if file != "" {
		b.WriteString(file)
		b.WriteByte(' ')
	}
	b.WriteString(msg)
	b.WriteByte('\n')
	return b.String()
}

// callerSite 返回 skip 层调用者的短文件名与行号，形如 file.go:123
// callerSite returns the short file name and line of the caller skip frames up, shaped like file.go:123
func callerSite(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line)
}

// output 是各导出方法输出固定文案时的共同实现
// skip 是相对 output 的调用层数：1 为导出方法本身，2 为导出方法的调用者
// 低于当前级别的调用在渲染之前返回，因此被过滤的日志不会产生消息字符串
// 调用点在此处直接采集后再交给 enqueue，不在中间再插入一层，使 skip 的含义与异步化之前保持一致
//
// output is the shared implementation behind the exported methods that log fixed text
// skip counts frames from output: 1 is the exported method itself and 2 is the caller of that method
// A call below the current level returns before rendering, so a filtered call builds no message string
// The call site is collected right here and then handed to enqueue, with no extra frame in between, which keeps the meaning of skip the same as it was before the asynchronous rewrite
func (l *Logger) output(level Level, skip int, args ...any) {
	if l.disabled || level < l.level {
		return
	}
	var file string
	if l.withFile {
		file = callerSite(skip + 1)
	}
	l.enqueue(level, file, fmt.Sprint(args...))
}

// outputKey 是各导出方法输出目录消息时的共同实现
// 取文案与格式化都发生在级别过滤之后，因此被过滤的调用不查目录也不做格式化
// skip 的含义与 output 相同
//
// outputKey is the shared implementation behind the exported methods that log catalogue messages
// Both the lookup and the formatting happen after the level check, so a filtered call touches no catalogue entry and formats nothing
// skip carries the same meaning as in output
func (l *Logger) outputKey(level Level, skip int, key i18n.Key, args ...any) {
	if l.disabled || level < l.level {
		return
	}
	var file string
	if l.withFile {
		file = callerSite(skip + 1)
	}
	l.enqueue(level, file, i18n.T(key, args...))
}

// enqueue 把已渲染好的日志投进队列
// 队列满时丢弃并计数，不阻塞调用方
//
// enqueue queues an already rendered record
// A full queue drops the record and counts it instead of blocking the caller
func (l *Logger) enqueue(level Level, file, msg string) {
	select {
	case l.ch <- entry{level: level, file: file, msg: msg}:
	default:
		l.dropped.Add(1)
	}
}

// Flush 等待队列中已排队的日志写出，最长等待 flushTimeout
// 任何一条新日志都可能排在这条屏障之前，因此返回时队列中较早的记录已经写出
//
// Flush waits until the records already queued are written, for at most flushTimeout
// Any later record queues behind this barrier, so the earlier records are written by the time it returns
func (l *Logger) Flush() {
	if l.disabled {
		return
	}

	barrier := make(chan struct{})
	timer := time.NewTimer(flushTimeout)
	defer timer.Stop()

	select {
	case l.ch <- entry{barrier: barrier}:
	case <-timer.C:
		return
	}

	select {
	case <-barrier:
	case <-timer.C:
	}
}

// Dropped 返回因队列满而丢弃的日志条数，供观测与测试使用
// Dropped returns how many records a full queue discarded, for observation and tests
func (l *Logger) Dropped() int64 {
	return l.dropped.Load()
}

// QueueLen 返回队列当前的待写条数
// QueueLen returns how many records currently wait in the queue
func (l *Logger) QueueLen() int {
	return len(l.ch)
}

// SetOutput 更换输出目标，仅供测试注入缓冲
// SetOutput replaces the output target and exists for tests that inject a buffer
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	l.out = w
	l.mu.Unlock()
}

// SetClock 更换时钟，仅供测试固定时间戳
// SetClock replaces the clock and exists for tests that pin the timestamp
func (l *Logger) SetClock(now func() time.Time) {
	l.mu.Lock()
	l.now = now
	l.mu.Unlock()
}

// Close 冲刷队列并停止写 goroutine，可重复调用
// Close drains the queue and stops the writer goroutine, and may be called more than once
func (l *Logger) Close() {
	l.closeOnce.Do(func() {
		close(l.done)
		l.writeWG.Wait()
	})
}

// std 是包级默认日志器，供各包直接调用
// std is the package-level default logger that other packages call directly
var std = newLogger(LevelInfo, false, os.Stdout)

// Init 按配置初始化默认日志器
// 级别无法识别时回落到 info；disable 为真时所有日志调用立即返回
//
// Init initialises the default logger from the configuration
// An unrecognised level falls back to info, and a true disable makes every logging call return immediately
func Init(level string, disable bool) {
	parsed, ok := ParseLevel(level)
	if !ok {
		parsed = LevelInfo
	}
	std.level = parsed
	std.withFile = parsed == LevelDebug
	std.disabled = disable
}

// Debug 输出 debug 级别日志
// Debug logs at the debug level
func (l *Logger) Debug(args ...any) { l.output(LevelDebug, 2, args...) }

// Info 输出 info 级别日志
// Info logs at the info level
func (l *Logger) Info(args ...any) { l.output(LevelInfo, 2, args...) }

// Warn 输出 warn 级别日志
// Warn logs at the warn level
func (l *Logger) Warn(args ...any) { l.output(LevelWarn, 2, args...) }

// Error 输出 error 级别日志
// Error logs at the error level
func (l *Logger) Error(args ...any) { l.output(LevelError, 2, args...) }

// Debugf 以 debug 级别输出一条目录消息，key 为消息键，args 交给 i18n 填充
// Debugf logs one catalogue message at the debug level, where key is the message key and the rest is handed to i18n
func (l *Logger) Debugf(key i18n.Key, args ...any) { l.outputKey(LevelDebug, 2, key, args...) }

// Infof 以 info 级别输出一条目录消息，key 为消息键，args 交给 i18n 填充
// Infof logs one catalogue message at the info level, where key is the message key and the rest is handed to i18n
func (l *Logger) Infof(key i18n.Key, args ...any) { l.outputKey(LevelInfo, 2, key, args...) }

// Warnf 以 warn 级别输出一条目录消息，key 为消息键，args 交给 i18n 填充
// Warnf logs one catalogue message at the warn level, where key is the message key and the rest is handed to i18n
func (l *Logger) Warnf(key i18n.Key, args ...any) { l.outputKey(LevelWarn, 2, key, args...) }

// Errorf 以 error 级别输出一条目录消息，key 为消息键，args 交给 i18n 填充
// Errorf logs one catalogue message at the error level, where key is the message key and the rest is handed to i18n
func (l *Logger) Errorf(key i18n.Key, args ...any) { l.outputKey(LevelError, 2, key, args...) }

// 下面八个包级函数把级别与 skip 直接交给内部方法，不经过 Logger 上的同名方法
// 若这里写成 std.Info(args...)，调用链会多出 (*Logger).Info 一层，而 output 按 skip=2 采集调用点，采到的就是本文件里的包装函数
// 那样 debug 行会显示 logx.go 而不是真实调用方，因此 skip 保持 2：0 为 callerSite、1 为 output、2 为本文件里的包级函数、3 为真实调用方
//
// The eight package-level functions below hand the level and skip straight to the internal methods instead of going through the same-named methods on Logger
// Writing std.Info(args...) here would add a (*Logger).Info frame, while output collects the call site with skip=2 and would therefore collect the wrapper in this file
// A debug line would then name logx.go rather than the real caller, so the skip stays 2: 0 is callerSite, 1 is output, 2 is the package-level function in this file and 3 is the real caller

// Debug 输出 debug 级别日志
// Debug logs at the debug level
func Debug(args ...any) { std.output(LevelDebug, 2, args...) }

// Info 输出 info 级别日志
// Info logs at the info level
func Info(args ...any) { std.output(LevelInfo, 2, args...) }

// Warn 输出 warn 级别日志
// Warn logs at the warn level
func Warn(args ...any) { std.output(LevelWarn, 2, args...) }

// Error 输出 error 级别日志
// Error logs at the error level
func Error(args ...any) { std.output(LevelError, 2, args...) }

// Debugf 以 debug 级别输出一条目录消息
// Debugf logs one catalogue message at the debug level
func Debugf(key i18n.Key, args ...any) { std.outputKey(LevelDebug, 2, key, args...) }

// Infof 以 info 级别输出一条目录消息
// Infof logs one catalogue message at the info level
func Infof(key i18n.Key, args ...any) { std.outputKey(LevelInfo, 2, key, args...) }

// Warnf 以 warn 级别输出一条目录消息
// Warnf logs one catalogue message at the warn level
func Warnf(key i18n.Key, args ...any) { std.outputKey(LevelWarn, 2, key, args...) }

// Errorf 以 error 级别输出一条目录消息
// Errorf logs one catalogue message at the error level
func Errorf(key i18n.Key, args ...any) { std.outputKey(LevelError, 2, key, args...) }

// Flush 冲刷默认日志器的队列
// Flush drains the queue of the default logger
func Flush() { std.Flush() }

// Dropped 返回默认日志器丢弃的条数
// Dropped returns how many records the default logger discarded
func Dropped() int64 { return std.Dropped() }

// Close 关闭默认日志器，供测试或进程收尾使用
// Close shuts the default logger down, for tests or process teardown
func Close() { std.Close() }
