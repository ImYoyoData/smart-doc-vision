package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/captcha"
	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/security"
)

const userContextKey = "current_user"
const claimsContextKey = "current_claims"

func currentUser(c *gin.Context) *model.User {
	v, exists := c.Get(userContextKey)
	if !exists {
		return &model.User{}
	}
	user, ok := v.(*model.User)
	if !ok {
		return &model.User{}
	}
	return user
}

func (s *Server) authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "请先登录", nil)
			c.Abort()
			return
		}
		claims, err := s.tokens.Parse(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil || claims.Type != "access" {
			fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "登录状态已失效，请重新登录", nil)
			c.Abort()
			return
		}
		var user model.User
		if err := s.db.First(&user, claims.UserID).Error; err != nil {
			fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "用户不存在", nil)
			c.Abort()
			return
		}
		if user.Status != "ACTIVE" {
			fail(c, http.StatusForbidden, "ACCOUNT_DISABLED", "账号已被停用", nil)
			c.Abort()
			return
		}
		if claims.TokenVersion != user.TokenVersion {
			fail(c, http.StatusUnauthorized, "TOKEN_REVOKED", "登录状态已被撤销，请重新登录", nil)
			c.Abort()
			return
		}
		s.hydrateUser(&user)
		c.Set(userContextKey, &user)
		c.Set(claimsContextKey, claims)
		c.Next()
	}
}

type loginRequest struct {
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
	CaptchaID string `json:"captcha_id"`
	CaptchaX  int    `json:"captcha_x"`
}

func (s *Server) captchaChallenge(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	if s.redis == nil {
		fail(c, http.StatusServiceUnavailable, "DEPENDENCY_MISSING", "验证码服务不可用", nil)
		return
	}
	ip := c.ClientIP()
	rateKey := "sdv:captcha:rate:" + strings.ToLower(ip)
	if count, err := s.redis.Incr(ctx, rateKey).Result(); err == nil {
		if count == 1 {
			_ = s.redis.Expire(ctx, rateKey, time.Minute).Err()
		}
		if count > 30 {
			fail(c, http.StatusTooManyRequests, "CAPTCHA_RATE_LIMITED", "验证码请求过于频繁，请稍后重试", nil)
			return
		}
	}
	challenge, answerX, err := captcha.Generate()
	if err != nil {
		serverError(c, err)
		return
	}
	id := uuid.NewString()
	payload, _ := json.Marshal(map[string]any{"x": answerX, "ip": ip})
	if err := s.redis.Set(ctx, captchaKey(id), payload, 2*time.Minute).Err(); err != nil {
		serverError(c, err)
		return
	}
	ok(c, gin.H{
		"id":         id,
		"background": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(challenge.Background),
		"piece":      "data:image/png;base64," + base64.StdEncoding.EncodeToString(challenge.Piece),
		"width":      challenge.Width,
		"height":     challenge.Height,
		"piece_size": challenge.PieceSize,
		"piece_y":    challenge.PieceY,
		"expires_in": challenge.ExpiresIn,
	})
}

func (s *Server) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "请输入账号和密码")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	ip := c.ClientIP()

	if retry, code := s.loginLocked(ctx, req.Username, ip); code != "" {
		s.writeLoginBlocked(c, time.Until(retry), code)
		return
	}
	if !s.verifyCaptcha(ctx, req.CaptchaID, req.CaptchaX, ip) {
		blocked, retry := s.recordLoginFailure(ctx, req.Username, ip)
		if blocked {
			s.writeLoginBlocked(c, retry, "IP_BLOCKED")
			return
		}
		s.createAuditLogNoReq(nil, req.Username, "LOGIN_CAPTCHA_FAILED", "USER", req.Username, "FAILED",
			map[string]any{"ip": ip}, c.GetString("trace_id"))
		fail(c, http.StatusBadRequest, "CAPTCHA_INVALID", "滑块验证失败或已过期，请重新拖动", nil)
		return
	}

	var user model.User
	err := s.db.Where("username = ?", req.Username).First(&user).Error
	if err != nil || !security.CheckPassword(user.PasswordHash, req.Password) {
		blocked, retry := s.recordLoginFailure(ctx, req.Username, ip)
		if blocked {
			s.writeLoginBlocked(c, retry, "IP_BLOCKED")
			return
		}
		s.createAuditLogNoReq(nil, req.Username, "LOGIN_FAILED", "USER", req.Username, "FAILED",
			map[string]any{"ip": ip}, c.GetString("trace_id"))
		fail(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "账号或密码错误", nil)
		return
	}
	if user.Status != "ACTIVE" {
		fail(c, http.StatusForbidden, "ACCOUNT_DISABLED", "账号已被停用", nil)
		return
	}
	if user.TokenVersion == 0 {
		user.TokenVersion = 1
		_ = s.db.Model(&user).Update("token_version", 1).Error
	}
	if user.MFAEnabled {
		ticket, err := s.createMFATicket(ctx, &user, ip)
		if err != nil {
			serverError(c, err)
			return
		}
		s.createAuditLogNoReq(nil, user.Username, "LOGIN_MFA_CHALLENGE", "USER", fmt.Sprint(user.ID), "SUCCESS",
			map[string]any{"ip": ip}, c.GetString("trace_id"))
		ok(c, gin.H{"mfa_required": true, "mfa_ticket": ticket, "expires_in": 300})
		return
	}
	s.issueSession(c, &user, ip)
}

