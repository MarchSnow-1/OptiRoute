// 本文件是 OptiRoute 可执行程序的入口，按 self.role 把同一个二进制分派为 center、edge、client、server 四种运行形态
// 启动顺序固定为确定语言 → 加载配置 → 校验配置 → 初始化日志 → 注册系统信号 → 启动角色服务，其中配置加载失败、配置校验失败与未知角色这三条路径都在启动服务之前以退出码 1 结束，避免带着非法配置进入数据面
// 语言先由命令行 --lang 确定，配置读入后再按「命令行优先于 self.lang」取生效标签，因此配置加载失败的文案使用命令行选定的语言，其后的全部文本使用生效语言
// 标签不受支持时回退到 en-US 并向标准错误打印一条提示，使语言写错不会挡住启动
// 日志由 logx 输出，退出前先冲刷队列，使已排队的日志不会因进程结束而丢失
// version 由构建期注入，并作为参数传给 edge 与 client/server 两个 agent，供注册与上报时标记本二进制版本
//
// This file is the entry point of the OptiRoute executable and dispatches the same binary into the center, edge, client and server runtime forms by self.role
// The start-up order is fixed: settle the language, load the configuration, validate it, initialise logging, register system signals and start the role service, and the three paths of a failed load, a failed validation and an unknown role all end with exit code 1 before the service starts, which keeps an invalid configuration out of the data plane
// The language is settled first from the command-line --lang, and once the configuration is read the effective tag follows "the command line outranks self.lang", so a failed load reports in the command-line language while every later piece of text uses the effective one
// An unsupported tag falls back to en-US and prints one notice to standard error, so a mistyped language cannot block start-up
// Logging goes through logx and the queue is flushed before exit, so records already queued are not lost when the process ends
// version is injected at build time and passed to edge and to the client and server agents, marking the binary version during registration and reporting
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/MarchSnow-1/OptiRoute/agent"
	"github.com/MarchSnow-1/OptiRoute/center"
	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/edge"
	"github.com/MarchSnow-1/OptiRoute/i18n"
	"github.com/MarchSnow-1/OptiRoute/logx"
)

// version 是构建期注入的版本号，直接 go build 时的取值为 dev
// release.yml 用 -ldflags "-X main.version=<tag 名>" 注入触发构建的 tag（形如 v1.2.3），dev.yml 注入 git describe 计算出的预发布版本（形如 v1.2.3-beta4-gabcdef 或 0.0.0-beta0-gabcdef）
//
// version is the version string injected at build time and stays dev for a plain go build
// release.yml injects the tag that triggered the build, such as v1.2.3, through -ldflags "-X main.version=<tag name>", while dev.yml injects the pre-release version computed from git describe, such as v1.2.3-beta4-gabcdef or 0.0.0-beta0-gabcdef
var version = "dev"

// resolveLang 把生效语言标签解析为受支持的语言，并返回需要打印的提示
// 标签受支持时提示为空串；不受支持时返回 Default 与一条提示，使语言写错只影响输出语言而不影响启动
// 提示按 Default 渲染而不随当前语言变化，因此取值与调用顺序无关：标签不受支持时生效语言本来就是 Default
//
// resolveLang resolves the effective language tag into a supported language and returns a notice to print
// The notice is an empty string for a supported tag, while an unsupported one yields Default plus a notice, so a mistyped language only changes the output language without blocking start-up
// The notice is rendered in Default rather than in the current language, which makes its value independent of call order: an unsupported tag makes Default the effective language anyway
func resolveLang(tag, source string) (i18n.Lang, string) {
	if lang, ok := i18n.Parse(tag); ok {
		return lang, ""
	}
	return i18n.Default(), i18n.Text(i18n.Default(), KeyLangUnsupported, source, tag)
}

