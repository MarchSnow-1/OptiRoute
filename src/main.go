package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/donnie4w/go-logger/logger"
	"github.com/MarchSnow-1/OptiRoute/agent"
	"github.com/MarchSnow-1/OptiRoute/center"
	"github.com/MarchSnow-1/OptiRoute/config"
	"github.com/MarchSnow-1/OptiRoute/edge"
)

// version 版本号，可通过 -ldflags "-X main.version=x.y.z" 在构建时注入
var version = "dev"

func main() {
	// 1. 加载配置（左序优先裁决：命令行 > config.json）
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "配置加载失败: %v\n", err)
		os.Exit(1)
	}

	// 2. 校验配置
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "配置校验失败: %v\n", err)
		os.Exit(1)
	}

	// 3. 初始化日志
	initLogger(cfg.Self.LogLevel)
	logger.Info("OptiRoute v", version, " 启动 role:", cfg.Self.Role)

	// 4. 监听系统信号，优雅退出
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		logger.Info("收到退出信号，正在关闭...")
		cancel()
	}()

	// 5. 启动对应服务
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