// issueSession finalizes a login after password (and MFA, when enabled)
// verification and returns the access/refresh token pair.
func (s *Server) issueSession(c *gin.Context, user *model.User, ip string) {
	access, err := s.tokens.Access(user.ID, user.Username, user.Role, user.TokenVersion)
	if err != nil {
		serverError(c, err)
		return
	}
	refresh, err := s.tokens.Refresh(user.ID, user.Username, user.Role, user.TokenVersion)
	if err != nil {
		serverError(c, err)
		return
	}
	now := time.Now()
	_ = s.db.Model(user).Updates(map[string]any{"last_login_at": now, "last_login_ip": ip}).Error
	user.LastLoginAt = &now
	user.LastLoginIP = ip
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.clearLoginFailure(cleanupCtx, user.Username, ip)
	s.createAuditLogNoReq(nil, user.Username, "LOGIN", "USER", fmt.Sprint(user.ID), "SUCCESS",
		map[string]any{"ip": ip}, c.GetString("trace_id"))
	s.hydrateUser(user)
	ok(c, gin.H{
		"access_token":  access,
		"refresh_token": refresh,
		"expires_in":    24 * 3600,
		"user":          userView(*user),
	})
}

func (s *Server) refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "缺少 refresh_token")
		return
	}
	claims, err := s.tokens.Parse(req.RefreshToken)
	if err != nil || claims.Type != "refresh" {
		fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "刷新令牌无效或已过期", nil)
		return
	}
	var user model.User
	if err := s.db.First(&user, claims.UserID).Error; err != nil || user.Status != "ACTIVE" {
		fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "用户不可用", nil)
		return
	}
	if claims.TokenVersion != user.TokenVersion {
		fail(c, http.StatusUnauthorized, "TOKEN_REVOKED", "登录状态已被撤销，请重新登录", nil)
		return
	}
	s.hydrateUser(&user)
	access, _ := s.tokens.Access(user.ID, user.Username, user.Role, user.TokenVersion)
	refresh, _ := s.tokens.Refresh(user.ID, user.Username, user.Role, user.TokenVersion)
	ok(c, gin.H{"access_token": access, "refresh_token": refresh, "expires_in": 24 * 3600, "user": userView(user)})
}

func (s *Server) logout(c *gin.Context) {
	user := currentUser(c)
	if user.ID > 0 {
		// Incrementing the server-side version invalidates this and all older
		// access/refresh tokens for the account immediately.
		_ = s.db.Model(&model.User{}).Where("id = ?", user.ID).
			Update("token_version", gorm.Expr("token_version + 1")).Error
	}
	ok(c, gin.H{"logged_out": true})
}

func (s *Server) me(c *gin.Context) {
	user := currentUser(c)
	ok(c, gin.H{"user": userView(*user)})
}

func userView(user model.User) gin.H {
	return gin.H{
		"id": user.ID, "username": user.Username, "display_name": user.DisplayName,
		"email": user.Email,
		"role":  user.Role, "status": user.Status, "last_login_at": user.LastLoginAt,
		"mfa_enabled": user.MFAEnabled,
		"project_ids": user.ProjectIDs, "permissions": user.Permissions, "menus": user.Menus,
	}
}

