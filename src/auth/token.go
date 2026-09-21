// 本文件实现 OptiRoute 的鉴权原语：shared_secret 的版本门控与轮换窗口、V2 路由 Token 的签发与校验、Edge → Center 控制面鉴权头的构造与校验，以及用于抗重放的 nonce 记录
// AuthManager 同时持有当前密钥与上一代密钥，UpdateSecret 只在推送版本号严格大于当前版本时替换密钥，并把被替换的密钥保留到双倍 Token TTL 的窗口内
// 在该窗口内，轮换前签发的 Token 仍能通过验签，窗口过后上一代密钥不再参与验签
// V2 Token 把 target UUID、issuer UUID、客户端 IP、nonce 与时间戳一起放进 HMAC 的签名内容，校验端逐项比对 target UUID、客户端 IP 与时间戳，再用 nonce 记录挡住同一 Token 的重复使用
// 签名覆盖 issuer 字段，但校验端不核对它是否为已知 Edge，只在校验失败时把它打进日志
//
// This file implements the authentication primitives of OptiRoute: version gating and the rotation window of the shared_secret, issuing and verification of V2 route Tokens, construction and verification of the Edge → Center control-plane auth header, and the nonce record used against replay
// AuthManager holds both the current secret and the previous one, UpdateSecret replaces the secret only when the pushed version is strictly greater than the current one, and it keeps the replaced secret for a window of twice the Token TTL
// Inside that window a Token signed before the rotation still verifies, and once the window has passed the previous secret takes no further part in verification
// A V2 Token puts the target UUID, issuer UUID, client IP, nonce and timestamp into the HMAC signed content, and the verifying side compares the target UUID, the client IP and the timestamp one by one before the nonce record blocks a repeated use of the same Token
// The signature covers the issuer field, while the verifying side does not check it against the known Edges and only logs it when verification fails
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/MarchSnow-1/OptiRoute/logx"
)

type AuthManager struct {
	mu             sync.RWMutex
	current        []byte
	previous       []byte
	prevExpiry     time.Time
	currentVersion uint64 // 已接受的最新 secret 版本号；0 表示本进程自启动或 ResetVersion() 之后还没接受过版本推送，而 version=0 的推送会被 UpdateSecret 当作非法丢弃 / the newest accepted secret version; 0 means this process has taken no version push since start-up or since ResetVersion(), and a push carrying version=0 is dropped by UpdateSecret as invalid

	nonceMu        sync.Mutex
	usedNonces     map[string]time.Time // nonce → 过期时间的映射，用于挡住 V2 Token 重放 / maps a nonce to its expiry to block V2 Token replay
	lastNoncePrune time.Time            // 上次清理过期 nonce 的时刻，用于按时间触发清理而不是只在表满时清理 / the instant of the previous sweep of expired nonces, so pruning is time driven instead of happening only on a full map
}

func NewAuthManager() *AuthManager {
	return &AuthManager{usedNonces: make(map[string]time.Time)}
}

// ResetVersion 清空已接受的 secret 版本号，使 currentVersion 回到 0
// Center 进程重启后版本号会从 1 重新开始，Edge 在建立新的 WS 连接时调用此方法，避免把新中心实例的低版本 secret 误判为“旧推送”而丢弃
//
// ResetVersion clears the accepted secret version and brings currentVersion back to 0
// A restarted Center process starts numbering again from 1, so Edge calls this when a new WS connection is established, which keeps it from misreading a low version of the new center instance as an old push and dropping it
func (a *AuthManager) ResetVersion() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.currentVersion = 0
}

// UpdateSecret 接受一次带版本号的 secret 推送，并据此轮换一次密钥
// 项目尚未进入 STABLE，不做旧版 Center 兼容，其中 version==0 的推送被当作非法，version<=currentVersion 的重复或乱序推送被当作旧推送
// 这两种情况都直接返回，current、previous 与 prevExpiry 都保持原值
// 版本号通过后先设置 previous 与 prevExpiry（当前时刻加双倍 tokenTTLS 秒），再写入新密钥，因此即使推送内容与当前密钥相同，也会走完一次轮换
//
// UpdateSecret accepts one versioned secret push and rotates the keys accordingly
// The project has not reached STABLE and keeps no compatibility with a legacy Center, where a push with version==0 counts as invalid and a repeated or out-of-order push with version<=currentVersion counts as an old push
// Both cases return right away with current, previous and prevExpiry left as they were
// An accepted version sets previous and prevExpiry (the current instant plus twice tokenTTLS seconds) first and stores the new secret afterwards, so a push carrying the same secret as the current one still completes a rotation
func (a *AuthManager) UpdateSecret(newSecret []byte, tokenTTLS int, version uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 项目尚未进入 STABLE，不做旧版 Center 兼容
	// version 需要严格递增，version=0 的旧推送与乱序推送一律忽略
	// The project has not reached STABLE, so no compatibility with a legacy Center is kept
	// The version needs to increase strictly, and a legacy push with version=0 or an out-of-order push is ignored
	if version == 0 || version <= a.currentVersion {
		logx.Warnf(KeySecretPushIgnored, version, a.currentVersion)
		return
	}
	a.currentVersion = version

	a.previous = a.current
	a.prevExpiry = time.Now().Add(time.Duration(tokenTTLS*2) * time.Second)
	a.current = newSecret
	logx.Debugf(KeySecretUpdated, secretFingerprint(newSecret))
}

