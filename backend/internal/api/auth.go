package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"smartdoc-vision/backend/internal/catalog"
	"smartdoc-vision/backend/internal/model"
	"smartdoc-vision/backend/internal/security"
)

const userContextKey = "current_user"

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
		s.hydrateUser(&user)
		c.Set(userContextKey, &user)
		c.Next()
	}
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
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

	if retry, locked := s.loginLocked(ctx, req.Username, c.ClientIP()); locked {
		retryAfter := int(time.Until(retry).Seconds())
		if retryAfter < 1 {
			retryAfter = 1
		}
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		fail(c, http.StatusTooManyRequests, "LOGIN_LOCKED", fmt.Sprintf("失败次数过多，请 %d 秒后重试", retryAfter), nil)
		return
	}

	var user model.User
	err := s.db.Where("username = ?", req.Username).First(&user).Error
	if err != nil || !security.CheckPassword(user.PasswordHash, req.Password) {
		s.recordLoginFailure(ctx, req.Username, c.ClientIP())
		s.createAuditLogNoReq(nil, req.Username, "LOGIN_FAILED", "USER", req.Username, "FAILED",
			map[string]any{"ip": c.ClientIP()}, c.GetString("trace_id"))
		fail(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "账号或密码错误", nil)
		return
	}
	if user.Status != "ACTIVE" {
		fail(c, http.StatusForbidden, "ACCOUNT_DISABLED", "账号已被停用", nil)
		return
	}

	access, err := s.tokens.Access(user.ID, user.Username, user.Role)
	if err != nil {
		serverError(c, err)
		return
	}
	refresh, err := s.tokens.Refresh(user.ID, user.Username, user.Role)
	if err != nil {
		serverError(c, err)
		return
	}
	now := time.Now()
	ip := c.ClientIP()
	_ = s.db.Model(&user).Updates(map[string]any{"last_login_at": now, "last_login_ip": ip}).Error
	user.LastLoginAt = &now
	user.LastLoginIP = ip
	s.clearLoginFailure(ctx, req.Username, ip)
	s.createAuditLogNoReq(nil, user.Username, "LOGIN", "USER", fmt.Sprint(user.ID), "SUCCESS",
		map[string]any{"ip": ip}, c.GetString("trace_id"))
	s.hydrateUser(&user)
	ok(c, gin.H{
		"access_token":  access,
		"refresh_token": refresh,
		"expires_in":    24 * 3600,
		"user":          userView(user),
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
	s.hydrateUser(&user)
	access, _ := s.tokens.Access(user.ID, user.Username, user.Role)
	refresh, _ := s.tokens.Refresh(user.ID, user.Username, user.Role)
	ok(c, gin.H{"access_token": access, "refresh_token": refresh, "expires_in": 24 * 3600, "user": userView(user)})
}

func (s *Server) me(c *gin.Context) {
	user := currentUser(c)
	ok(c, gin.H{"user": userView(*user)})
}

func userView(user model.User) gin.H {
	return gin.H{
		"id": user.ID, "username": user.Username, "display_name": user.DisplayName,
		"role": user.Role, "status": user.Status, "last_login_at": user.LastLoginAt,
		"project_ids": user.ProjectIDs, "permissions": user.Permissions, "menus": user.Menus,
	}
}

func (s *Server) hydrateUser(user *model.User) {
	user.Permissions = catalog.RolePermissions(user.Role)
	user.Menus = catalog.RoleMenus(user.Role)
	if user.Role == model.RoleSuperAdmin {
		user.ProjectIDs = nil
		return
	}
	var ids []uint64
	_ = s.db.Model(&model.ProjectMember{}).Where("user_id = ?", user.ID).Pluck("project_id", &ids).Error
	user.ProjectIDs = ids
}

func (s *Server) loginLocked(ctx context.Context, username, ip string) (time.Time, bool) {
	if s.redis == nil {
		return time.Time{}, false
	}
	keys := []string{loginFailKey("user", username), loginFailKey("ip", ip)}
	for _, key := range keys {
		val, err := s.redis.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		count, _ := strconv.Atoi(val)
		limit := 5
		if strings.HasPrefix(key, "sdv:login:fail:ip:") {
			limit = 30
		}
		if count >= limit {
			ttl, err := s.redis.TTL(ctx, key).Result()
			if err == nil && ttl > 0 {
				return time.Now().Add(ttl), true
			}
		}
	}
	return time.Time{}, false
}

func (s *Server) recordLoginFailure(ctx context.Context, username, ip string) {
	if s.redis == nil {
		return
	}
	for _, key := range []string{loginFailKey("user", username), loginFailKey("ip", ip)} {
		pipe := s.redis.TxPipeline()
		pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, 15*time.Minute)
		_, _ = pipe.Exec(ctx)
	}
}

func (s *Server) clearLoginFailure(ctx context.Context, username, ip string) {
	if s.redis == nil {
		return
	}
	_ = s.redis.Del(ctx, loginFailKey("user", username), loginFailKey("ip", ip)).Err()
}

func loginFailKey(scope, value string) string {
	return "sdv:login:fail:" + scope + ":" + strings.ToLower(value)
}

func isNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
