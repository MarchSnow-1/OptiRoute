// 本文件声明 auth 包用到的消息键，各语言的文案放在 i18n/locales 下的语言文件里
// 键名按「包名.用途」命名，前缀 [UpdateSecret] 与 [VerifyRouteToken] 保留在文案里，使日志仍能直接指出产生它的函数
//
// This file declares the message keys auth uses, and the copy of each language lives in the language files under i18n/locales
// A key is named package.purpose, while the prefixes [UpdateSecret] and [VerifyRouteToken] stay inside the text so that a log line still names the function that produced it
package auth

import (
	"fmt"

	"github.com/MarchSnow-1/OptiRoute/i18n"
)

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

	// KeyTokenSecretUninitialized 是签发时当前密钥为空的键，不带参数
	// 该分支出现在 Center 尚未推送任何 secret 时，因此签发无从进行
	//
	// KeyTokenSecretUninitialized is the key used when the current secret is empty at issuing time, and it takes no argument
	// The branch is reached while Center has pushed no secret yet, which leaves nothing to sign with
	KeyTokenSecretUninitialized i18n.Key = "auth.token_secret_uninitialized"

	// KeyTokenUUIDsRequired 是签发时目标 UUID 或签发方 UUID 为空的键，不带参数
	// 两个 UUID 都必须非空，是否放行由校验端决定
	//
	// KeyTokenUUIDsRequired is the key used when the target UUID or the issuer UUID is empty at issuing time, and it takes no argument
	// Both UUIDs have to be non-empty, while letting the Token through stays the decision of the verifying side
	KeyTokenUUIDsRequired i18n.Key = "auth.token_uuids_required"

	// KeyTokenNonceGenerateFailed 是生成 nonce 失败时的键，参数为底层错误
	// KeyTokenNonceGenerateFailed is the key used when generating the nonce fails, whose argument is the underlying error
	KeyTokenNonceGenerateFailed i18n.Key = "auth.token_nonce_generate_failed"

	// KeyTokenClaimsEncodeFailed 是签发时序列化 claims 失败时的键，参数为序列化错误
	// 该键与校验路径上重新序列化 claims 的键分开：两处的文案与上下文不同
	//
	// KeyTokenClaimsEncodeFailed is the key used when marshalling the claims fails at issuing time, whose argument is the marshalling error
	// The key stays apart from the one used for re-serialising the claims on the verifying path, whose text and context differ
	KeyTokenClaimsEncodeFailed i18n.Key = "auth.token_claims_encode_failed"

	// KeyTokenEnvelopeMarshalFailed 是序列化 Token 信封失败时的键，参数为序列化错误
	// KeyTokenEnvelopeMarshalFailed is the key used when marshalling the Token envelope fails, whose argument is the marshalling error
	KeyTokenEnvelopeMarshalFailed i18n.Key = "auth.token_envelope_marshal_failed"
)

// errf 按消息键取当前语言的格式串并交给 fmt.Errorf
// 取的是未填充的格式串而不是已渲染的文本，使 %w 仍由 fmt.Errorf 处理，错误链得以保留
//
// errf takes the format string of a message key in the current language and hands it to fmt.Errorf
// The unfilled format string is used rather than rendered text, so a %w verb is still handled by fmt.Errorf and the error chain survives
func errf(key i18n.Key, args ...any) error {
	return fmt.Errorf(i18n.T(key), args...)
}
