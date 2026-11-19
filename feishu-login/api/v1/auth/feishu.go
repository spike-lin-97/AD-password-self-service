// Package auth 飞书扫码登录 + AD 姓名匹配 + 改密码接口。
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"password-self-service/internal/audit"
	"password-self-service/internal/config"
	"password-self-service/internal/ldap"
	"password-self-service/internal/session"
	"password-self-service/pkg/feishu"
	"password-self-service/pkg/response"

	"github.com/gin-gonic/gin"
	golap "github.com/go-ldap/ldap/v3"
)

const (
	hintInvalidState      = "invalid_state"
	hintMissingCode       = "missing_code"
	hintFeishuTokenFailed = "feishu_token_failed"
	hintFeishuUserFailed  = "feishu_userinfo_failed"
	hintFeishuNameEmpty   = "feishu_name_empty"
	hintFeishuDenied      = "feishu_denied"
	hintADConnectFailed   = "ad_connect_failed"
	hintADUserNotFound    = "ad_user_not_found"
	hintADUserAmbiguous   = "ad_user_ambiguous"
	hintSessionSaveFailed = "session_save_failed"
	hintGenerateState     = "generate_state_failed"
	hintUnauthorized      = "unauthorized"
	hintInvalidPassword   = "invalid_password"
	hintModifyFailed      = "modify_failed"
)

// Handler 登录相关 handler
type Handler struct {
	store   *config.Store
	session *session.Manager
	audit   *audit.Store
}

// NewHandler 构造 Handler
func NewHandler(store *config.Store, sm *session.Manager, logs *audit.Store) *Handler {
	return &Handler{store: store, session: sm, audit: logs}
}

func (h *Handler) record(c *gin.Context, op string, user *ldap.ADUser, expireAt string) {
	if h == nil || h.audit == nil || user == nil {
		return
	}
	rec := audit.FromRequest(c, op, user.SamAccountName, user.DisplayName, expireAt)
	if err := h.audit.Append(rec); err != nil {
		log.Printf("audit append failed: %v", err)
	}
}

func expireOf(client *ldap.Client, user *ldap.ADUser) string {
	if client == nil || user == nil {
		return "—"
	}
	t := client.PasswordTimesOf(user)
	if t.ExpireAt == "" {
		return "—"
	}
	return t.ExpireAt
}

func (h *Handler) fs() *feishu.Client {
	cfg := h.store.FeishuRuntime()
	return feishu.NewClient(cfg)
}

func (h *Handler) ldapCfg() ldap.Config {
	return h.store.LDAPRuntime()
}

// Register 注册路由
func (h *Handler) Register(r *gin.RouterGroup) {
	g := r.Group("/auth")
	{
		fs := g.Group("/feishu")
		{
			fs.GET("/login", h.login)
			fs.GET("/qr-config", h.qrConfig)
			fs.GET("/callback", h.callback)
			fs.POST("/logout", h.logout)
			fs.GET("/logout", h.logoutRedirect)
		}
		g.GET("/me", h.session.AuthRequired, h.me)
		g.POST("/ad/login", h.adLogin)
		g.POST("/change-password", h.session.AuthRequired, h.changePassword)
		g.POST("/unlock-account", h.session.AuthRequired, h.unlockAccount)
	}
	// 兼容旧路径
	r.POST("/change-password", h.session.AuthRequired, h.changePassword)
}

// login 生成随机 state 写入 Cookie，再跳转飞书授权页
func (h *Handler) login(c *gin.Context) {
	state, err := randomState(32)
	if err != nil {
		h.redirectError(c, hintGenerateState, "初始化登录失败，请刷新页面重试", nil)
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "_fs_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	c.Redirect(http.StatusFound, h.fs().AuthorizeURL(state))
}

// qrConfig 为页内二维码 SDK 下发 goto 地址，并写入 state Cookie
func (h *Handler) qrConfig(c *gin.Context) {
	state, err := randomState(32)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, hintGenerateState, "初始化登录失败，请刷新页面重试")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "_fs_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	response.OK(c, http.StatusOK, "ok", gin.H{
		"goto": h.fs().QRAuthorizeURL(state),
	})
}

// callback 飞书回调：校验 state → 换 token → 按姓名匹配 AD → 写登录态 → 跳转改密页
func (h *Handler) callback(c *gin.Context) {
	if feishuErr := c.Query("error"); feishuErr != "" {
		h.redirectError(c, hintFeishuDenied, c.Query("error_description"), nil)
		return
	}

	state := c.Query("state")
	saved, _ := c.Cookie("_fs_state")
	if state == "" || state != saved {
		h.redirectError(c, hintInvalidState, "登录校验失败，请重新扫码", nil)
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{Name: "_fs_state", Value: "", Path: "/", MaxAge: -1})

	code := c.Query("code")
	if code == "" {
		h.redirectError(c, hintMissingCode, "飞书未返回授权码", nil)
		return
	}

	token, err := h.fs().ExchangeToken(context.Background(), code)
	if err != nil {
		h.redirectError(c, hintFeishuTokenFailed, "飞书授权失败，请稍后重试", nil)
		return
	}

	userInfo, err := h.fs().GetUserInfo(context.Background(), token.AccessToken)
	if err != nil {
		h.redirectError(c, hintFeishuUserFailed, "获取飞书用户信息失败", nil)
		return
	}

	name := userInfo.Data.Name
	if name == "" {
		h.redirectError(c, hintFeishuNameEmpty, "飞书用户姓名为空，无法匹配 AD", nil)
		return
	}

	client, err := ldap.NewClient(h.ldapCfg())
	if err != nil {
		h.redirectError(c, hintADConnectFailed, "连接 AD 失败，请联系管理员", nil)
		return
	}
	defer client.Close()

	adUser, err := client.FindByName(name)
	if err != nil {
		hint := hintADUserNotFound
		msg := "未在 AD 中找到对应用户，请确认飞书姓名与 AD 显示名一致"
		if strings.Contains(err.Error(), "multiple AD users") {
			hint = hintADUserAmbiguous
			msg = "AD 中存在多个同名用户，无法唯一匹配"
		}
		h.redirectError(c, hint, msg, url.Values{"name": {name}})
		return
	}

	if err := h.session.Save(c.Writer, &session.Session{User: adUser}); err != nil {
		h.redirectError(c, hintSessionSaveFailed, "写入登录态失败，请重试", nil)
		return
	}
	h.record(c, audit.OpFeishuLogin, adUser, expireOf(client, adUser))

	c.Redirect(http.StatusFound, "/console.html")
}

