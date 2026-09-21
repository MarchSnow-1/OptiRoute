// 本文件登记 auth 包用到的消息键与其三种语言文案
// 键名按「包名.用途」命名，前缀 [UpdateSecret] 与 [VerifyRouteToken] 保留在文案里，使日志仍能直接指出产生它的函数
// 在 init 中一次性登记，登记完成后目录只被读取
//
// This file registers the message keys auth uses together with their text in three languages
// A key is named package.purpose, while the prefixes [UpdateSecret] and [VerifyRouteToken] stay inside the text so that a log line still names the function that produced it
// Registration happens once in init and the catalogue is only read afterwards
package auth

import "github.com/MarchSnow-1/OptiRoute/i18n"

const (
	// KeySecretPushIgnored 是密钥推送因版本号为 0 或不大于当前版本而被忽略时的键
	// 参数依次为推送版本号与当前版本号
	//
	// KeySecretPushIgnored is the key used when a secret push is ignored because its version is 0 or not greater than the current one
	// The arguments are the pushed version and the current version
	KeySecretPushIgnored i18n.Key = "auth.secret_push_ignored"

	// KeySecretUpdated 是密钥完成一次轮换后的键，参数为新密钥的指纹
	// KeySecretUpdated is the key used after a secret rotation, whose argument is the fingerprint of the new secret
	KeySecretUpdated i18n.Key = "auth.secret_updated"

	// KeyTokenFormatUnknown 是 Token 缺少 v2: 前缀时的键
	// KeyTokenFormatUnknown is the key used when a Token carries no v2: prefix
	KeyTokenFormatUnknown i18n.Key = "auth.token_format_unknown"

	// KeyTokenBase64Failed 是 Token 主体 base64 解码失败时的键，参数为解码错误
	// KeyTokenBase64Failed is the key used when the Token body fails base64 decoding, whose argument is the decoding error
	KeyTokenBase64Failed i18n.Key = "auth.token_base64_failed"

	// KeyTokenJSONFailed 是 Token 信封 JSON 解析失败时的键，参数为解析错误
	// KeyTokenJSONFailed is the key used when the Token envelope fails JSON decoding, whose argument is the decoding error
	KeyTokenJSONFailed i18n.Key = "auth.token_json_failed"

	// KeyTokenVersionUnknown 是 Token 版本号不是 v2 时的键，参数为实际版本号
	// KeyTokenVersionUnknown is the key used when the Token version is not v2, whose argument is the version read from the envelope
	KeyTokenVersionUnknown i18n.Key = "auth.token_version_unknown"

	// KeyTokenTargetMismatch 是 Token 目标 UUID 与本单位 UUID 不一致时的键
	// KeyTokenTargetMismatch is the key used when the target UUID of a Token differs from this node's UUID
	KeyTokenTargetMismatch i18n.Key = "auth.token_target_mismatch"

	// KeyTokenIPMismatch 是 Token 内客户端 IP 与当前连接来源 IP 不一致时的键
	// 参数依次为 Token 内 IP 与连接来源 IP
	//
	// KeyTokenIPMismatch is the key used when the client IP inside a Token differs from the source IP of the current connection
	// The arguments are the IP inside the Token and the source IP of the connection
	KeyTokenIPMismatch i18n.Key = "auth.token_ip_mismatch"

	// KeyTokenTSMismatch 是 Token 时间戳与首包时间戳不一致时的键
	// KeyTokenTSMismatch is the key used when the Token timestamp differs from the one seen on the first packet
	KeyTokenTSMismatch i18n.Key = "auth.token_ts_mismatch"

	// KeyTokenWindowExpired 是 Token 超出时间窗口时的键，参数依次为当前时刻、Token 时间戳与 TTL 秒数
	// KeyTokenWindowExpired is the key used when a Token falls outside its time window, with the current instant, the Token timestamp and the TTL in seconds as arguments
	KeyTokenWindowExpired i18n.Key = "auth.token_window_expired"

	// KeyTokenClaimsMarshalFailed 是重新序列化 claims 失败时的键，参数为序列化错误
	// KeyTokenClaimsMarshalFailed is the key used when re-serialising the claims fails, whose argument is the marshalling error
	KeyTokenClaimsMarshalFailed i18n.Key = "auth.token_claims_marshal_failed"

	// KeyTokenSignatureFailed 是验签失败时的键，参数依次为目标 UUID 与签发方 UUID
	// 签发方不被校验是否为已知 Edge，因此这里只把它打进日志供排查
	//
	// KeyTokenSignatureFailed is the key used when signature verification fails, with the target UUID and the issuer UUID as arguments
	// The issuer is not checked against the known Edges, so it is logged here for troubleshooting only
	KeyTokenSignatureFailed i18n.Key = "auth.token_signature_failed"

	// KeyTokenNonceReplay 是 nonce 在有效期内重复出现时的键，参数依次为目标 UUID 与 nonce
	// KeyTokenNonceReplay is the key used when a nonce appears again inside its validity window, with the target UUID and the nonce as arguments
	KeyTokenNonceReplay i18n.Key = "auth.token_nonce_replay"
)

