package ldap

import (
	"crypto/x509"
	"fmt"
	"time"

	golap "github.com/go-ldap/ldap/v3"
)

// CertInfo AD 服务器出示的 TLS 证书摘要
type CertInfo struct {
	Subject   string   `json:"subject"`
	Issuer    string   `json:"issuer"`
	NotBefore string   `json:"not_before"`
	NotAfter  string   `json:"not_after"`
	DNSNames  []string `json:"dns_names,omitempty"`
	Expired   bool     `json:"expired"`
	DaysLeft  int      `json:"days_left"`
}

// ProbeResult AD 连通测试结果
type ProbeResult struct {
	OK         bool      `json:"ok"`
	Protocol   string    `json:"protocol"`
	Address    string    `json:"address"`
	SkipVerify bool      `json:"skip_verify"`
	BindOK     bool      `json:"bind_ok"`
	SearchOK   bool      `json:"search_ok"`
	Message    string    `json:"message"`
	Cert       *CertInfo `json:"cert,omitempty"`
}

// Probe 测试 LDAPS/LDAP 连接、绑定、搜索，并读取对端证书（即使跳过校验）。
func Probe(cfg Config) ProbeResult {
	if cfg.Port == 0 {
		if cfg.TLS {
			cfg.Port = 636
		} else {
			cfg.Port = 389
		}
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	res := ProbeResult{
		Protocol:   "ldap",
		Address:    addr,
		SkipVerify: cfg.SkipVerify,
	}
	if cfg.TLS {
		res.Protocol = "ldaps"
	}

	var peer *x509.Certificate
	conn, err := dialWithPeer(cfg, addr, &peer)
	if peer != nil {
		res.Cert = certInfo(peer)
	}
	if err != nil {
		res.Message = "连接失败: " + err.Error()
		if res.Cert != nil && res.Cert.Expired {
			res.Message += "。对端证书已过期。"
		}
		return res
	}
	defer conn.Close()

	if err := conn.Bind(cfg.Username, cfg.Password); err != nil {
		res.Message = "TLS 已通，绑定失败（请检查账号/密码/DN）: " + err.Error()
		return res
	}
	res.BindOK = true

	if cfg.Base == "" {
		res.OK = true
		res.Message = summarizeProbe(cfg, res, "连接并绑定成功")
		return res
	}

	req := golap.NewSearchRequest(
		cfg.Base,
		golap.ScopeBaseObject, golap.NeverDerefAliases,
		1, cfg.TimeLimit, false,
		"(objectClass=*)",
		[]string{"distinguishedName"},
		nil,
	)
	if _, err := conn.Search(req); err != nil {
		res.Message = "绑定成功，但搜索 Base DN 失败: " + err.Error()
		return res
	}
	res.SearchOK = true
	res.OK = true
	res.Message = summarizeProbe(cfg, res, "连接、绑定、搜索均成功")
	return res
}

func dialWithPeer(cfg Config, addr string, peer **x509.Certificate) (*golap.Conn, error) {
	if !cfg.TLS {
		return golap.DialURL("ldap://" + addr)
	}
	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		return nil, err
	}
	tlsCfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) > 0 && peer != nil {
			if c, err := x509.ParseCertificate(rawCerts[0]); err == nil {
				*peer = c
			}
		}
		return nil
	}
	return golap.DialURL("ldaps://"+addr, golap.DialWithTLSConfig(tlsCfg))
}

func summarizeProbe(cfg Config, res ProbeResult, base string) string {
	msg := base
	if res.Cert != nil {
		if res.Cert.Expired {
			msg += "。服务器证书已过期"
			if cfg.SkipVerify {
				msg += "（当前跳过校验，所以仍能连上）"
			}
		} else if res.Cert.DaysLeft <= 30 {
			msg += fmt.Sprintf("。证书将在 %d 天后过期", res.Cert.DaysLeft)
		} else {
			msg += fmt.Sprintf("。证书有效期至 %s", res.Cert.NotAfter)
		}
	}
	if cfg.SkipVerify {
		msg += "。当前跳过 TLS 证书校验：自签或过期证书一般不会导致 AD 连不上。"
	}
	return msg
}

func certInfo(c *x509.Certificate) *CertInfo {
	now := time.Now()
	days := int(c.NotAfter.Sub(now).Hours() / 24)
	loc := shanghai()
	return &CertInfo{
		Subject:   c.Subject.String(),
		Issuer:    c.Issuer.String(),
		NotBefore: c.NotBefore.In(loc).Format("2006-01-02 15:04"),
		NotAfter:  c.NotAfter.In(loc).Format("2006-01-02 15:04"),
		DNSNames:  c.DNSNames,
		Expired:   now.After(c.NotAfter),
		DaysLeft:  days,
	}
}
