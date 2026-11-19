package session

import (
	"net/http"
	"time"

	"password-self-service/pkg/response"

	"github.com/gin-gonic/gin"
)

const adminCookie = "pss_admin"

// AdminSession 管理后台登录态
type AdminSession struct {
	Username string
	LoginAt  time.Time
}

// SaveAdmin 写入管理员 Cookie
func (m *Manager) SaveAdmin(w http.ResponseWriter, username string) error {
	s := AdminSession{Username: username, LoginAt: time.Now()}
	encoded, err := m.sc.Encode(adminCookie, s)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookie,
		Value:    encoded,
		Path:     "/",
		MaxAge:   m.maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// GetAdmin 读取管理员登录态
func (m *Manager) GetAdmin(r *http.Request) (*AdminSession, error) {
	cookie, err := r.Cookie(adminCookie)
	if err != nil {
		return nil, err
	}
	var s AdminSession
	if err := m.sc.Decode(adminCookie, cookie.Value, &s); err != nil {
		return nil, err
	}
	if s.Username == "" {
		return nil, http.ErrNoCookie
	}
	return &s, nil
}

// DestroyAdmin 清除管理员 Cookie
func (m *Manager) DestroyAdmin(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

// AdminRequired 管理后台鉴权中间件
func (m *Manager) AdminRequired(c *gin.Context) {
	sess, err := m.GetAdmin(c.Request)
	if err != nil || sess == nil || sess.Username == "" {
		response.Abort(c, http.StatusUnauthorized, "unauthorized", "请先登录管理后台")
		return
	}
	c.Set("admin", sess)
	c.Next()
}
