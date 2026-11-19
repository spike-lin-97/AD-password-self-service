package admin

import (
	"net/http"
	"strconv"
	"strings"

	"password-self-service/internal/audit"
	"password-self-service/internal/config"
	"password-self-service/internal/ldap"
	"password-self-service/internal/session"
	"password-self-service/pkg/feishu"
	"password-self-service/pkg/response"

	"github.com/gin-gonic/gin"
)

// Handler 管理后台
type Handler struct {
	store   *config.Store
	session *session.Manager
	audit   *audit.Store
}

func NewHandler(store *config.Store, sm *session.Manager, logs *audit.Store) *Handler {
	return &Handler{store: store, session: sm, audit: logs}
}

func (h *Handler) Register(r *gin.RouterGroup) {
	g := r.Group("/admin")
	g.POST("/login", h.login)
	g.POST("/logout", h.logout)
	auth := g.Group("")
	auth.Use(h.session.AdminRequired)
	{
		auth.GET("/settings", h.getSettings)
		auth.PUT("/settings", h.putSettings)
		auth.GET("/logs", h.listLogs)
		auth.POST("/test/ldap", h.testLDAP)
		auth.POST("/test/feishu", h.testFeishu)
	}
}

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *Handler) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid", "请输入用户名和密码")
		return
	}
	cfg := h.store.Snapshot()
	if cfg.Admin.Username == "" || cfg.Admin.Password == "" {
		response.Fail(c, http.StatusForbidden, "not_configured", "尚未配置管理后台账号，请先在 yaml 的 admin 段设置")
		return
	}
	if req.Username != cfg.Admin.Username || req.Password != cfg.Admin.Password {
		response.Fail(c, http.StatusUnauthorized, "unauthorized", "用户名或密码错误")
		return
	}
	if err := h.session.SaveAdmin(c.Writer, cfg.Admin.Username); err != nil {
		response.Fail(c, http.StatusInternalServerError, "session", "写入登录态失败")
		return
	}
	response.OK(c, http.StatusOK, "登录成功", gin.H{"username": cfg.Admin.Username})
}

func (h *Handler) logout(c *gin.Context) {
	h.session.DestroyAdmin(c.Writer)
	response.OK(c, http.StatusOK, "已退出", nil)
}

func (h *Handler) getSettings(c *gin.Context) {
	cfg := h.store.Snapshot()
	skip := true
	if cfg.LDAP.SkipVerify != nil {
		skip = *cfg.LDAP.SkipVerify
	}
	response.OK(c, http.StatusOK, "ok", gin.H{
		"feishu": gin.H{
			"app_id":         cfg.Feishu.AppID,
			"app_secret_set": cfg.Feishu.AppSecret != "",
			"redirect_url":   cfg.Feishu.RedirectURL,
			"ad_user_base":   cfg.Feishu.ADUserBase,
			"ad_mobile_attr": cfg.Feishu.ADMobileAttr,
		},
		"ldap": gin.H{
			"host":              cfg.LDAP.Host,
			"port":              cfg.LDAP.Port,
			"tls":               cfg.LDAP.TLS,
			"skip_verify":       skip,
			"username":          cfg.LDAP.Username,
			"password_set":      cfg.LDAP.Password != "",
			"base":              cfg.LDAP.Base,
			"ca_file":           cfg.LDAP.CAFile,
			"tls_server_name":   cfg.LDAP.TLSServerName,
			"password_max_days": cfg.LDAP.PasswordMaxDays,
		},
		"admin": gin.H{
			"username":     cfg.Admin.Username,
			"password_set": cfg.Admin.Password != "",
		},
		"tls_note": "当前默认 LDAPS(636) + 跳过证书校验。AD 自签证书过期一般不会中断连接；若关闭跳过校验，则必须提供有效 CA。",
	})
}

type putReq struct {
	Feishu *struct {
		AppID        *string `json:"app_id"`
		AppSecret    *string `json:"app_secret"`
		RedirectURL  *string `json:"redirect_url"`
		ADUserBase   *string `json:"ad_user_base"`
		ADMobileAttr *string `json:"ad_mobile_attr"`
	} `json:"feishu"`
	LDAP *struct {
		Host            *string `json:"host"`
		Port            *int    `json:"port"`
		TLS             *bool   `json:"tls"`
		SkipVerify      *bool   `json:"skip_verify"`
		Username        *string `json:"username"`
		Password        *string `json:"password"`
		Base            *string `json:"base"`
		CAFile          *string `json:"ca_file"`
		TLSServerName   *string `json:"tls_server_name"`
		PasswordMaxDays *int    `json:"password_max_days"`
	} `json:"ldap"`
	Admin *struct {
		Username *string `json:"username"`
		Password *string `json:"password"`
	} `json:"admin"`
}

