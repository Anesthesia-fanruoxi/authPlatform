package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"authplatform/common"
	"authplatform/model"
	"gorm.io/gorm"
)

// ListUsers 用户列表（超级管理员不展示；支持 keyword / category 筛选）。
func (s *Server) ListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	users, err := s.Users.List(r.Context(), q.Get("keyword"), true, q.Get("category"))
	if err != nil {
		s.internalError(w, err)
		return
	}
	safe := make([]map[string]any, 0, len(users))
	for _, u := range users {
		safe = append(safe, u.SafeUser())
	}
	OK(w, map[string]any{"users": safe})
}

// CreateUser 创建用户（用户名唯一，密码策略统一校验；可选手机号/邮箱）。
func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Nickname string `json:"nickname"`
		Phone    string `json:"phone"`
		Email    string `json:"email"`
		// 可选用户分类（未配置分类时忽略）
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	policy := s.Settings.GetPasswordPolicy(r.Context())
	if err := common.ValidatePasswordWithPolicy(req.Password, policy); err != nil {
		Fail(w, CodeBadParam, err.Error())
		return
	}
	hash, err := common.HashPassword(req.Password)
	if err != nil {
		s.internalError(w, err)
		return
	}
	uid, err := common.NewUID()
	if err != nil {
		s.internalError(w, err)
		return
	}
	u := &model.User{
		UID:          uid,
		Username:     req.Username,
		PasswordHash: hash,
		Nickname:     req.Nickname,
		Phone:        emptyToNil(req.Phone),
		Email:        emptyToNil(req.Email),
		Category:     req.Category,
		Status:       1,
	}
	if err := s.Users.Create(r.Context(), u); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			Fail(w, CodeUserExists, "用户名、手机号或邮箱已存在")
			return
		}
		s.internalError(w, err)
		return
	}
	OK(w, u.SafeUser())
}

// UpdateUser 更新用户（昵称/手机号/邮箱/启用状态）。
func (s *Server) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	var req struct {
		Nickname *string `json:"nickname"`
		Phone    *string `json:"phone"`
		Email    *string `json:"email"`
		Status   *int    `json:"status"`
		Category *string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	updates := map[string]any{}
	if req.Nickname != nil {
		updates["nickname"] = *req.Nickname
	}
	if req.Phone != nil {
		updates["phone"] = emptyToNilExpr(*req.Phone)
	}
	if req.Email != nil {
		updates["email"] = emptyToNilExpr(*req.Email)
	}
	if req.Status != nil {
		if id == currentUserID(r) && *req.Status != 1 {
			Fail(w, CodeBadParam, "不能禁用当前登录账号")
			return
		}
		updates["status"] = *req.Status
	}
	if req.Category != nil {
		updates["category"] = *req.Category
	}
	if len(updates) == 0 {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	if err := s.Users.Update(r.Context(), id, updates); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			Fail(w, CodeUserExists, "手机号或邮箱已被其他用户使用")
			return
		}
		s.internalError(w, err)
		return
	}
	u, err := s.Users.GetByID(r.Context(), id)
	if err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, u.SafeUser())
}

// DeleteUser 删除用户（级联清理授权由 grant store 处理，见 DeleteUserGrants）。
func (s *Server) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	if id == currentUserID(r) {
		Fail(w, CodeBadParam, "不能删除当前登录账号")
		return
	}
	if err := s.Grants.DeleteByUser(r.Context(), id); err != nil {
		s.internalError(w, err)
		return
	}
	if err := s.Users.Delete(r.Context(), id); err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, nil)
}

// ResetPassword 重置用户密码（管理侧）。
func (s *Server) ResetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	var req struct {
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NewPassword == "" {
		Fail(w, CodeBadParam, "参数错误")
		return
	}
	if err := common.ValidatePassword(req.NewPassword); err != nil {
		Fail(w, CodeBadParam, err.Error())
		return
	}
	hash, err := common.HashPassword(req.NewPassword)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if err := s.Users.Update(r.Context(), id, map[string]any{"password_hash": hash}); err != nil {
		s.internalError(w, err)
		return
	}
	OK(w, nil)
}

// pathID 解析路由路径参数 {id}。
func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// emptyToNil 空串转 nil（手机号/邮箱为空时不落库，避免与唯一索引冲突）。
func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// emptyToNilExpr 空串转 SQL NULL 表达式（更新时清空字段）。
func emptyToNilExpr(s string) any {
	if s == "" {
		return gorm.Expr("NULL")
	}
	return s
}

// currentUserID 返回当前登录管理员 ID（由 adminAuth 中间件写入）。
func currentUserID(r *http.Request) int64 {
	id, _ := r.Context().Value(CtxKeyUserID).(int64)
	return id
}