func (s *Server) hydrateUser(user *model.User) {
	if definition, ok := s.roleDefinition(user.Role); ok {
		user.Permissions = definition.Permissions
		user.Menus = definition.Menus
	} else {
		user.Permissions = []string{}
		user.Menus = []string{}
	}
	if user.Role == model.RoleSuperAdmin {
		user.ProjectIDs = nil
		return
	}
	var ids []uint64
	_ = s.db.Model(&model.ProjectMember{}).Where("user_id = ?", user.ID).Pluck("project_id", &ids).Error
	user.ProjectIDs = ids
}

func (s *Server) loginLocked(ctx context.Context, username, ip string) (time.Time, string) {
	if s.redis == nil {
		return time.Time{}, ""
	}
	if retry, blocked := s.ipBlocked(ctx, ip); blocked {
		return retry, "IP_BLOCKED"
	}
	value, err := s.redis.Get(ctx, loginFailKey("user", username)).Result()
	if err != nil {
		return time.Time{}, ""
	}
	count, _ := strconv.Atoi(value)
	if count < 5 {
		return time.Time{}, ""
	}
	ttl, err := s.redis.TTL(ctx, loginFailKey("user", username)).Result()
	if err == nil && ttl > 0 {
		return time.Now().Add(ttl), "LOGIN_LOCKED"
	}
	return time.Time{}, ""
}

func (s *Server) ipBlocked(ctx context.Context, ip string) (time.Time, bool) {
	if s.redis == nil {
		return time.Time{}, false
	}
	ttl, err := s.redis.TTL(ctx, ipBlockKey(ip)).Result()
	if err == nil && ttl > 0 {
		return time.Now().Add(ttl), true
	}
	return time.Time{}, false
}

func (s *Server) recordLoginFailure(ctx context.Context, username, ip string) (bool, time.Duration) {
	if s.redis == nil {
		return false, 0
	}
	pipe := s.redis.TxPipeline()
	userCmd := pipe.Incr(ctx, loginFailKey("user", username))
	pipe.Expire(ctx, loginFailKey("user", username), 15*time.Minute)
	ipCmd := pipe.Incr(ctx, loginFailKey("ip", ip))
	pipe.Expire(ctx, loginFailKey("ip", ip), 10*time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0
	}
	_ = userCmd
	if ipCmd.Val() >= 3 {
		blockFor := 10 * time.Minute
		_ = s.redis.Set(ctx, ipBlockKey(ip), "1", blockFor).Err()
		_ = s.redis.Del(ctx, loginFailKey("ip", ip)).Err()
		return true, blockFor
	}
	return false, 0
}

func (s *Server) writeLoginBlocked(c *gin.Context, retry time.Duration, code string) {
	retryAfter := int(retry.Seconds())
	if retryAfter < 1 {
		retryAfter = 1
	}
	c.Header("Retry-After", strconv.Itoa(retryAfter))
	message := fmt.Sprintf("登录失败次数过多，请 %d 秒后重试", retryAfter)
	if code == "IP_BLOCKED" {
		message = fmt.Sprintf("连续 3 次登录失败，该 IP 已被封禁，请 %d 秒后重试", retryAfter)
	}
	fail(c, http.StatusTooManyRequests, code, message, gin.H{"retry_after": retryAfter})
}

func (s *Server) clearLoginFailure(ctx context.Context, username, ip string) {
	if s.redis == nil {
		return
	}
	_ = s.redis.Del(ctx, loginFailKey("user", username), loginFailKey("ip", ip)).Err()
}

func (s *Server) verifyCaptcha(ctx context.Context, id string, x int, ip string) bool {
	if s.redis == nil || strings.TrimSpace(id) == "" || x < 0 {
		return false
	}
	raw, err := s.redis.Get(ctx, captchaKey(id)).Result()
	if err != nil {
		return false
	}
	_ = s.redis.Del(ctx, captchaKey(id)).Err()
	var stored struct {
		X  int    `json:"x"`
		IP string `json:"ip"`
	}
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return false
	}
	if stored.IP != "" && stored.IP != ip {
		return false
	}
	diff := x - stored.X
	if diff < 0 {
		diff = -diff
	}
	return diff <= 8
}

func loginFailKey(scope, value string) string {
	return "sdv:login:fail:" + scope + ":" + strings.ToLower(value)
}

func ipBlockKey(ip string) string {
	return "sdv:login:block:ip:" + strings.ToLower(ip)
}

func captchaKey(id string) string {
	return "sdv:auth:captcha:" + id
}

func isNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
