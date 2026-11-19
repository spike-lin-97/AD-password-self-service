// Package audit 记录用户自助操作（改密、解锁、登录），供管理后台查阅。
package audit

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Record 一条操作日志
type Record struct {
	ID          string `json:"id"`
	Time        string `json:"time"`
	IP          string `json:"ip"`
	MAC         string `json:"mac"`
	OS          string `json:"os"`
	UserAgent   string `json:"user_agent"`
	ADAccount   string `json:"ad_account"`
	DisplayName string `json:"display_name"`
	Operation   string `json:"operation"`
	ExpireAt    string `json:"expire_at"`
}

const (
	OpChangePassword = "修改密码"
	OpUnlock         = "解锁账户"
	OpFeishuLogin    = "飞书登录"
	OpADLogin        = "AD 登录"
)

// Store JSONL 追加存储
type Store struct {
	mu   sync.Mutex
	path string
}

// New 创建日志库，必要时建立目录
func New(path string) (*Store, error) {
	if path == "" {
		path = "var/audit.jsonl"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create audit dir: %w", err)
	}
	return &Store{path: path}, nil
}

// Path 日志文件路径
func (s *Store) Path() string {
	return s.path
}

// Append 写入一条记录。失败只返回 error，调用方不应阻断业务。
func (s *Store) Append(rec Record) error {
	if rec.ID == "" {
		rec.ID = newID()
	}
	if rec.Time == "" {
		rec.Time = nowCST()
	}
	if rec.MAC == "" {
		rec.MAC = "—"
	}
	if rec.OS == "" {
		rec.OS = ParseOS(rec.UserAgent)
	}
	if rec.OS == "" {
		rec.OS = "—"
	}
	if rec.ExpireAt == "" {
		rec.ExpireAt = "—"
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

// FromRequest 根据 HTTP 请求补全 IP / MAC
func FromRequest(c *gin.Context, op, account, display, expireAt string) Record {
	ip := ClientIP(c)
	ua := ""
	if c != nil {
		ua = c.GetHeader("User-Agent")
	}
	return Record{
		IP:          ip,
		MAC:         LookupMAC(ip),
		OS:          ParseOS(ua),
		UserAgent:   ua,
		ADAccount:   account,
		DisplayName: display,
		Operation:   op,
		ExpireAt:    expireAt,
	}
}

// List 从新到旧返回记录。q 匹配账号、姓名、IP、MAC、操作。
func (s *Store) List(q string, offset, limit int) ([]Record, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Record{}, 0, nil
		}
		return nil, 0, err
	}
	defer f.Close()

	q = strings.ToLower(strings.TrimSpace(q))
	var all []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec Record
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if q != "" && !matchRecord(rec, q) {
			continue
		}
		all = append(all, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, 0, err
	}
	total := len(all)
	// 文件是旧→新，页面要新→旧
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if offset >= total {
		return []Record{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func matchRecord(rec Record, q string) bool {
	blob := strings.ToLower(strings.Join([]string{
		rec.ADAccount, rec.DisplayName, rec.IP, rec.MAC, rec.OS, rec.UserAgent, rec.Operation, rec.Time, rec.ExpireAt,
	}, " "))
	return strings.Contains(blob, q)
}

// ClientIP 取客户端 IP（尊重 X-Forwarded-For / X-Real-IP）
func ClientIP(c *gin.Context) string {
	if c == nil {
		return ""
	}
	ip := strings.TrimSpace(c.ClientIP())
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return ip
}

// LookupMAC 从本机 ARP/邻居表查 MAC。仅当客户端与服务器在同一二层网段时才可能有值。
func LookupMAC(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" || ip == "127.0.0.1" || ip == "::1" {
		return "—"
	}
	if parsed := net.ParseIP(ip); parsed != nil {
		if v4 := parsed.To4(); v4 != nil {
			ip = v4.String()
		}
	}
	if mac := lookupProcARP(ip); mac != "" {
		return mac
	}
	return "—"
}

func lookupProcARP(ip string) string {
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return ""
	}
	for i, line := range strings.Split(string(data), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if fields[0] != ip {
			continue
		}
		mac := strings.ToLower(fields[3])
		if mac == "00:00:00:00:00:00" {
			return ""
		}
		if _, err := net.ParseMAC(mac); err != nil {
			return ""
		}
		return mac
	}
	return ""
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func nowCST() string {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return time.Now().In(loc).Format("2006-01-02 15:04:05")
}
