package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sync"
	"time"

	logger "github.com/donnie4w/go-logger/logger"
)

type AuthManager struct {
	mu         sync.RWMutex
	current    []byte
	previous   []byte
	prevExpiry time.Time
}

func NewAuthManager() *AuthManager {
	return &AuthManager{}
}

func (a *AuthManager) UpdateSecret(newSecret []byte, tokenTTLS int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.previous = a.current
	a.prevExpiry = time.Now().Add(time.Duration(tokenTTLS*2) * time.Second)
	a.current = newSecret
	logger.Debug("AuthManager secret 已更新 fingerprint:", secretFingerprint(newSecret))
}

func (a *AuthManager) GenerateToken(targetUUID string) (string, int64, error) {
	a.mu.RLock()
	secret := a.current
	a.mu.RUnlock()

	if len(secret) == 0 {
		return "", 0, fmt.Errorf("secret 尚未初始化")
	}

	ts := time.Now().Unix()
	msg := fmt.Sprintf("%s:%d", targetUUID, ts)
	token := computeHMAC(secret, msg)
	return token, ts, nil
}

func (a *AuthManager) VerifyToken(token string, ts int64, selfUUID string, tokenTTLS int) bool {
	now := time.Now().Unix()
	if math.Abs(float64(now-ts)) > float64(tokenTTLS) {
		logger.Warn("[VerifyToken] 时间窗口超期 now:", now, " ts:", ts, " diff:", now-ts, " ttl:", tokenTTLS)
		return false
	}

	msg := fmt.Sprintf("%s:%d", selfUUID, ts)

	a.mu.RLock()
	current := a.current
	previous := a.previous
	prevExpiry := a.prevExpiry
	a.mu.RUnlock()

	// 用当前 secret 验签
	expectedCurrent := computeHMAC(current, msg)
	if hmac.Equal([]byte(expectedCurrent), []byte(token)) {
		return true
	}

	// 过渡期内用旧 secret 验签
	if len(previous) > 0 && time.Now().Before(prevExpiry) {
		expectedPrev := computeHMAC(previous, msg)
		if hmac.Equal([]byte(expectedPrev), []byte(token)) {
			return true
		}
	}

	logger.Warn("[VerifyToken] 验签失败 msg:", msg)
	return false
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
