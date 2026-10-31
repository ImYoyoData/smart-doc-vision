package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/security"
)

const mfaTicketTTL = 5 * time.Minute

type mfaTicket struct {
	UserID uint64 `json:"uid"`
	IP     string `json:"ip"`
}

// loginWithMFA authenticates an account with only its TOTP code. It is an
// opt-in convenience for users who already enabled two-factor authentication;
// password login remains the normal path.
func (s *Server) loginWithMFA(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Code     string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请输入账号和动态验证码")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Code = strings.TrimSpace(req.Code)
	if req.Username == "" {
		badRequest(c, "请输入账号")
		return
	}
	if !sixDigitCode(req.Code) {
		badRequest(c, "请输入 6 位动态验证码")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if s.redis == nil {
		fail(c, http.StatusServiceUnavailable, "DEPENDENCY_MISSING", "验证码服务不可用", nil)
		return
	}
	ip := c.ClientIP()
	if retry, code := s.loginLocked(ctx, req.Username, ip); code != "" {
		s.writeLoginBlocked(c, time.Until(retry), code)
		return
	}

	var user model.User
	lookupErr := s.db.Where("username = ?", req.Username).First(&user).Error
	secret := ""
	if lookupErr == nil && user.MFAEnabled {
		secret, _ = s.cipher.Decrypt(user.MFASecretEnc)
	}
	if lookupErr != nil || user.Status != "ACTIVE" || !user.MFAEnabled || secret == "" ||
		!security.ValidateTOTP(secret, req.Code, time.Now()) {
		blocked, retry := s.recordLoginFailure(ctx, req.Username, ip)
		if blocked {
			s.writeLoginBlocked(c, retry, "IP_BLOCKED")
			return
		}
		s.createAuditLogNoReq(nil, req.Username, "LOGIN_MFA_FAILED", "USER", req.Username, "FAILED",
			map[string]any{"ip": ip, "method": "totp"}, c.GetString("trace_id"))
		// Keep the response identical for unknown accounts, disabled accounts
		// and wrong codes so this endpoint cannot be used for user enumeration.
		fail(c, http.StatusUnauthorized, "MFA_LOGIN_INVALID", "账号或动态验证码错误", nil)
		return
	}
	if user.TokenVersion == 0 {
		user.TokenVersion = 1
		_ = s.db.Model(&user).Update("token_version", 1).Error
	}
	s.createAuditLogNoReq(nil, user.Username, "LOGIN_MFA_ONLY", "USER", fmt.Sprint(user.ID), "SUCCESS",
		map[string]any{"ip": ip}, c.GetString("trace_id"))
	s.issueSession(c, &user, ip)
}

func sixDigitCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, char := range code {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (s *Server) createMFATicket(ctx context.Context, user *model.User, ip string) (string, error) {
	if s.redis == nil {
		return "", errors.New("MFA 依赖 Redis，请检查 Redis 配置")
	}
	ticket := uuid.NewString()
	payload, _ := json.Marshal(mfaTicket{UserID: user.ID, IP: ip})
	if err := s.redis.Set(ctx, "sdv:auth:mfa:ticket:"+ticket, payload, mfaTicketTTL).Err(); err != nil {
		return "", err
	}
	return ticket, nil
}

// verifyMFA exchanges a short-lived login ticket plus a TOTP code for a
// session. The ticket is single-use and bound to the requesting IP.
func (s *Server) verifyMFA(c *gin.Context) {
	var req struct {
		Ticket string `json:"ticket" binding:"required"`
		Code   string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请输入动态验证码")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if s.redis == nil {
		fail(c, http.StatusServiceUnavailable, "DEPENDENCY_MISSING", "验证码服务不可用", nil)
		return
	}
	key := "sdv:auth:mfa:ticket:" + strings.TrimSpace(req.Ticket)
	raw, err := s.redis.GetDel(ctx, key).Result()
	if err != nil {
		fail(c, http.StatusUnauthorized, "MFA_TICKET_EXPIRED", "登录验证已过期，请重新登录", nil)
		return
	}
	var ticket mfaTicket
	if json.Unmarshal([]byte(raw), &ticket) != nil || ticket.IP != c.ClientIP() {
		fail(c, http.StatusUnauthorized, "MFA_TICKET_INVALID", "登录验证无效，请重新登录", nil)
		return
	}
	var user model.User
	if err := s.db.First(&user, ticket.UserID).Error; err != nil || user.Status != "ACTIVE" {
		fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "账号不可用", nil)
		return
	}
	secret, err := s.cipher.Decrypt(user.MFASecretEnc)
	if err != nil || !security.ValidateTOTP(secret, req.Code, time.Now()) {
		s.createAuditLogNoReq(nil, user.Username, "LOGIN_MFA_FAILED", "USER", fmt.Sprint(user.ID), "FAILED",
			map[string]any{"ip": c.ClientIP()}, c.GetString("trace_id"))
		fail(c, http.StatusUnauthorized, "MFA_INVALID", "动态验证码不正确", nil)
		return
	}
	s.issueSession(c, &user, c.ClientIP())
}

func (s *Server) mfaStatus(c *gin.Context) {
	user := currentUser(c)
	ok(c, gin.H{"enabled": user.MFAEnabled})
}

func (s *Server) mfaSetup(c *gin.Context) {
	user := currentUser(c)
	if user.MFAEnabled {
		badRequest(c, "已启用动态验证码，如需重置请先关闭")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if s.redis == nil {
		fail(c, http.StatusServiceUnavailable, "DEPENDENCY_MISSING", "验证码服务不可用", nil)
		return
	}
	secret, err := security.GenerateTOTPSecret()
	if err != nil {
		serverError(c, err)
		return
	}
	encrypted, err := s.cipher.Encrypt(secret)
	if err != nil {
		serverError(c, err)
		return
	}
	if err := s.redis.Set(ctx, fmt.Sprintf("sdv:auth:mfa:setup:%d", user.ID), encrypted, 10*time.Minute).Err(); err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{
		"secret":      secret,
		"otpauth_url": security.TOTPProvisioningURL(secret, user.Username, "智审图档 SmartDoc Vision"),
		"issuer":      "智审图档 SmartDoc Vision",
		"account":     user.Username,
		"expires_in":  600,
		"digits":      6,
		"period":      30,
		"algorithm":   "SHA1",
	})
}

func (s *Server) mfaEnable(c *gin.Context) {
	user := currentUser(c)
	if user.MFAEnabled {
		badRequest(c, "已启用动态验证码")
		return
	}
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请输入动态验证码")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if s.redis == nil {
		fail(c, http.StatusServiceUnavailable, "DEPENDENCY_MISSING", "验证码服务不可用", nil)
		return
	}
	key := fmt.Sprintf("sdv:auth:mfa:setup:%d", user.ID)
	encrypted, err := s.redis.Get(ctx, key).Result()
	if err != nil {
		badRequest(c, "设置已过期，请重新生成动态验证码密钥")
		return
	}
	secret, err := s.cipher.Decrypt(encrypted)
	if err != nil {
		serverError(c, err)
		return
	}
	if !security.ValidateTOTP(secret, req.Code, time.Now()) {
		badRequest(c, "动态验证码不正确，请确认手机时间与服务器一致后重试")
		return
	}
	if err := s.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"mfa_enabled": true, "mfa_secret_enc": encrypted,
	}).Error; err != nil {
		serverError(c, err)
		return
	}
	_ = s.redis.Del(ctx, key).Err()
	s.writeAudit(c, nil, "MFA_ENABLE", "USER", fmt.Sprint(user.ID), "SUCCESS", nil)
	ok(c, gin.H{"enabled": true})
}

