// 本文件声明 main 包用到的消息键，各语言的文案放在 i18n/locales 下的语言文件里
// 覆盖启动与退出路径上的全部文本：配置加载或校验失败、未知角色、启动横幅、退出信号、服务异常退出，以及语言标签不受支持时的回退提示
// 配置加载失败发生在 self.lang 读到之前，因此该条只能用命令行 --lang 选定的语言输出
//
// This file declares the message keys the main package uses, and the copy of each language lives in the language files under i18n/locales
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
