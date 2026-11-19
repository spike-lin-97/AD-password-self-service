package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config 飞书客户端配置
type Config struct {
	AppID       string
	AppSecret   string
	RedirectURL string
}

// TokenResponse 用 code 换 user_access_token 的响应
type TokenResponse struct {
	Code         int    `json:"code"`
	Msg          string `json:"msg"`
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	UserId       string `json:"user_id"`
	UnionId      string `json:"union_id"`
	OpenId       string `json:"open_id"`
}

// UserInfo 飞书用户信息（扁平结构，对应实际 API 返回）
type UserInfo struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		UserId     string `json:"user_id"`
		UnionId    string `json:"union_id"`
		OpenId     string `json:"open_id"`
		Name       string `json:"name"`
		EnName     string `json:"en_name"`
		Email      string `json:"email"`
		Mobile     string `json:"mobile"`
		AvatarURL  string `json:"avatar_url"`
		EmployeeNo string `json:"employee_no"`
	} `json:"data"`
}

// Client 飞书客户端
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// NewClient 创建飞书客户端
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// AuthorizeURL 本机飞书一键授权（跳转飞书授权页，已登录客户端可直接确认）
func (c *Client) AuthorizeURL(state string) string {
	v := url.Values{}
	v.Set("client_id", c.cfg.AppID)
	v.Set("redirect_uri", c.cfg.RedirectURL)
	v.Set("state", state)
	v.Set("response_type", "code")
	return "https://open.feishu.cn/open-apis/authen/v1/index?" + v.Encode()
}

// QRAuthorizeURL 页内二维码 SDK 使用的旧版授权地址
func (c *Client) QRAuthorizeURL(state string) string {
	v := url.Values{}
	v.Set("client_id", c.cfg.AppID)
	v.Set("redirect_uri", c.cfg.RedirectURL)
	v.Set("response_type", "code")
	v.Set("state", state)
	return "https://passport.feishu.cn/suite/passport/oauth/authorize?" + v.Encode()
}

// ExchangeToken 用授权码换取 user_access_token（新版接口）
func (c *Client) ExchangeToken(ctx context.Context, code string) (*TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", c.cfg.AppID)
	form.Set("client_secret", c.cfg.AppSecret)
	form.Set("redirect_uri", c.cfg.RedirectURL)
	form.Set("code", code)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://accounts.feishu.cn/oauth/v3/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse token resp failed: %s, body: %s", err, string(body))
	}
	if tokenResp.Code != 0 {
		return nil, fmt.Errorf("feishu token error: code=%d msg=%s", tokenResp.Code, tokenResp.Msg)
	}
	return &tokenResp, nil
}

// GetUserInfo 用 user_access_token 获取用户信息（含手机号）
func (c *Client) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://open.feishu.cn/open-apis/authen/v1/user_info", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var userResp UserInfo
	if err := json.Unmarshal(body, &userResp); err != nil {
		return nil, fmt.Errorf("parse user info failed: %s, body: %s", err, string(body))
	}
	if userResp.Code != 0 {
		return nil, fmt.Errorf("get user info failed: code=%d msg=%s", userResp.Code, userResp.Msg)
	}
	return &userResp, nil
}

// TestApp 用 app_id/app_secret 换 tenant_access_token，验证飞书应用凭证。
func TestApp(appID, appSecret string) (string, error) {
	if appID == "" || appSecret == "" {
		return "", fmt.Errorf("app_id / app_secret 不能为空")
	}
	body, _ := json.Marshal(map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
	})
	req, err := http.NewRequest(http.MethodPost,
		"https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal",
		strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out struct {
		Code              int    `json:"code"`
		Msg               string `json:"msg"`
		TenantAccessToken string `json:"tenant_access_token"`
		Expire            int    `json:"expire"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("parse feishu resp failed: %s", string(raw))
	}
	if out.Code != 0 || out.TenantAccessToken == "" {
		return "", fmt.Errorf("飞书校验失败: code=%d msg=%s", out.Code, out.Msg)
	}
	return fmt.Sprintf("飞书应用凭证有效，token 约 %d 秒后过期", out.Expire), nil
}

// NormalizeMobile 清洗手机号：去 +86 / 空格 / 横线 / 括号，只留数字
func NormalizeMobile(mobile string) string {
	mobile = strings.ReplaceAll(mobile, "+86", "")
	mobile = strings.ReplaceAll(mobile, " ", "")
	mobile = strings.ReplaceAll(mobile, "-", "")
	mobile = strings.ReplaceAll(mobile, "(", "")
	mobile = strings.ReplaceAll(mobile, ")", "")
	return mobile
}
