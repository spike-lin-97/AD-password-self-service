// Package ldap 封装 AD/LDAP 连接、用户查询、密码修改。
//
// 关于 AD 密码修改（unicodePwd）：
//   - 值必须是「带引号的 UTF-16LE 明文」，例如 "NewPass@123"
//   - 必须在加密连接上进行（LDAPS 636 或 StartTLS），绑定账号需有「重置密码」权限
package ldap

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"github.com/go-ldap/ldap/v3"
	"golang.org/x/text/encoding/unicode"
)

// Config AD/LDAP 连接配置
type Config struct {
	Host            string
	Port            int
	TLS             bool
	Username        string // 绑定账号 DN
	Password        string
	Base            string // 搜索 base DN（通常为 FeishuSync OU）
	TimeLimit       int
	SizeLimit       int
	MobileAttr      string // 手机号字段名，默认 telephoneNumber
	NameAttr        string // 姓名匹配字段名，默认 displayName
	CAFile          string // 可选：企业 CA 证书。为空则跳过证书校验（仅内网测试）
	TLSServerName   string // 用 IP 连接时填写证书 CN/SAN
	PasswordMaxDays int    // 读不到域 maxPwdAge 时的兜底天数，默认 90
	SkipVerify      bool   // true 时不校验 AD 证书（自签/过期也能连）
}

// Client LDAP 客户端
type Client struct {
	cfg  Config
	conn *ldap.Conn
}

// NewClient 建立连接并绑定
func NewClient(cfg Config) (*Client, error) {
	if cfg.MobileAttr == "" {
		cfg.MobileAttr = "telephoneNumber"
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	conn, err := dial(cfg, addr)
	if err != nil {
		return nil, err
	}
	if err := conn.Bind(cfg.Username, cfg.Password); err != nil {
		conn.Close()
		return nil, fmt.Errorf("bind ldap failed (check bind_dn/password): %w", err)
	}
	return &Client{cfg: cfg, conn: conn}, nil
}

func dial(cfg Config, addr string) (*ldap.Conn, error) {
	if !cfg.TLS {
		conn, err := ldap.DialURL("ldap://" + addr)
		if err != nil {
			return nil, fmt.Errorf("connect ldap %s failed: %w", addr, err)
		}
		return conn, nil
	}

	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	conn, err := ldap.DialURL("ldaps://"+addr, ldap.DialWithTLSConfig(tlsCfg))
	if err != nil {
		return nil, fmt.Errorf("connect ldaps %s failed: %w", addr, err)
	}
	return conn, nil
}

func buildTLSConfig(cfg Config) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		ServerName:         cfg.TLSServerName,
		InsecureSkipVerify: cfg.SkipVerify,
	}
	tlsCfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		return nil
	}
	if cfg.CAFile == "" || cfg.SkipVerify {
		return tlsCfg, nil
	}
	pemBytes, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("read ldap ca-file: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("parse ldap ca-file: no certificates found in %s", cfg.CAFile)
	}
	tlsCfg.RootCAs = pool
	tlsCfg.InsecureSkipVerify = false
	tlsCfg.VerifyPeerCertificate = nil
	if tlsCfg.ServerName == "" {
		tlsCfg.ServerName = cfg.Host
	}
	return tlsCfg, nil
}

// Close 关闭连接
func (c *Client) Close() {
	if c != nil && c.conn != nil {
		c.conn.Close()
	}
}

var userAttrs = []string{"dn", "userPrincipalName", "displayName", "telephoneNumber", "mail", "sAMAccountName", "lockoutTime", "pwdLastSet", "userAccountControl"}

// ADUser 匹配到的 AD 用户信息
type ADUser struct {
	DN                 string
	UserPrincipalName  string
	DisplayName        string
	Mobile             string
	Email              string
	SamAccountName     string
	Locked             bool
	PwdLastSetRaw      string
	UserAccountControl int
}

func userFromEntry(entry *ldap.Entry) *ADUser {
	lockout := entry.GetAttributeValue("lockoutTime")
	uac := 0
	if raw := entry.GetAttributeValue("userAccountControl"); raw != "" {
		fmt.Sscanf(raw, "%d", &uac)
	}
	return &ADUser{
		DN:                 entry.DN,
		UserPrincipalName:  entry.GetAttributeValue("userPrincipalName"),
		DisplayName:        entry.GetAttributeValue("displayName"),
		Mobile:             entry.GetAttributeValue("telephoneNumber"),
		Email:              entry.GetAttributeValue("mail"),
		SamAccountName:     entry.GetAttributeValue("sAMAccountName"),
		Locked:             lockout != "" && lockout != "0",
		PwdLastSetRaw:      entry.GetAttributeValue("pwdLastSet"),
		UserAccountControl: uac,
	}
}

