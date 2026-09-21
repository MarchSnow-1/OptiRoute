// 本文件是 OptiRoute 可执行程序的入口，按 self.role 把同一个二进制分派为 center、edge、client、server 四种运行形态
// 启动顺序固定为加载配置 → 校验配置 → 初始化日志 → 注册系统信号 → 启动角色服务，其中配置加载失败、配置校验失败与未知角色这三条路径都在启动服务之前以退出码 1 结束，避免带着非法配置进入数据面
// version 由构建期注入，并作为参数传给 edge 与 client/server 两个 agent，供注册与上报时标记本二进制版本
//
// This file is the entry point of the OptiRoute executable and dispatches the same binary into the center, edge, client and server runtime forms by self.role
// The start-up order is fixed: load the configuration, validate it, initialise logging, register system signals and start the role service, and the three paths of a failed load, a failed validation and an unknown role all end with exit code 1 before the service starts, which keeps an invalid configuration out of the data plane
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
	"github.com/donnie4w/go-logger/logger"
)

// version 是构建期注入的版本号，直接 go build 时的取值为 dev
// release.yml 用 -ldflags "-X main.version=<tag 名>" 注入触发构建的 tag（形如 v1.2.3），dev.yml 注入 git describe 计算出的预发布版本（形如 v1.2.3-beta4-gabcdef 或 0.0.0-beta0-gabcdef）
//
// version is the version string injected at build time and stays dev for a plain go build
// release.yml injects the tag that triggered the build, such as v1.2.3, through -ldflags "-X main.version=<tag name>", while dev.yml injects the pre-release version computed from git describe, such as v1.2.3-beta4-gabcdef or 0.0.0-beta0-gabcdef
var version = "dev"

// main 串起配置加载、校验、日志初始化、信号监听与服务启动
// main wires up configuration loading, validation, logging initialisation, signal handling and service start-up
func main() {
	// 1. 加载配置，命令行参数优先于 config.json，取 os.Args 中第一个带 --config- 前缀的参数
	// 1. Load the configuration with command-line arguments taking precedence over config.json, taking the first argument in os.Args that carries the --config- prefix
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置加载失败: %v\n", err)
		os.Exit(1)
	}

	// 2. 校验配置
	// 2. Validate the configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "配置校验失败: %v\n", err)
		os.Exit(1)
	}

	// 3. 初始化日志
	// 3. Initialise logging
	initLogger(cfg.Self.LogLevel)
	logger.Info("OptiRoute v", version, " 启动 role:", cfg.Self.Role)

	// 4. 监听 SIGINT 与 SIGTERM，收到信号后取消 ctx，由各角色在自己的循环里退出
	// 4. Listen for SIGINT and SIGTERM and cancel ctx on arrival, leaving each role to exit inside its own loop
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		logger.Info("收到退出信号，正在关闭...")
		cancel()
	}()

	// 5. 按角色启动对应服务，四个分支各自把启动错误回填给 runErr
	// 5. Start the service for the role, with each of the four branches reporting its start error back through runErr
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
		fmt.Fprintf(os.Stderr, "未知角色: %s\n", cfg.Self.Role)
		os.Exit(1)
	}

	if runErr != nil {
		logger.Error("服务异常退出 err:", runErr)
		os.Exit(1)
	}
}

// initLogger 按 log_level 配置控制台的日志级别与格式
// 只有 debug 会额外打开文件名格式化（FORMAT_SHORTFILENAME 并把 file 放进模板），未知或空 level 落到 info，因为 logLevel 的初始值就是 LEVEL_INFO 且 switch 没有 default 分支
//
// initLogger sets the console log level and format from the log_level setting
// Only debug turns on file-name formatting (FORMAT_SHORTFILENAME plus file in the template), and an unknown or empty level falls back to info, because logLevel starts as LEVEL_INFO and the switch has no default arm
func initLogger(level string) {
	logLevel := logger.LEVEL_INFO
	switch level {
	case "debug":
		logLevel = logger.LEVEL_DEBUG
	case "warn":
		logLevel = logger.LEVEL_WARN
	case "error":
		logLevel = logger.LEVEL_ERROR
	}

	levelFmt := func(level logger.LEVELTYPE) string {
		switch level {
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
	if level == "debug" {
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
