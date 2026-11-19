// Package session Cookie Session 管理。
// 用 gorilla/securecookie 将登录态（ADUser 信息）加密签名后写入 Cookie，
// 后续改密码接口读取 Cookie 即可确认身份，无需验证码。
package session

import (
	"encoding/gob"
	"fmt"
	"net/http"
	"time"

	"password-self-service/internal/ldap"
	"password-self-service/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/securecookie"
)

// Session 登录态
type Session struct {
	User    *ldap.ADUser
	LoginAt time.Time
}

// Manager Cookie Session 管理器
type Manager struct {
	sc     *securecookie.SecureCookie
	name   string // Cookie 名
	maxAge int    // 有效期（秒）
}

// NewManager 创建 Session 管理器。
// hashKey 用于 HMAC 签名，blockKey 用于加密，二者应通过配置注入（生产环境务必随机且保密）。
func NewManager(cookieName, hashKey, blockKey string, maxAge int) (*Manager, error) {
	if cookieName == "" {
		cookieName = "pss_session"
	}
	if maxAge <= 0 {
		maxAge = 3600
	}
	if len(hashKey) == 0 || len(blockKey) == 0 {
		return nil, fmt.Errorf("session hashKey/blockKey must not be empty")
	}
	sc := securecookie.New([]byte(hashKey), []byte(blockKey))
	sc.MaxAge(maxAge)
	gob.Register(&ldap.ADUser{})
	gob.Register(&Session{})
	gob.Register(AdminSession{})
	return &Manager{sc: sc, name: cookieName, maxAge: maxAge}, nil
}

// Save 将登录态写入 Cookie
func (m *Manager) Save(w http.ResponseWriter, s *Session) error {
	if s == nil {
		s = &Session{}
	}
	if s.LoginAt.IsZero() {
		s.LoginAt = time.Now()
	}
	encoded, err := m.sc.Encode(m.name, s)
	if err != nil {
		return err
	}
	cookie := &http.Cookie{
		Name:     m.name,
		Value:    encoded,
		Path:     "/",
		MaxAge:   m.maxAge,
		HttpOnly: true, // JS 不可读，防 XSS
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
	return nil
}

// Get 从 Cookie 读取登录态
func (m *Manager) Get(r *http.Request) (*Session, error) {
	cookie, err := r.Cookie(m.name)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := m.sc.Decode(m.name, cookie.Value, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// AuthRequired Gin 中间件：校验登录态，将 *Session 注入上下文
func (m *Manager) AuthRequired(c *gin.Context) {
	sess, err := m.Get(c.Request)
	if err != nil || sess == nil || sess.User == nil || sess.User.DN == "" {
		response.Abort(c, http.StatusUnauthorized, "unauthorized", "未登录或登录态已过期，请先登录")
		return
	}
	c.Set("session", sess)
	c.Next()
}

// Destroy 清除 Cookie（登出）
func (m *Manager) Destroy(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     m.name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)
}
