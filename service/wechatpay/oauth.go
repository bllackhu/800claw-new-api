package wechatpay

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	wechatTokenURL       = "https://api.weixin.qq.com/cgi-bin/token"
	wechatTicketURL      = "https://api.weixin.qq.com/cgi-bin/ticket/getticket"
	wechatOAuthAccessURL = "https://api.weixin.qq.com/sns/oauth2/access_token"
)

type tokenCache struct {
	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
	jsapiTicket string
	ticketExp   time.Time
}

var globalTokenCache tokenCache

type wechatTokenResp struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
}

type wechatTicketResp struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int    `json:"expires_in"`
	ErrCode   int    `json:"errcode"`
	ErrMsg    string `json:"errmsg"`
}

type wechatOAuthAccessResp struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Openid       string `json:"openid"`
	Scope        string `json:"scope"`
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
}

func getAccessToken(appID, appSecret string) (string, error) {
	globalTokenCache.mu.Lock()
	defer globalTokenCache.mu.Unlock()

	if globalTokenCache.accessToken != "" && time.Now().Before(globalTokenCache.expiresAt) {
		return globalTokenCache.accessToken, nil
	}

	u, _ := url.Parse(wechatTokenURL)
	q := u.Query()
	q.Set("grant_type", "client_credential")
	q.Set("appid", appID)
	q.Set("secret", appSecret)
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("get access_token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tr wechatTokenResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("parse access_token response: %w", err)
	}
	if tr.ErrCode != 0 {
		return "", fmt.Errorf("wechat access_token error: %d %s", tr.ErrCode, tr.ErrMsg)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("empty access_token")
	}

	globalTokenCache.accessToken = tr.AccessToken
	globalTokenCache.expiresAt = time.Now().Add(time.Duration(tr.ExpiresIn-60) * time.Second)
	return tr.AccessToken, nil
}

func getJsapiTicket(appID, appSecret string) (string, error) {
	globalTokenCache.mu.Lock()
	if globalTokenCache.jsapiTicket != "" && time.Now().Before(globalTokenCache.ticketExp) {
		ticket := globalTokenCache.jsapiTicket
		globalTokenCache.mu.Unlock()
		return ticket, nil
	}

	accessToken := globalTokenCache.accessToken
	expiresAt := globalTokenCache.expiresAt
	globalTokenCache.mu.Unlock()

	if accessToken == "" || time.Now().After(expiresAt) {
		var err error
		accessToken, err = getAccessToken(appID, appSecret)
		if err != nil {
			return "", err
		}
	}

	u, _ := url.Parse(wechatTicketURL)
	q := u.Query()
	q.Set("access_token", accessToken)
	q.Set("type", "jsapi")
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("get jsapi_ticket: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tr wechatTicketResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("parse jsapi_ticket response: %w", err)
	}
	if tr.ErrCode != 0 {
		return "", fmt.Errorf("wechat jsapi_ticket error: %d %s", tr.ErrCode, tr.ErrMsg)
	}
	if tr.Ticket == "" {
		return "", fmt.Errorf("empty jsapi_ticket")
	}

	globalTokenCache.mu.Lock()
	globalTokenCache.jsapiTicket = tr.Ticket
	globalTokenCache.ticketExp = time.Now().Add(time.Duration(tr.ExpiresIn-60) * time.Second)
	globalTokenCache.mu.Unlock()

	return tr.Ticket, nil
}

func GetOpenid(appID, appSecret, code string) (string, error) {
	if code == "" {
		return "", fmt.Errorf("empty oauth code")
	}
	if appID == "" || appSecret == "" {
		return "", fmt.Errorf("wechat app_id or app_secret not configured")
	}

	u, _ := url.Parse(wechatOAuthAccessURL)
	q := u.Query()
	q.Set("appid", appID)
	q.Set("secret", appSecret)
	q.Set("code", code)
	q.Set("grant_type", "authorization_code")
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("get openid: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var oa wechatOAuthAccessResp
	if err := json.Unmarshal(body, &oa); err != nil {
		return "", fmt.Errorf("parse oauth response: %w", err)
	}
	if oa.ErrCode != 0 {
		return "", fmt.Errorf("wechat oauth error: %d %s", oa.ErrCode, oa.ErrMsg)
	}
	if oa.Openid == "" {
		return "", fmt.Errorf("empty openid")
	}
	return oa.Openid, nil
}

func GenerateJsapiSignature(jsapiTicket, nonceStr, timestamp, pageURL string) string {
	raw := fmt.Sprintf("jsapi_ticket=%s&noncestr=%s&timestamp=%s&url=%s",
		jsapiTicket, nonceStr, timestamp, pageURL)
	h := sha1.New()
	h.Write([]byte(raw))
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateChooseWxPaySign(appID string, timestamp int64, nonceStr, prepayID string, privateKey *rsa.PrivateKey) (string, error) {
	packageStr := "prepay_id=" + prepayID
	ts := strconv.FormatInt(timestamp, 10)

	raw := fmt.Sprintf("%s\n%s\n%s\n%s\n", appID, ts, nonceStr, packageStr)

	h := sha256.New()
	h.Write([]byte(raw))
	digest := h.Sum(nil)

	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest)
	if err != nil {
		return "", fmt.Errorf("rsa sign: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func GenerateNonceStr() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 16)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return string(b)
}

func GenerateJsapiConfig(appID, appSecret string, timestamp int64, nonceStr, pageURL string) (signature string, err error) {
	ticket, err := getJsapiTicket(appID, appSecret)
	if err != nil {
		return "", err
	}
	return GenerateJsapiSignature(ticket, nonceStr, strconv.FormatInt(timestamp, 10), pageURL), nil
}

func GenerateJsapiPayParams(appID string, prepayID string, privateKey *rsa.PrivateKey) (timestamp int64, nonceStr, packageStr, signType, paySign string, err error) {
	timestamp = time.Now().Unix()
	nonceStr = GenerateNonceStr()
	packageStr = "prepay_id=" + prepayID
	signType = "RSA"

	ts := strconv.FormatInt(timestamp, 10)
	raw := fmt.Sprintf("%s\n%s\n%s\n%s\n", appID, ts, nonceStr, packageStr)

	h := sha256.New()
	h.Write([]byte(raw))
	digest := h.Sum(nil)

	sig, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest)
	if err != nil {
		return 0, "", "", "", "", fmt.Errorf("rsa sign: %w", err)
	}
	paySign = base64.StdEncoding.EncodeToString(sig)
	return
}

func GetJsapiAppID(cfg *Config) (string, error) {
	if cfg == nil || cfg.AppID == "" {
		return "", fmt.Errorf("wechat app_id not configured")
	}
	if cfg.AppSecret == "" {
		return "", fmt.Errorf("wechat app_secret not configured")
	}
	return cfg.AppID, nil
}

func nonceStr() string {
	return GenerateNonceStr()
}

func SnapNonceStr() string {
	return nonceStr()
}

func IsJsapiConfigured(cfg *Config) bool {
	return cfg != nil && cfg.AppID != "" && cfg.AppSecret != "" && cfg.IsComplete()
}

func StripTrailingHash(pageURL string) string {
	if idx := strings.Index(pageURL, "#"); idx >= 0 {
		return pageURL[:idx]
	}
	return pageURL
}