// FindByMobile 在 base 下按手机号搜索用户，返回第一个匹配项。
func (c *Client) FindByMobile(mobile string) (*ADUser, error) {
	attr := c.cfg.MobileAttr
	if attr == "" {
		attr = "telephoneNumber"
	}
	filter := fmt.Sprintf("(%s=%s)", attr, ldap.EscapeFilter(mobile))

	search := ldap.NewSearchRequest(
		c.cfg.Base,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		c.cfg.SizeLimit, c.cfg.TimeLimit, false,
		filter,
		userAttrs,
		nil,
	)
	results, err := c.conn.Search(search)
	if err != nil {
		return nil, fmt.Errorf("search user by mobile failed: %w", err)
	}
	if len(results.Entries) == 0 {
		return nil, fmt.Errorf("no AD user matched mobile=%s in %s", mobile, c.cfg.Base)
	}
	return userFromEntry(results.Entries[0]), nil
}

// FindByName 在 base 下按 displayName 搜索用户，返回唯一匹配项。
func (c *Client) FindByName(name string) (*ADUser, error) {
	attr := c.cfg.NameAttr
	if attr == "" {
		attr = "displayName"
	}
	filter := fmt.Sprintf("(%s=%s)", attr, ldap.EscapeFilter(name))

	search := ldap.NewSearchRequest(
		c.cfg.Base,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		c.cfg.SizeLimit, c.cfg.TimeLimit, false,
		filter,
		userAttrs,
		nil,
	)
	results, err := c.conn.Search(search)
	if err != nil {
		return nil, fmt.Errorf("search user by name failed: %w", err)
	}
	if len(results.Entries) == 0 {
		return nil, fmt.Errorf("no AD user matched name=%s in %s", name, c.cfg.Base)
	}
	if len(results.Entries) > 1 {
		return nil, fmt.Errorf("multiple AD users matched name=%s (%d found), cannot uniquely identify", name, len(results.Entries))
	}
	return userFromEntry(results.Entries[0]), nil
}

// FindByAccount 按 sAMAccountName 或 UPN 查找用户。
func (c *Client) FindByAccount(username string) (*ADUser, error) {
	username = strings.TrimSpace(username)
	if i := strings.LastIndex(username, `\`); i >= 0 {
		username = username[i+1:]
	}
	if username == "" {
		return nil, fmt.Errorf("username is empty")
	}
	escaped := ldap.EscapeFilter(username)
	filter := fmt.Sprintf("(|(sAMAccountName=%s)(userPrincipalName=%s))", escaped, escaped)

	search := ldap.NewSearchRequest(
		c.cfg.Base,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		c.cfg.SizeLimit, c.cfg.TimeLimit, false,
		filter,
		userAttrs,
		nil,
	)
	results, err := c.conn.Search(search)
	if err != nil {
		return nil, fmt.Errorf("search user by account failed: %w", err)
	}
	if len(results.Entries) == 0 {
		return nil, fmt.Errorf("no AD user matched account=%s in %s", username, c.cfg.Base)
	}
	if len(results.Entries) > 1 {
		return nil, fmt.Errorf("multiple AD users matched account=%s", username)
	}
	return userFromEntry(results.Entries[0]), nil
}

// Authenticate 用服务账号查出用户后，另开连接校验用户密码。
func (c *Client) Authenticate(username, password string) (*ADUser, error) {
	user, err := c.FindByAccount(username)
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port)
	conn, err := dial(c.cfg, addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := conn.Bind(user.DN, password); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return user, nil
}

// UnlockAccount 将 lockoutTime 置 0，解除账户锁定。
func (c *Client) UnlockAccount(dn string) error {
	req := ldap.NewModifyRequest(dn, nil)
	req.Replace("lockoutTime", []string{"0"})
	if err := c.conn.Modify(req); err != nil {
		return fmt.Errorf("unlock account failed: %w", err)
	}
	return nil
}

// ModifyPassword 修改指定用户（DN）的密码。
// AD 要求 unicodePwd 为带引号的 UTF-16LE 明文，例如 "NewPass@123"。
func (c *Client) ModifyPassword(dn, newPassword string) error {
	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	pwdEncoded, err := utf16.NewEncoder().String(`"` + newPassword + `"`)
	if err != nil {
		return fmt.Errorf("encode password failed: %w", err)
	}
	req := ldap.NewModifyRequest(dn, nil)
	req.Replace("unicodePwd", []string{pwdEncoded})
	if err := c.conn.Modify(req); err != nil {
		return fmt.Errorf("modify password failed (need LDAPS/StartTLS & reset-password right): %w", err)
	}
	return nil
}
