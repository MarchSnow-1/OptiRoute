package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	logger "github.com/donnie4w/go-logger/logger"
)

type AuthManager struct {
	mu             sync.RWMutex
	current        []byte
	previous       []byte
	prevExpiry     time.Time
	currentVersion uint64 // 已接受的最新 secret 版本号；0 表示无版本（兼容旧中心）

	nonceMu    sync.Mutex
	usedNonces map[string]time.Time // nonce → 过期时间，防止 V2 Token 重放
}

func NewAuthManager() *AuthManager {
	return &AuthManager{usedNonces: make(map[string]time.Time)}
}

// ResetVersion 清空已接受的 secret 版本号。
// Center 进程重启后版本号会从 1 重新开始；Edge 在新 WS 连接建立时调用此方法，
// 避免把新中心实例的低版本 secret 误判为“旧推送”而丢弃。
func (a *AuthManager) ResetVersion() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.currentVersion = 0
}

func (a *AuthManager) UpdateSecret(newSecret []byte, tokenTTLS int, version uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 项目尚未进入 STABLE，不做旧版 Center 兼容。
	// version 必须严格递增，version=0 的旧推送与乱序推送一律忽略。
	if version == 0 || version <= a.currentVersion {
		logger.Warn("[UpdateSecret] 忽略非法或旧版本 secret push version:", version, " current:", a.currentVersion)
		return
	}
	a.currentVersion = version

	a.previous = a.current
	a.prevExpiry = time.Now().Add(time.Duration(tokenTTLS*2) * time.Second)
	a.current = newSecret
	logger.Debug("AuthManager secret 已更新 fingerprint:", secretFingerprint(newSecret))
}

const (
	routeTokenPrefix   = "v2:"
	routeTokenVersion  = 2
	routeTokenNonceLen = 16
	maxUsedNonces      = 4096
)

// routeTokenClaims 是 V2 Token 的签名内容。字段变化必须同步升级版本号。
type routeTokenClaims struct {
	V        int    `json:"v"`
	Target   string `json:"target"`
	Issuer   string `json:"issuer"`
	ClientIP string `json:"client_ip"`
	Nonce    string `json:"nonce"`
	TS       int64  `json:"ts"`
}

// routeTokenEnvelope 是实际下发给客户端的不透明 Token。
type routeTokenEnvelope struct {
	routeTokenClaims
	HMAC string `json:"hmac"`
}

// GenerateRouteToken 签发 V2 路由 Token。
// Token 绑定：目标 Edge UUID、签发 Edge UUID、客户端 IP、一次性 nonce、时间戳。
// HMAC 仍使用 Center 下发的 shared_secret，因此任意合法 Edge 仍可跨 Edge 签发；
// 但目标 Edge 现在可以校验来源并拒绝重放。
func (a *AuthManager) GenerateRouteToken(targetUUID, issuerUUID, clientIP string) (string, int64, error) {
	a.mu.RLock()
	secret := a.current
	a.mu.RUnlock()

	if len(secret) == 0 {
		return "", 0, fmt.Errorf("secret 尚未初始化")
	}
	if targetUUID == "" || issuerUUID == "" {
		return "", 0, fmt.Errorf("targetUUID、issuerUUID 均不能为空")
	}

	nonceBytes := make([]byte, routeTokenNonceLen)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", 0, fmt.Errorf("生成 nonce 失败: %w", err)
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
		return "", 0, fmt.Errorf("序列化 token claims 失败: %w", err)
	}

	envelope := routeTokenEnvelope{
		routeTokenClaims: claims,
		HMAC:             computeHMAC(secret, string(unsigned)),
	}
	signed, err := json.Marshal(envelope)
	if err != nil {
		return "", 0, fmt.Errorf("序列化 token 失败: %w", err)
	}

	return routeTokenPrefix + base64.RawURLEncoding.EncodeToString(signed), ts, nil
}

// VerifyRouteToken 校验 V2 路由 Token。
// clientTS 与 clientIP 来自目标 Edge 当前连接，必须和签发时一致。
func (a *AuthManager) VerifyRouteToken(token string, clientTS int64, selfUUID, clientIP string, tokenTTLS int) bool {
	if !strings.HasPrefix(token, routeTokenPrefix) {
		logger.Warn("[VerifyRouteToken] 未知 token 格式")
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, routeTokenPrefix))
	if err != nil {
		logger.Warn("[VerifyRouteToken] token base64 解码失败 err:", err)
		return false
	}

	var env routeTokenEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		logger.Warn("[VerifyRouteToken] token JSON 解析失败 err:", err)
		return false
	}
	if env.V != routeTokenVersion {
		logger.Warn("[VerifyRouteToken] 未知 token 版本:", env.V)
		return false
	}
	if env.Target != selfUUID {
		logger.Warn("[VerifyRouteToken] token 目标 UUID 不匹配")
		return false
	}
	if env.ClientIP != clientIP {
		logger.Warn("[VerifyRouteToken] token 客户端 IP 不匹配 token_ip:", env.ClientIP, " conn_ip:", clientIP)
		return false
	}
	if env.TS != clientTS {
		logger.Warn("[VerifyRouteToken] token 时间戳与首包不一致")
		return false
	}

	now := time.Now().Unix()
	if math.Abs(float64(now-env.TS)) > float64(tokenTTLS) {
		logger.Warn("[VerifyRouteToken] 时间窗口超期 now:", now, " ts:", env.TS, " ttl:", tokenTTLS)
		return false
	}

	unsigned, err := json.Marshal(env.routeTokenClaims)
	if err != nil {
		logger.Warn("[VerifyRouteToken] 序列化 claims 失败 err:", err)
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
		logger.Warn("[VerifyRouteToken] 验签失败 target:", env.Target, " issuer:", env.Issuer)
		return false
	}

	if !a.markNonceUsed(env.Nonce, time.Now().Add(time.Duration(tokenTTLS)*time.Second)) {
		logger.Warn("[VerifyRouteToken] nonce 重放拒绝 target:", env.Target, " nonce:", env.Nonce)
		return false
	}
	return true
}

func (a *AuthManager) markNonceUsed(nonce string, expireAt time.Time) bool {
	a.nonceMu.Lock()
	defer a.nonceMu.Unlock()

	now := time.Now()
	if len(a.usedNonces) >= maxUsedNonces {
		for n, exp := range a.usedNonces {
			if now.After(exp) {
				delete(a.usedNonces, n)
			}
		}
	}
	if exp, ok := a.usedNonces[nonce]; ok && now.Before(exp) {
		return false
	}
	a.usedNonces[nonce] = expireAt
	return true
}

// BuildCommSecretHeader 构造 Edge → Center WebSocket 鉴权头。
// 使用 hex 编码避免原始 comm_secret 中的特殊字符影响 Header 解析；
// 同时 secret 不再出现在 URL query 中，降低代理/网关日志泄露风险。
func BuildCommSecretHeader(secret string) string {
	return "Bearer " + hex.EncodeToString([]byte(secret))
}

// VerifyCommSecretHeader 校验 Edge → Center WebSocket 鉴权头。
// 使用常数时间比较，避免通过响应时间侧信道猜测密钥。
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

func computeHMAC(secret []byte, message string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// secretFingerprint 返回 secret hex 的前 8 字符，用于调试日志（不泄露完整密钥）
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
