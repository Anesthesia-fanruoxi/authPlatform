package common

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"sync"
	"time"
)

// 验证码：内存存储（dev 模式）。发送器暂不实现（后续接入短信/邮件服务商时替换发码环节）。
// 生成后由调用方决定是否回传（当前 dev 模式在响应中返回 code 并打印日志，便于联调）。
const (
	verCodeTTL      = 5 * time.Minute
	verCodeMaxTry   = 5 // 校验尝试次数上限，超限作废
	verCodeDigits   = 6
)

type verCodeInfo struct {
	code      string
	expiresAt time.Time
	attempts  int
}

// VerCodeStore 验证码内存存储（进程内有效；多实例部署需替换为 Redis 等共享存储）。
type VerCodeStore struct {
	mu   sync.Mutex
	data map[string]*verCodeInfo
}

func NewVerCodeStore() *VerCodeStore {
	return &VerCodeStore{data: make(map[string]*verCodeInfo)}
}

// key = method + "|" + identifier（如 phone_code|13800138000）
func vcKey(method, identifier string) string { return method + "|" + identifier }

// Generate 生成 6 位数字验证码并存储（覆盖旧码）。
func (s *VerCodeStore) Generate(method, identifier string) (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate vercode: %w", err)
	}
	code := fmt.Sprintf("%06d", (int(b[0])<<24|int(b[1])<<16|int(b[2])<<8|int(b[3]))%1000000)
	if code == "" || len(code) != verCodeDigits {
		code = "000000"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[vcKey(method, identifier)] = &verCodeInfo{code: code, expiresAt: time.Now().Add(verCodeTTL)}
	return code, nil
}

// Verify 校验验证码：成功即作废；失败累计次数，超限作废。
func (s *VerCodeStore) Verify(method, identifier, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, ok := s.data[vcKey(method, identifier)]
	if !ok {
		return false
	}
	if time.Now().After(info.expiresAt) {
		delete(s.data, vcKey(method, identifier))
		return false
	}
	if subtle.ConstantTimeCompare([]byte(info.code), []byte(code)) != 1 {
		info.attempts++
		if info.attempts >= verCodeMaxTry {
			delete(s.data, vcKey(method, identifier))
		}
		return false
	}
	delete(s.data, vcKey(method, identifier)) // 一次性使用
	return true
}