// V2 路由 Token 的格式常量：前缀、版本号、nonce 字节数与记录的 nonce 条数上限
// Format constants of the V2 route Token: prefix, version, nonce byte length and the cap on recorded nonces
const (
	routeTokenPrefix   = "v2:"
	routeTokenVersion  = 2
	routeTokenNonceLen = 16
	maxUsedNonces      = 4096
	// noncePruneInterval 是两次过期 nonce 清理之间的最短间隔，与 maxUsedNonces 共同触发清理
	// 只按表长触发时，表长会在并发量低的时段持续增长而得不到清理
	// noncePruneInterval is the shortest interval between two sweeps of expired nonces and triggers a sweep together with maxUsedNonces
	// A sweep driven by the map length alone would leave the map growing through periods of low concurrency
	noncePruneInterval = 30 * time.Second
)

// routeTokenClaims 是 V2 Token 的签名内容，字段变化需要同步升级 routeTokenVersion
// routeTokenClaims is the signed content of a V2 Token, and a field change needs a matching routeTokenVersion bump
type routeTokenClaims struct {
	V        int    `json:"v"`
	Target   string `json:"target"`
	Issuer   string `json:"issuer"`
	ClientIP string `json:"client_ip"`
	Nonce    string `json:"nonce"`
	TS       int64  `json:"ts"`
}

// routeTokenEnvelope 是实际下发给客户端的不透明 Token，由 claims 与 HMAC 拼成
// routeTokenEnvelope is the opaque Token actually handed to the client, made of the claims plus the HMAC
type routeTokenEnvelope struct {
	routeTokenClaims
	HMAC string `json:"hmac"`
}

// GenerateRouteToken 签发 V2 路由 Token
// Token 绑定目标 Edge UUID、签发 Edge UUID、客户端 IP、一次性 nonce 与时间戳
// HMAC 使用 Center 下发的 shared_secret，因此持有同一 secret 的 Edge 之间可以互相签发
// 这里不校验 issuerUUID 是否为已注册 Edge，只校验它非空，是否放行由校验端决定
//
// GenerateRouteToken issues a V2 route Token
// The Token binds the target Edge UUID, the issuing Edge UUID, the client IP, a one-shot nonce and a timestamp
// The HMAC uses the shared_secret distributed by Center, so Edges holding the same secret can issue for each other
// This function does not check whether issuerUUID is a registered Edge and only requires it to be non-empty, while letting the Token through is up to the verifying side
func (a *AuthManager) GenerateRouteToken(targetUUID, issuerUUID, clientIP string) (string, int64, error) {
	a.mu.RLock()
	secret := a.current
	a.mu.RUnlock()

	if len(secret) == 0 {
		return "", 0, errf(KeyTokenSecretUninitialized)
	}
	if targetUUID == "" || issuerUUID == "" {
		return "", 0, errf(KeyTokenUUIDsRequired)
	}

	nonceBytes := make([]byte, routeTokenNonceLen)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", 0, errf(KeyTokenNonceGenerateFailed, err)
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)

	ts := time.Now().Unix()
	claims := routeTokenClaims{
		V:        routeTokenVersion,
		Target:   targetUUID,
		Issuer:   issuerUUID,
		ClientIP: clientIP,
		Nonce:    nonce,
		TS:       ts,
	}

	unsigned, err := json.Marshal(claims)
	if err != nil {
		return "", 0, errf(KeyTokenClaimsEncodeFailed, err)
	}

	envelope := routeTokenEnvelope{
		routeTokenClaims: claims,
		HMAC:             computeHMAC(secret, string(unsigned)),
	}
	signed, err := json.Marshal(envelope)
	if err != nil {
		return "", 0, errf(KeyTokenEnvelopeMarshalFailed, err)
	}

	return routeTokenPrefix + base64.RawURLEncoding.EncodeToString(signed), ts, nil
}