// me 返回当前登录用户（供前端确认 session）
func (h *Handler) me(c *gin.Context) {
	s := mustSession(c)
	if s == nil {
		response.Fail(c, http.StatusUnauthorized, hintUnauthorized, "未登录或登录态已过期，请先登录")
		return
	}
	client, err := ldap.NewClient(h.ldapCfg())
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, hintADConnectFailed, "连接 AD 失败，请联系管理员")
		return
	}
	defer client.Close()

	user := s.User
	if fresh, err := client.FindByDN(s.User.DN); err == nil {
		user = fresh
	}
	times := client.PasswordTimesOf(user)

	response.OK(c, http.StatusOK, "ok", gin.H{
		"display_name":      user.DisplayName,
		"login_name":        user.SamAccountName,
		"sam":               user.SamAccountName,
		"locked":            user.Locked,
		"pwd_last_set":      times.LastSet,
		"pwd_expire_at":     times.ExpireAt,
		"pwd_never_expires": times.NeverExpire,
	})
}

// logout 登出（API）
func (h *Handler) logout(c *gin.Context) {
	h.session.Destroy(c.Writer)
	response.OK(c, http.StatusOK, "已退出登录", nil)
}

// logoutRedirect 登出后回到首页
func (h *Handler) logoutRedirect(c *gin.Context) {
	h.session.Destroy(c.Writer)
	c.Redirect(http.StatusFound, "/")
}

// changePasswordReq 改密码请求
type changePasswordReq struct {
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

// changePassword 已通过 Session 认证，直接改 AD 密码
func (h *Handler) changePassword(c *gin.Context) {
	s := mustSession(c)
	if s == nil {
		response.Fail(c, http.StatusUnauthorized, hintUnauthorized, "未登录或登录态已过期，请先登录")
		return
	}
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, hintInvalidPassword, "新密码必填且至少 8 位")
		return
	}
	if msg := validatePasswordPolicy(req.NewPassword); msg != "" {
		response.Fail(c, http.StatusBadRequest, hintInvalidPassword, msg)
		return
	}

	client, err := ldap.NewClient(h.ldapCfg())
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, hintADConnectFailed, "连接 AD 失败，请联系管理员")
		return
	}
	defer client.Close()

	if err := client.ModifyPassword(s.User.DN, req.NewPassword); err != nil {
		response.Fail(c, http.StatusInternalServerError, hintModifyFailed, friendlyPasswordError(err))
		return
	}
	user := s.User
	if fresh, err := client.FindByDN(s.User.DN); err == nil {
		user = fresh
	}
	h.record(c, audit.OpChangePassword, user, expireOf(client, user))

	response.OK(c, http.StatusOK, fmt.Sprintf("用户 %s 密码修改成功", s.User.DisplayName), nil)
}

func (h *Handler) redirectError(c *gin.Context, hint, message string, extra url.Values) {
	q := url.Values{}
	q.Set("error", hint)
	if message != "" {
		q.Set("msg", message)
	}
	for k, vs := range extra {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	c.Redirect(http.StatusFound, "/?"+q.Encode())
}

func mustSession(c *gin.Context) *session.Session {
	val, ok := c.Get("session")
	if !ok {
		return nil
	}
	s, ok := val.(*session.Session)
	if !ok || s == nil || s.User == nil || s.User.DN == "" {
		return nil
	}
	return s
}

func randomState(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func validatePasswordPolicy(pw string) string {
	if len(pw) < 8 {
		return "密码至少 8 位"
	}
	var upper, lower, digit, special bool
	for _, r := range pw {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		default:
			special = true
		}
	}
	n := 0
	for _, ok := range []bool{upper, lower, digit, special} {
		if ok {
			n++
		}
	}
	if n < 3 {
		return "密码需包含大写、小写、数字、特殊字符中的至少 3 类"
	}
	return ""
}

func friendlyPasswordError(err error) string {
	var le *golap.Error
	if errors.As(err, &le) {
		switch le.ResultCode {
		case golap.LDAPResultConstraintViolation:
			return "密码不符合 AD 复杂度策略：至少 8 位，需包含大小写、数字或特殊字符，且不能与近期密码重复"
		case golap.LDAPResultUnwillingToPerform:
			return "AD 拒绝修改密码，请确认已使用 LDAPS 且账号有重置密码权限"
		case golap.LDAPResultInsufficientAccessRights:
			return "当前绑定账号没有重置密码权限，请联系管理员"
		}
	}
	return "修改密码失败，请确认密码符合复杂度要求后重试"
}
