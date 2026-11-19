package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"password-self-service/internal/ldap"
	"password-self-service/pkg/feishu"
)

// Store 运行时配置，支持后台修改后立刻生效并写回 YAML。
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  *Config
}

// NewStore 从已加载的配置创建 Store
func NewStore(path string, cfg *Config) *Store {
	return &Store{path: path, cfg: cfg}
}

// Path 配置文件路径
func (s *Store) Path() string {
	return s.path
}

// Snapshot 返回配置副本
func (s *Store) Snapshot() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.cfg
}

// LDAPRuntime 当前 AD 连接参数
func (s *Store) LDAPRuntime() ldap.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.cfg
	base := c.Feishu.ADUserBase
	if base == "" {
		base = c.LDAP.Base
	}
	skip := true
	if c.LDAP.SkipVerify != nil {
		skip = *c.LDAP.SkipVerify
	}
	return ldap.Config{
		Host:            c.LDAP.Host,
		Port:            c.LDAP.Port,
		TLS:             c.LDAP.TLS,
		Username:        c.LDAP.Username,
		Password:        c.LDAP.Password,
		Base:            base,
		TimeLimit:       c.LDAP.TimeLimit,
		SizeLimit:       c.LDAP.SizeLimit,
		MobileAttr:      c.Feishu.ADMobileAttr,
		CAFile:          c.LDAP.CAFile,
		TLSServerName:   c.LDAP.TLSServerName,
		PasswordMaxDays: c.LDAP.PasswordMaxDays,
		SkipVerify:      skip,
	}
}

// FeishuRuntime 当前飞书参数
func (s *Store) FeishuRuntime() feishu.Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return feishu.Config{
		AppID:       s.cfg.Feishu.AppID,
		AppSecret:   s.cfg.Feishu.AppSecret,
		RedirectURL: s.cfg.Feishu.RedirectURL,
	}
}

// Update 修改配置并写回文件
func (s *Store) Update(fn func(c *Config) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(s.cfg); err != nil {
		return err
	}
	s.cfg.applyDefaults()
	if err := s.cfg.validate(); err != nil {
		return err
	}
	return s.writeFileLocked()
}

func (s *Store) writeFileLocked() error {
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".pss-config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(s.cfg); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("encode config: %w", err)
	}
	enc.Close()
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