func (h *Handler) putSettings(c *gin.Context) {
	var req putReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid", "请求参数不正确")
		return
	}
	err := h.store.Update(func(cfg *config.Config) error {
		if req.Feishu != nil {
			setStr(&cfg.Feishu.AppID, req.Feishu.AppID)
			setSecret(&cfg.Feishu.AppSecret, req.Feishu.AppSecret)
			setStr(&cfg.Feishu.RedirectURL, req.Feishu.RedirectURL)
			setStr(&cfg.Feishu.ADUserBase, req.Feishu.ADUserBase)
			setStr(&cfg.Feishu.ADMobileAttr, req.Feishu.ADMobileAttr)
		}
		if req.LDAP != nil {
			setStr(&cfg.LDAP.Host, req.LDAP.Host)
			if req.LDAP.Port != nil && *req.LDAP.Port > 0 {
				cfg.LDAP.Port = *req.LDAP.Port
			}
			if req.LDAP.TLS != nil {
				cfg.LDAP.TLS = *req.LDAP.TLS
			}
			if req.LDAP.SkipVerify != nil {
				v := *req.LDAP.SkipVerify
				cfg.LDAP.SkipVerify = &v
			}
			setStr(&cfg.LDAP.Username, req.LDAP.Username)
			setSecret(&cfg.LDAP.Password, req.LDAP.Password)
			setStr(&cfg.LDAP.Base, req.LDAP.Base)
			setStr(&cfg.LDAP.CAFile, req.LDAP.CAFile)
			setStr(&cfg.LDAP.TLSServerName, req.LDAP.TLSServerName)
			if req.LDAP.PasswordMaxDays != nil && *req.LDAP.PasswordMaxDays > 0 {
				cfg.LDAP.PasswordMaxDays = *req.LDAP.PasswordMaxDays
			}
		}
		if req.Admin != nil {
			setStr(&cfg.Admin.Username, req.Admin.Username)
			setSecret(&cfg.Admin.Password, req.Admin.Password)
		}
		return nil
	})
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "save_failed", err.Error())
		return
	}
	response.OK(c, http.StatusOK, "已保存，新配置立即生效", nil)
}

func (h *Handler) listLogs(c *gin.Context) {
	if h.audit == nil {
		response.OK(c, http.StatusOK, "ok", gin.H{"items": []audit.Record{}, "total": 0})
		return
	}
	q := c.Query("q")
	offset, _ := strconv.Atoi(c.Query("offset"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 50
	}
	items, total, err := h.audit.List(q, offset, limit)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "log_read_failed", "读取操作日志失败")
		return
	}
	if items == nil {
		items = []audit.Record{}
	}
	response.OK(c, http.StatusOK, "ok", gin.H{
		"items":  items,
		"total":  total,
		"offset": offset,
		"limit":  limit,
	})
}

type testLDAPReq struct {
	Host          string `json:"host"`
	Port          int    `json:"port"`
	TLS           *bool  `json:"tls"`
	SkipVerify    *bool  `json:"skip_verify"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Base          string `json:"base"`
	CAFile        string `json:"ca_file"`
	TLSServerName string `json:"tls_server_name"`
}

func (h *Handler) testLDAP(c *gin.Context) {
	var req testLDAPReq
	_ = c.ShouldBindJSON(&req)
	cfg := h.store.LDAPRuntime()
	if req.Host != "" {
		cfg.Host = req.Host
	}
	if req.Port > 0 {
		cfg.Port = req.Port
	}
	if req.TLS != nil {
		cfg.TLS = *req.TLS
	}
	if req.SkipVerify != nil {
		cfg.SkipVerify = *req.SkipVerify
	}
	if req.Username != "" {
		cfg.Username = req.Username
	}
	if strings.TrimSpace(req.Password) != "" {
		cfg.Password = req.Password
	}
	if req.Base != "" {
		cfg.Base = req.Base
	}
	if req.CAFile != "" {
		cfg.CAFile = req.CAFile
	}
	if req.TLSServerName != "" {
		cfg.TLSServerName = req.TLSServerName
	}
	result := ldap.Probe(cfg)
	response.OK(c, http.StatusOK, result.Message, result)
}

type testFeishuReq struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

func (h *Handler) testFeishu(c *gin.Context) {
	var req testFeishuReq
	_ = c.ShouldBindJSON(&req)
	cur := h.store.FeishuRuntime()
	if req.AppID != "" {
		cur.AppID = req.AppID
	}
	if strings.TrimSpace(req.AppSecret) != "" {
		cur.AppSecret = req.AppSecret
	}
	msg, err := feishu.TestApp(cur.AppID, cur.AppSecret)
	if err != nil {
		response.Fail(c, http.StatusBadGateway, "feishu_test_failed", err.Error())
		return
	}
	response.OK(c, http.StatusOK, msg, gin.H{"app_id": cur.AppID})
}

func setStr(dst *string, src *string) {
	if src == nil {
		return
	}
	*dst = strings.TrimSpace(*src)
}

func setSecret(dst *string, src *string) {
	if src == nil {
		return
	}
	if strings.TrimSpace(*src) == "" {
		return
	}
	*dst = *src
}