func (s *Server) mfaDisable(c *gin.Context) {
	user := currentUser(c)
	if !user.MFAEnabled {
		badRequest(c, "尚未启用动态验证码")
		return
	}
	var req struct {
		Password string `json:"password" binding:"required"`
		Code     string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请输入登录密码和动态验证码")
		return
	}
	if !security.CheckPassword(user.PasswordHash, req.Password) {
		badRequest(c, "登录密码不正确")
		return
	}
	secret, err := s.cipher.Decrypt(user.MFASecretEnc)
	if err != nil || !security.ValidateTOTP(secret, req.Code, time.Now()) {
		badRequest(c, "动态验证码不正确")
		return
	}
	if err := s.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"mfa_enabled": false, "mfa_secret_enc": "",
	}).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, nil, "MFA_DISABLE", "USER", fmt.Sprint(user.ID), "SUCCESS", nil)
	ok(c, gin.H{"enabled": false})
}

// resetUserMFA is an administrative recovery path for a lost authenticator.
func (s *Server) resetUserMFA(c *gin.Context) {
	if currentUser(c).Role != model.RoleSuperAdmin {
		forbidden(c, "仅超级管理员可重置动态验证码")
		return
	}
	id, valid := parseUint(c.Param("id"))
	if !valid {
		badRequest(c, "用户 ID 无效")
		return
	}
	var user model.User
	if err := s.db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			notFound(c, "用户不存在")
			return
		}
		serverError(c, err)
		return
	}
	if !user.MFAEnabled {
		badRequest(c, "该账号未启用动态验证码")
		return
	}
	if err := s.db.Model(&model.User{}).Where("id = ?", user.ID).Updates(map[string]any{
		"mfa_enabled": false, "mfa_secret_enc": "", "token_version": gorm.Expr("token_version + 1"),
	}).Error; err != nil {
		serverError(c, err)
		return
	}
	s.writeAudit(c, nil, "MFA_RESET", "USER", fmt.Sprint(user.ID), "SUCCESS", map[string]any{"username": user.Username})
	ok(c, gin.H{"reset": true})
}
