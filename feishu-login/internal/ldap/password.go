package ldap

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

const (
	windowsEpochOffset int64 = 11644473600
	ufDontExpirePasswd       = 0x10000
	maxPwdAgeNever     int64 = math.MinInt64
)

// PasswordTimes 给前端展示的密码时间信息
type PasswordTimes struct {
	LastSet     string `json:"pwd_last_set"`
	ExpireAt    string `json:"pwd_expire_at"`
	NeverExpire bool   `json:"pwd_never_expires"`
}

// FindByDN 按 DN 读取用户最新属性
func (c *Client) FindByDN(dn string) (*ADUser, error) {
	search := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		c.cfg.SizeLimit, c.cfg.TimeLimit, false,
		"(objectClass=*)",
		userAttrs,
		nil,
	)
	results, err := c.conn.Search(search)
	if err != nil {
		return nil, fmt.Errorf("search user by dn failed: %w", err)
	}
	if len(results.Entries) == 0 {
		return nil, fmt.Errorf("no AD user matched dn=%s", dn)
	}
	return userFromEntry(results.Entries[0]), nil
}

// DomainPasswordMaxAge 读取域密码最长使用期限
func (c *Client) DomainPasswordMaxAge() (time.Duration, bool, error) {
	base := domainDN(c.cfg.Base)
	search := ldap.NewSearchRequest(
		base,
		ldap.ScopeBaseObject, ldap.NeverDerefAliases,
		1, c.cfg.TimeLimit, false,
		"(objectClass=domainDNS)",
		[]string{"maxPwdAge"},
		nil,
	)
	results, err := c.conn.Search(search)
	if err != nil {
		return 0, false, err
	}
	if len(results.Entries) == 0 {
		return 0, false, fmt.Errorf("domain object not found")
	}
	raw := results.Entries[0].GetAttributeValue("maxPwdAge")
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false, err
	}
	if n == 0 || n == maxPwdAgeNever {
		return 0, true, nil
	}
	if n < 0 {
		n = -n
	}
	return time.Duration(n) * 100 * time.Nanosecond, false, nil
}

// PasswordTimesOf 计算上次改密时间和过期时间
func (c *Client) PasswordTimesOf(user *ADUser) PasswordTimes {
	loc := shanghai()
	info := PasswordTimes{LastSet: "未知", ExpireAt: "未知"}
	if user == nil {
		return info
	}
	if user.UserAccountControl&ufDontExpirePasswd != 0 {
		info.NeverExpire = true
		info.ExpireAt = "永不过期"
	}

	last, ok := filetimeToTime(user.PwdLastSetRaw)
	if !ok {
		if user.PwdLastSetRaw == "0" {
			info.LastSet = "未设置（下次登录须改密）"
		}
		if !info.NeverExpire {
			info.ExpireAt = "未知"
		}
		return info
	}
	info.LastSet = last.In(loc).Format("2006-01-02 15:04")
	if info.NeverExpire {
		return info
	}

	maxAge, domainNever, err := c.DomainPasswordMaxAge()
	if err != nil || maxAge <= 0 {
		days := c.cfg.PasswordMaxDays
		if days <= 0 {
			days = 90
		}
		maxAge = time.Duration(days) * 24 * time.Hour
	}
	if domainNever {
		info.NeverExpire = true
		info.ExpireAt = "永不过期"
		return info
	}
	expire := last.Add(maxAge)
	info.ExpireAt = expire.In(loc).Format("2006-01-02 15:04")
	if time.Now().After(expire) {
		info.ExpireAt += "（已过期）"
	}
	return info
}

func filetimeToTime(ft string) (time.Time, bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(ft), 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}, false
	}
	sec := n/10_000_000 - windowsEpochOffset
	nsec := (n % 10_000_000) * 100
	return time.Unix(sec, nsec), true
}

func domainDN(base string) string {
	upper := strings.ToUpper(base)
	i := strings.Index(upper, "DC=")
	if i < 0 {
		return base
	}
	return base[i:]
}

func shanghai() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}
