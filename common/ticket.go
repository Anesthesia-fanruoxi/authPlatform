package common

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// 多步骤登录的临时票据（login_ticket）：
// 第一步验证通过后签发，服务端内存存储，短有效期；后续步骤携带 ticket 继续，
// 全部验证通过后签发最终 token 并销毁 ticket。
const (
	ticketTTL      = 5 * time.Minute
	ticketTokenLen = 32 // 字节
)

type LoginTicket struct {
	UserID      int64
	PlatformID  int64
	Methods     []string // 完整登录方式列表（按序）
	DoneMethods []string // 已完成的方式
	ExpiresAt   time.Time
}

// TicketStore login_ticket 内存存储（进程内有效）。
type TicketStore struct {
	mu   sync.Mutex
	data map[string]*LoginTicket
}

func NewTicketStore() *TicketStore {
	return &TicketStore{data: make(map[string]*LoginTicket)}
}

// Create 签发新票据，返回 ticket 字符串。
func (s *TicketStore) Create(userID, platformID int64, methods []string) (string, error) {
	b := make([]byte, ticketTokenLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tk := hex.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[tk] = &LoginTicket{
		UserID:     userID,
		PlatformID: platformID,
		Methods:    methods,
		ExpiresAt:  time.Now().Add(ticketTTL),
	}
	return tk, nil
}

// Get 获取票据（校验有效期，过期即删除）。
func (s *TicketStore) Get(tk string) (*LoginTicket, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.data[tk]
	if !ok {
		return nil, false
	}
	if time.Now().After(t.ExpiresAt) {
		delete(s.data, tk)
		return nil, false
	}
	return t, true
}

// MarkDone 记录一步已完成（用于推进多步骤流程）。
func (s *TicketStore) MarkDone(tk, method string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.data[tk]
	if !ok {
		return
	}
	for _, m := range t.DoneMethods {
		if m == method {
			return
		}
	}
	t.DoneMethods = append(t.DoneMethods, method)
}

// Delete 销毁票据（登录完成/失败清理）。
func (s *TicketStore) Delete(tk string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, tk)
}
