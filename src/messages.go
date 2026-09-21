// 本文件登记 main 包用到的消息键与其三种语言文案
// 覆盖启动与退出路径上的全部文本：配置加载或校验失败、未知角色、启动横幅、退出信号、服务异常退出，以及语言标签不受支持时的回退提示
// 配置加载失败发生在 self.lang 读到之前，因此该条只能用命令行 --lang 选定的语言输出
//
// This file registers the message keys the main package uses together with their text in three languages
// It covers every piece of text on the start-up and exit paths: a failed configuration load or validation, an unknown role, the start-up banner, the exit signal, an abnormal service exit and the fallback notice for an unsupported language tag
// A failed configuration load happens before self.lang can be read, so that one notice can only use the language the command-line --lang selected
package main

import "github.com/MarchSnow-1/OptiRoute/i18n"

const (
	// KeyConfigLoadFailed 是配置加载失败时的键，参数为底层错误
	// KeyConfigLoadFailed is the key used when the configuration cannot be loaded, whose argument is the underlying error
	KeyConfigLoadFailed i18n.Key = "main.config_load_failed"

	// KeyConfigValidateFailed 是配置校验失败时的键，参数为底层错误
	// KeyConfigValidateFailed is the key used when the configuration fails validation, whose argument is the underlying error
	KeyConfigValidateFailed i18n.Key = "main.config_validate_failed"

	// KeyRoleUnknown 是 self.role 不在四种角色之内时的键，参数为实际取值
	// KeyRoleUnknown is the key used when self.role names none of the four roles, whose argument is the value read
	KeyRoleUnknown i18n.Key = "main.role_unknown"

	// KeyStarted 是启动横幅的键，参数依次为构建期注入的版本号与运行角色
	// KeyStarted is the key of the start-up banner, with the build-time version and the running role as arguments
	KeyStarted i18n.Key = "main.started"

	// KeyShutdownSignal 是收到退出信号时的键
	// KeyShutdownSignal is the key used when an exit signal arrives
	KeyShutdownSignal i18n.Key = "main.shutdown_signal"

	// KeyServiceExited 是角色服务异常退出时的键，参数为退出错误
	// KeyServiceExited is the key used when the role service exits with an error, whose argument is that error
	KeyServiceExited i18n.Key = "main.service_exited"

	// KeyLangUnsupported 是语言标签不受支持时的键，参数依次为标签来源与标签取值
	// 标签来源取 --lang 或 self.lang，使提示能直接指出该改哪一个
	//
	// KeyLangUnsupported is the key used when a language tag is not supported, with the source of the tag and its value as arguments
	// The source is either --lang or self.lang, so the notice states which one to change
	KeyLangUnsupported i18n.Key = "main.lang_unsupported"
)

// messages 保存本包的消息，键为 i18n.Key，值为三种语言下的文案
// messages holds this package's messages, keyed by i18n.Key with the text in three languages
var messages = map[i18n.Key]i18n.Entry{
	KeyConfigLoadFailed: i18n.New(
		"configuration load failed: %v",
		"配置加载失败: %v",
		"設定載入失敗: %v",
	),
	KeyConfigValidateFailed: i18n.New(
		"configuration validation failed: %v",
		"配置校验失败: %v",
		"設定校驗失敗: %v",
	),
	KeyRoleUnknown: i18n.New(
		"unknown role: %s",
		"未知角色: %s",
		"未知角色: %s",
	),
	KeyStarted: i18n.New(
		"OptiRoute v%s started role:%s",
		"OptiRoute v%s 启动 role:%s",
		"OptiRoute v%s 啟動 role:%s",
	),
	KeyShutdownSignal: i18n.New(
		"received an exit signal, shutting down...",
		"收到退出信号，正在关闭...",
		"收到結束訊號，正在關閉...",
	),
	KeyServiceExited: i18n.New(
		"service exited with an error err:%v",
		"服务异常退出 err:%v",
		"服務異常結束 err:%v",
	),
	KeyLangUnsupported: i18n.New(
		"%s=%s is not a supported language tag, falling back to en-US",
		"%s=%s 不是受支持的语言标签，回退到 en-US",
		"%s=%s 不是受支援的語言標籤，回退到 en-US",
	),
}

// init 登记本包的消息，使启动路径上的第一条日志之前目录已经就绪
// init registers this package's messages so that the catalogue is ready before the first log line of the start-up path
func init() {
	i18n.Register(messages)
}