// VerifyRouteToken 校验 V2 路由 Token
// clientTS 与 clientIP 来自目标 Edge 当前连接，其中 clientTS 需要与签发时一致
// clientIP 携带当前连接的来源 IP；是否比对由 bindClientIP 决定，该开关只在验证方生效：edge.handleBootstrap 调用 GenerateRouteToken 时把客户端 IP 传入 claims，因此关闭绑定只是让本函数跳过比对，claims 中仍有该 IP
// 用独立的布尔量而不是空字符串表达「关闭绑定」，是为了让「连接没有可用 IP」与「不绑定」两种情况在调用点就区分开
// 依次校验前缀、base64、JSON、Token 版本、目标 UUID、可选的客户端 IP、时间戳与时间窗口，再用当前密钥或窗口内的上一代密钥验签，最后登记 nonce 挡住重放，任一环节不通过即返回 false
//
// VerifyRouteToken verifies a V2 route Token
// clientTS and clientIP come from the current connection of the target Edge, and clientTS needs to match the value seen at issuing time
// clientIP carries the source IP of the current connection while bindClientIP decides whether it is compared, and that switch acts on the verifying side only: edge.handleBootstrap passes the client IP into GenerateRouteToken, which writes it into the claims, so turning the binding off merely makes this function skip the comparison while the claims still carry the IP
// A separate boolean rather than an empty string expresses "binding off" so that the call site keeps "no IP available" apart from "not bound"
// It checks the prefix, the base64, the JSON, the Token version, the target UUID, the optional client IP, the timestamp and the time window in turn, then verifies the signature with the current secret or with the previous one inside its window, and finally records the nonce to block replay, returning false as soon as one step fails
func (a *AuthManager) VerifyRouteToken(token string, clientTS int64, selfUUID, clientIP string, bindClientIP bool, tokenTTLS int) bool {
	if !strings.HasPrefix(token, routeTokenPrefix) {
		logx.Warnf(KeyTokenFormatUnknown)
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, routeTokenPrefix))
	if err != nil {
		logx.Warnf(KeyTokenBase64Failed, err)
		return false
	}

	var env routeTokenEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		logx.Warnf(KeyTokenJSONFailed, err)
		return false
	}
	if env.V != routeTokenVersion {
		logx.Warnf(KeyTokenVersionUnknown, env.V)
		return false
	}
	if env.Target != selfUUID {
		logx.Warnf(KeyTokenTargetMismatch)
		return false
	}
	if bindClientIP && env.ClientIP != clientIP {
		logx.Warnf(KeyTokenIPMismatch, env.ClientIP, clientIP)
		return false
	}
	if env.TS != clientTS {
		logx.Warnf(KeyTokenTSMismatch)
		return false
	}

	now := time.Now().Unix()
	if math.Abs(float64(now-env.TS)) > float64(tokenTTLS) {
		logx.Warnf(KeyTokenWindowExpired, now, env.TS, tokenTTLS)
		return false
	}

	unsigned, err := json.Marshal(env.routeTokenClaims)
	if err != nil {
		logx.Warnf(KeyTokenClaimsMarshalFailed, err)
		return false
	}

	a.mu.RLock()
	current := a.current
	previous := a.previous
	prevExpiry := a.prevExpiry
	a.mu.RUnlock()

	expectedCurrent := computeHMAC(current, string(unsigned))
	valid := hmac.Equal([]byte(expectedCurrent), []byte(env.HMAC))
	if !valid && len(previous) > 0 && time.Now().Before(prevExpiry) {
		expectedPrev := computeHMAC(previous, string(unsigned))
		valid = hmac.Equal([]byte(expectedPrev), []byte(env.HMAC))
	}
	if !valid {
		logx.Warnf(KeyTokenSignatureFailed, env.Target, env.Issuer)
		return false
	}

	// nonce 的登记有效期以 Token 的签发时刻 ts 为基准，而不是以本次验签时刻为基准
	// Token 的可接受窗口是 |now-ts| <= ttl，两侧各留 ttl；若从本次验签时刻起算，当验证方时钟落后于签发方（now < ts）时 nonce 会在 ts 时刻先于 Token 窗口失效，ts 到 ts+ttl 之间的重放就不会被挡住
	// 以 max(now, ts) + ttl 为失效时刻，覆盖从当前时刻到窗口右边界 ts+ttl 的整个区间
	//
	// The recorded validity of a nonce is based on the Token issue instant ts rather than on this verification instant
	// A Token is accepted while |now-ts| <= ttl, which leaves ttl of room on either side; counting from this verification instant instead would let the nonce expire at ts when the verifying clock lags behind the issuing one (now < ts), and a replay between ts and ts+ttl would then go through
	// Taking max(now, ts) + ttl as the expiry covers the whole span from the current instant up to the right edge of the window at ts+ttl
	nonceBase := now
	if env.TS > nonceBase {
		nonceBase = env.TS
	}
	if !a.markNonceUsed(env.Nonce, time.Unix(nonceBase, 0).Add(time.Duration(tokenTTLS)*time.Second)) {
		logx.Warnf(KeyTokenNonceReplay, env.Target, env.Nonce)
		return false
	}
	return true
}