// main 串起语言确定、配置加载、校验、日志初始化、信号监听与服务启动
// main wires up language resolution, configuration loading, validation, logging initialisation, signal handling and service start-up
func main() {
	// 1. 配置尚未读入，先按命令行 --lang 定语言，使配置加载失败的文案也能用同一种语言输出
	// 命令行取值此时未与配置文件合并，但回退判定用的是同一个 i18n.Parse，因此这里的提示与合并之后的结果一致，不会重复
	// 提示必须在此打印：配置加载失败会直接以退出码 1 结束，等到合并之后再判定就无法提示，与文档承诺的「标签写错会出现在输出里」不符
	//
	// 1. The configuration is not read yet, so the language comes from the command-line --lang first, which lets a failed load report in that same language
	// The command-line value is not merged with the configuration file yet, but the fallback call uses the same i18n.Parse, so the notice printed here matches the outcome after the merge and is not repeated
	// The notice has to be printed here: a failed load ends the process with exit code 1, and judging the fallback only after the merge would leave that path silent, which contradicts the documented promise that a mistyped tag shows up in the output
	cliTag, cliSet := config.CLILang()
	cliLang := i18n.Default()
	cliAccepted := false
	if cliSet {
		cliLang, cliAccepted = i18n.Parse(cliTag)
		i18n.Set(cliLang)
		if !cliAccepted {
			fmt.Fprintln(os.Stderr, i18n.Text(i18n.Default(), KeyLangUnsupported, "--lang", cliTag))
		}
	}

	// 2. 加载配置，命令行参数优先于 config.json，取 os.Args 中第一个带 --config- 前缀的参数
	// 2. Load the configuration with command-line arguments taking precedence over config.json, taking the first argument in os.Args that carries the --config- prefix
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.T(KeyConfigLoadFailed, err))
		os.Exit(1)
	}

	// 3. 取生效语言：命令行给出则以命令行为准，否则用 self.lang（defaults 已填默认值）
	// 命令行已经生效，这里只处理它未给出时的来源；语言在配置校验之前确定，使校验失败的文案也使用选定的语言
	//
	// 3. Take the effective language: the command line when it gives one, otherwise self.lang, which defaults() has already filled
	// The command line is already in force, so only the case where it was absent is handled here, and the language is settled before validation so that a validation failure reports in the selected language too
	if !cliSet {
		lang, notice := resolveLang(cfg.Self.Lang, "self.lang")
		i18n.Set(lang)
		if notice != "" {
			fmt.Fprintln(os.Stderr, notice)
		}
	}

	// 4. 校验配置
	// 4. Validate the configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", i18n.T(KeyConfigValidateFailed, err))
		os.Exit(1)
	}

	// 5. 初始化日志，disable_log 为真时所有日志调用立即返回
	// 5. Initialise logging, and a true disable_log makes every logging call return immediately
	logx.Init(cfg.Self.LogLevel, cfg.Self.DisableLog)
	logx.Infof(KeyStarted, version, cfg.Self.Role)

	// 6. 监听 SIGINT 与 SIGTERM，收到信号后取消 ctx，由各角色在自己的循环里退出
	// 6. Listen for SIGINT and SIGTERM and cancel ctx on arrival, leaving each role to exit inside its own loop
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		logx.Infof(KeyShutdownSignal)
		cancel()
	}()

	// 7. 按角色启动对应服务，四个分支各自把启动错误回填给 runErr
	// 7. Start the service for the role, with each of the four branches reporting its start error back through runErr
	var runErr error
	switch cfg.Self.Role {
	case config.RoleCenter:
		s := center.New(cfg)
		runErr = s.Start(ctx)

	case config.RoleEdge:
		n := edge.NewNode(cfg, version)
		runErr = n.Start(ctx)

	case config.RoleClient:
		a := agent.NewClientAgent(cfg, version)
		runErr = a.Start(ctx)

	case config.RoleServer:
		a := agent.NewServerAgent(cfg, version)
		runErr = a.Start(ctx)

	default:
		fmt.Fprintf(os.Stderr, "%s\n", i18n.T(KeyRoleUnknown, cfg.Self.Role))
		logx.Flush()
		os.Exit(1)
	}

	if runErr != nil {
		logx.Errorf(KeyServiceExited, runErr)
		logx.Flush()
		os.Exit(1)
	}

	// 正常退出路径也要冲刷，使队列中剩余的日志写完
	// The normal exit path flushes as well, so the records left in the queue are written
	logx.Flush()
}
