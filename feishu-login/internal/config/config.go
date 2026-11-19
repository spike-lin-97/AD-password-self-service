// Package config 应用配置加载。
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 应用配置（对应 YAML 根节点）
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Redis   RedisConfig   `yaml:"redis"`
	LDAP    LDAPConfig    `yaml:"ldap"`
	Cron    CronConfig    `yaml:"cron"`
	Channel ChannelConfig `yaml:"channel"`
	Feishu  FeishuConfig  `yaml:"feishu"`
	Session SessionConfig `yaml:"session"`
	Admin   AdminConfig   `yaml:"admin"`
}

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	Mode          string `yaml:"mode"`
	Addr          string `yaml:"addr"`
	RateLimit     int    `yaml:"rate-limit"`
	RSAPublicKey  string `yaml:"rsa-public-key"`
	RSAPrivateKey string `yaml:"rsa-private-key"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// LDAPConfig AD/LDAP 配置
type LDAPConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	TLS             bool   `yaml:"tls"`
	Username        string `yaml:"username"`
	Password        string `yaml:"password"`
	Base            string `yaml:"base"`
	Filter          string `yaml:"filter"`
	TimeLimit       int    `yaml:"time-limit"`
	SizeLimit       int    `yaml:"size-limit"`
	CAFile          string `yaml:"ca-file"`
	TLSServerName   string `yaml:"tls-server-name"`
	PasswordMaxDays int    `yaml:"password-max-days"`
	SkipVerify      *bool  `yaml:"skip-verify"`
}

// AdminConfig 管理后台账号
type AdminConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// CronConfig 定时任务配置
type CronConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Schedule string `yaml:"schedule"`
}

// ChannelConfig 通道配置（飞书登录场景下可留空，保留结构避免改原 YAML 报错）
type ChannelConfig struct {
	PlatformURL    string      `yaml:"platform-url"`
	VerifyChannel  string      `yaml:"verify-channel"`
	ExpiredChannel string      `yaml:"expired-channel"`
	Mail           interface{} `yaml:"mail"`
	AliyunSMS      interface{} `yaml:"aliyunsms"`
	AliyunVoice    interface{} `yaml:"aliyunvoice"`
	TencentSMS     interface{} `yaml:"tencentsms"`
}

// FeishuConfig 飞书扫码登录配置
type FeishuConfig struct {
	// AppID 飞书应用 App ID（client_id）
	AppID string `yaml:"app-id"`
	// AppSecret 飞书应用 App Secret（client_secret）
	AppSecret string `yaml:"app-secret"`
	// RedirectURL 回调地址，须与飞书开放平台「重定向 URL」完全一致
	RedirectURL string `yaml:"redirect-url"`
	// StateSecret 预留（state 使用随机值 + Cookie 校验，此字段暂未使用）
	StateSecret string `yaml:"state-secret"`
	// ADMobileAttr AD 中手机号字段名，默认 telephoneNumber
	ADMobileAttr string `yaml:"ad-mobile-attr"`
	// ADUserBase 匹配用户时的搜索 base，为空则使用 ldap.base
	ADUserBase string `yaml:"ad-user-base"`
}

// SessionConfig Cookie Session 配置
type SessionConfig struct {
	// CookieName Cookie 名，默认 pss_session
	CookieName string `yaml:"cookie-name"`
	// HashKey HMAC-SHA256 签名密钥（32+ 字节），生产环境务必随机且保密
	HashKey string `yaml:"hash-key"`
	// BlockKey AES 加密密钥（16/24/32 字节）
	BlockKey string `yaml:"block-key"`
	// MaxAge 有效期（秒），默认 3600
	MaxAge int `yaml:"max-age"`
}

// Load 从指定路径加载 YAML 配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s failed: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config failed: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	_ = data
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Session.CookieName == "" {
		c.Session.CookieName = "pss_session"
	}
	if c.Session.MaxAge <= 0 {
		c.Session.MaxAge = 3600
	}
	if c.Feishu.ADMobileAttr == "" {
		c.Feishu.ADMobileAttr = "telephoneNumber"
	}
	if c.Feishu.ADUserBase == "" {
		c.Feishu.ADUserBase = c.LDAP.Base
	}
	if c.LDAP.PasswordMaxDays <= 0 {
		c.LDAP.PasswordMaxDays = 90
	}
	if c.LDAP.SkipVerify == nil {
		t := true
		c.LDAP.SkipVerify = &t
	}
	if c.Admin.Username == "" {
		c.Admin.Username = "admin"
	}
}

// validate 校验必填项
func (c *Config) validate() error {
	if c.Feishu.AppID == "" || c.Feishu.AppSecret == "" {
		return fmt.Errorf("feishu.app-id / feishu.app-secret 不能为空")
	}
	if c.Feishu.RedirectURL == "" {
		return fmt.Errorf("feishu.redirect-url 不能为空（需与飞书开放平台配置一致）")
	}
	if c.Session.HashKey == "" || c.Session.BlockKey == "" {
		return fmt.Errorf("session.hash-key / session.block-key 不能为空（生产环境请使用随机密钥）")
	}
	if c.LDAP.Host == "" || c.LDAP.Username == "" {
		return fmt.Errorf("ldap.host / ldap.username 不能为空")
	}
	return nil
}

// RedisAddr 返回 Redis 网络地址
func (c *Config) RedisAddr() string {
	if c.Redis.Port == 0 {
		return c.Redis.Host
	}
	return fmt.Sprintf("%s:%d", c.Redis.Host, c.Redis.Port)
}