// markNonceUsed 登记一个 nonce，返回 true 表示本次是首次使用，返回 false 表示它在有效期内重复出现
// 已经过期但仍在 map 中的 nonce 会被重新登记，因此重放判定只覆盖各 nonce 自己的有效窗口
// 清理按时间触发：距上次清理达到 noncePruneInterval 或表长达到 maxUsedNonces 时遍历删除过期项，使表长不随累计连接数无界增长
//
// markNonceUsed records a nonce and returns true when this is its first use, or false when it appears again inside its validity window
// A nonce that has expired but is still in the map gets recorded again, so the replay check only covers each nonce's own validity window
// Pruning is time driven: the map is swept once noncePruneInterval has passed since the previous sweep or its length reaches maxUsedNonces, so the map cannot grow without bound with the number of connections
func (a *AuthManager) markNonceUsed(nonce string, expireAt time.Time) bool {
	a.nonceMu.Lock()
	defer a.nonceMu.Unlock()

	now := time.Now()
	if len(a.usedNonces) >= maxUsedNonces || now.Sub(a.lastNoncePrune) >= noncePruneInterval {
		a.pruneNoncesLocked(now)
	}
	if exp, ok := a.usedNonces[nonce]; ok && now.Before(exp) {
		return false
	}
	a.usedNonces[nonce] = expireAt
	return true
}

// pruneNoncesLocked 删除已过期的 nonce 并记录本次清理时刻，调用方需持有 a.nonceMu
// pruneNoncesLocked drops the expired nonces and records the sweep time, and expects the caller to hold a.nonceMu
func (a *AuthManager) pruneNoncesLocked(now time.Time) {
	for n, exp := range a.usedNonces {
		if now.After(exp) {
			delete(a.usedNonces, n)
		}
	}
	a.lastNoncePrune = now
}

// BuildCommSecretHeader 构造 Edge → Center WebSocket 鉴权头
// 使用 hex 编码避免原始 comm_secret 中的特殊字符影响 Header 解析，同时 secret 不再出现在 URL query 中，降低代理与网关日志泄露的风险
//
// BuildCommSecretHeader builds the Edge → Center WebSocket authentication header
// The hex encoding keeps special characters of the raw comm_secret from disturbing header parsing, and the secret stops appearing in the URL query, which lowers the risk of leaking it through proxy or gateway logs
func BuildCommSecretHeader(secret string) string {
	return "Bearer " + hex.EncodeToString([]byte(secret))
}

// VerifyCommSecretHeader 校验 Edge → Center WebSocket 鉴权头
// 使用常数时间比较，避免通过响应时间侧信道猜测密钥
//
// VerifyCommSecretHeader verifies the Edge → Center WebSocket authentication header
// It uses a constant-time comparison, which keeps the response time from acting as a side channel for guessing the secret
func VerifyCommSecretHeader(header, secret string) bool {
	if secret == "" || !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	supplied := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	suppliedSecret, err := hex.DecodeString(supplied)
	if err != nil || len(suppliedSecret) == 0 {
		return false
	}
	expected := []byte(secret)
	return len(suppliedSecret) == len(expected) &&
		subtle.ConstantTimeCompare(suppliedSecret, expected) == 1
}

// computeHMAC 返回 secret 对 message 的 HMAC-SHA256 十六进制串
// computeHMAC returns the hex HMAC-SHA256 of message under secret
func computeHMAC(secret []byte, message string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// secretFingerprint 返回 secret 的 hex 前 8 字符，用于调试日志（不泄露完整密钥）
// hex 长度不超过 8 时返回完整 hex，secret 为空时返回 <empty>
//
// secretFingerprint returns the first 8 hex characters of the secret for debug logs, without exposing the whole key
// A hex of 8 characters or fewer is returned as is, and an empty secret yields <empty>
func secretFingerprint(secret []byte) string {
	if len(secret) == 0 {
		return "<empty>"
	}
	fp := hex.EncodeToString(secret)
	if len(fp) > 8 {
		return fp[:8] + "..."
	}
	return fp
}
