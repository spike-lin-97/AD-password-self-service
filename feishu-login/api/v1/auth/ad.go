package auth

import (
	"net/http"
	"strings"

	"password-self-service/internal/audit"
	"password-self-service/internal/ldap"
	"password-self-service/internal/session"
	"password-self-service/pkg/response"

	"github.com/gin-gonic/gin"
)

type adLoginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// adLogin 用 AD 账密登录。锁定或忘记密码时无法走此通道，需飞书扫码。
func (h *Handler) adLogin(c *gin.Context) {
	var req adLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, hintInvalidPassword, "请输入 AD 用户名和密码")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		response.Fail(c, http.StatusBadRequest, hintInvalidPassword, "请输入 AD 用户名和密码")
		return
	}

	client, err := ldap.NewClient(h.ldapCfg())
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, hintADConnectFailed, "连接 AD 失败，请联系管理员")
		return
	}
	defer client.Close()

	user, err := client.Authenticate(req.Username, req.Password)
	if err != nil {
		msg := "用户名或密码错误。若账户已锁定或忘记密码，请改用飞书扫码登录。"
		if strings.Contains(err.Error(), "no AD user matched") {
			msg = "未找到该 AD 账号。可尝试登录名（如 zhangsan）或 UPN（如 zhangsan@example.com）。"
		}
		response.Fail(c, http.StatusUnauthorized, "ad_login_failed", msg)
		return
	}

	if err := h.session.Save(c.Writer, &session.Session{User: user}); err != nil {
		response.Fail(c, http.StatusInternalServerError, hintSessionSaveFailed, "写入登录态失败，请重试")
		return
	}
	h.record(c, audit.OpADLogin, user, expireOf(client, user))

	response.OK(c, http.StatusOK, "登录成功", gin.H{
		"display_name": user.DisplayName,
		"login_name":   user.SamAccountName,
		"sam":          user.SamAccountName,
		"upn":          user.UserPrincipalName,
		"next":         "/console.html",
	})
}

// unlockAccount 解锁当前登录用户的 AD 账户
func (h *Handler) unlockAccount(c *gin.Context) {
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

	if err := client.UnlockAccount(s.User.DN); err != nil {
		response.Fail(c, http.StatusInternalServerError, "unlock_failed", "解锁失败，请联系管理员检查绑定账号权限")
		return
	}

	s.User.Locked = false
	_ = h.session.Save(c.Writer, s)
	expire := "—"
	if fresh, err := client.FindByDN(s.User.DN); err == nil {
		expire = expireOf(client, fresh)
	} else {
		expire = expireOf(client, s.User)
	}
	h.record(c, audit.OpUnlock, s.User, expire)

	response.OK(c, http.StatusOK, "账户已解锁", gin.H{
		"login_name":   s.User.SamAccountName,
		"display_name": s.User.DisplayName,
		"locked":       false,
	})
}