// messages 保存本包的消息，键为 i18n.Key，值为三种语言下的文案
// messages holds this package's messages, keyed by i18n.Key with the text in three languages
var messages = map[i18n.Key]i18n.Entry{
	KeySecretPushIgnored: i18n.New(
		"[UpdateSecret] ignoring an invalid or older secret push version:%d current:%d",
		"[UpdateSecret] 忽略非法或旧版本 secret push version:%d current:%d",
		"[UpdateSecret] 忽略非法或舊版本 secret push version:%d current:%d",
	),
	KeySecretUpdated: i18n.New(
		"AuthManager secret updated fingerprint:%s",
		"AuthManager secret 已更新 fingerprint:%s",
		"AuthManager secret 已更新 fingerprint:%s",
	),
	KeyTokenFormatUnknown: i18n.New(
		"[VerifyRouteToken] unknown token format",
		"[VerifyRouteToken] 未知 token 格式",
		"[VerifyRouteToken] 未知 token 格式",
	),
	KeyTokenBase64Failed: i18n.New(
		"[VerifyRouteToken] token base64 decoding failed err:%v",
		"[VerifyRouteToken] token base64 解码失败 err:%v",
		"[VerifyRouteToken] token base64 解碼失敗 err:%v",
	),
	KeyTokenJSONFailed: i18n.New(
		"[VerifyRouteToken] token JSON decoding failed err:%v",
		"[VerifyRouteToken] token JSON 解析失败 err:%v",
		"[VerifyRouteToken] token JSON 解析失敗 err:%v",
	),
	KeyTokenVersionUnknown: i18n.New(
		"[VerifyRouteToken] unknown token version:%d",
		"[VerifyRouteToken] 未知 token 版本:%d",
		"[VerifyRouteToken] 未知 token 版本:%d",
	),
	KeyTokenTargetMismatch: i18n.New(
		"[VerifyRouteToken] token target UUID mismatch",
		"[VerifyRouteToken] token 目标 UUID 不匹配",
		"[VerifyRouteToken] token 目標 UUID 不匹配",
	),
	KeyTokenIPMismatch: i18n.New(
		"[VerifyRouteToken] token client IP mismatch token_ip:%s conn_ip:%s",
		"[VerifyRouteToken] token 客户端 IP 不匹配 token_ip:%s conn_ip:%s",
		"[VerifyRouteToken] token 用戶端 IP 不匹配 token_ip:%s conn_ip:%s",
	),
	KeyTokenTSMismatch: i18n.New(
		"[VerifyRouteToken] token timestamp differs from the first packet",
		"[VerifyRouteToken] token 时间戳与首包不一致",
		"[VerifyRouteToken] token 時間戳與首包不一致",
	),
	KeyTokenWindowExpired: i18n.New(
		"[VerifyRouteToken] time window exceeded now:%d ts:%d ttl:%d",
		"[VerifyRouteToken] 时间窗口超期 now:%d ts:%d ttl:%d",
		"[VerifyRouteToken] 時間窗口超期 now:%d ts:%d ttl:%d",
	),
	KeyTokenClaimsMarshalFailed: i18n.New(
		"[VerifyRouteToken] marshalling claims failed err:%v",
		"[VerifyRouteToken] 序列化 claims 失败 err:%v",
		"[VerifyRouteToken] 序列化 claims 失敗 err:%v",
	),
	KeyTokenSignatureFailed: i18n.New(
		"[VerifyRouteToken] signature verification failed target:%s issuer:%s",
		"[VerifyRouteToken] 验签失败 target:%s issuer:%s",
		"[VerifyRouteToken] 驗簽失敗 target:%s issuer:%s",
	),
	KeyTokenNonceReplay: i18n.New(
		"[VerifyRouteToken] nonce replay rejected target:%s nonce:%s",
		"[VerifyRouteToken] nonce 重放拒绝 target:%s nonce:%s",
		"[VerifyRouteToken] nonce 重放拒絕 target:%s nonce:%s",
	),
}

// init 登记本包的消息，使任何日志调用之前目录已经就绪
// init registers this package's messages so that the catalogue is ready before any logging call
func init() {
	i18n.Register(messages)
}
