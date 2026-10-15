package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const MaxChatFileBytes int64 = 1024 * 1024 * 1024

const chatFileGETTTL = 24 * time.Hour

const chatFilePUTTTL = 24 * time.Hour

type ChatOSSConfig struct {
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	AccessKeySecret string
}

type ChatFileGrant struct {
	ObjectKey  string            `json:"objectKey"`
	PutURL     string            `json:"putUrl"`
	PutHeaders map[string]string `json:"putHeaders"`
	GetURL     string            `json:"getUrl"`
}

func LoadChatOSSConfig() (ChatOSSConfig, error) {
	cfg := ChatOSSConfig{
		Endpoint:        strings.TrimSpace(common.GetEnvOrDefaultString("ALIYUN_OSS_ENDPOINT", "")),
		Bucket:          strings.TrimSpace(common.GetEnvOrDefaultString("ALIYUN_OSS_BUCKET", "")),
		AccessKeyID:     strings.TrimSpace(common.GetEnvOrDefaultString("ALIYUN_OSS_ACCESS_KEY_ID", "")),
		AccessKeySecret: strings.TrimSpace(common.GetEnvOrDefaultString("ALIYUN_OSS_ACCESS_KEY_SECRET", "")),
	}
	cfg.Endpoint = strings.TrimPrefix(strings.TrimPrefix(cfg.Endpoint, "https://"), "http://")
	cfg.Endpoint = strings.Trim(cfg.Endpoint, "/")
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.AccessKeySecret == "" {
		return ChatOSSConfig{}, fmt.Errorf("oss is not configured")
	}
	return cfg, nil
}

func ChatObjectKey(tokenID int, name string, now time.Time) (string, error) {
	if tokenID <= 0 {
		return "", fmt.Errorf("token id is required")
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	day := now.In(loc)
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"chat-temp/%d/%04d/%02d/%02d/%s/%s",
		tokenID,
		day.Year(),
		int(day.Month()),
		day.Day(),
		hex.EncodeToString(buf),
		safeChatFileName(name),
	), nil
}

func ChatObjectKeyAllowed(objectKey string, tokenID int) bool {
	if tokenID <= 0 || objectKey == "" || strings.Contains(objectKey, "..") {
		return false
	}
	prefix := fmt.Sprintf("chat-temp/%d/", tokenID)
	if !strings.HasPrefix(objectKey, prefix) {
		return false
	}
	rest := strings.TrimPrefix(objectKey, prefix)
	return rest != "" && !strings.HasPrefix(rest, "/")
}

func (cfg ChatOSSConfig) Grant(tokenID int, name string, size int64, now time.Time) (ChatFileGrant, error) {
	if size < 0 || size > MaxChatFileBytes {
		return ChatFileGrant{}, fmt.Errorf("file exceeds 1GB")
	}
	objectKey, err := ChatObjectKey(tokenID, name, now)
	if err != nil {
		return ChatFileGrant{}, err
	}
	contentType := "application/octet-stream"
	putURL, err := cfg.Presign("PUT", objectKey, contentType, now.Add(chatFilePUTTTL))
	if err != nil {
		return ChatFileGrant{}, err
	}
	getURL, err := cfg.Presign("GET", objectKey, "", now.Add(chatFileGETTTL))
	if err != nil {
		return ChatFileGrant{}, err
	}
	return ChatFileGrant{
		ObjectKey:  objectKey,
		PutURL:     putURL,
		PutHeaders: map[string]string{"Content-Type": contentType},
		GetURL:     getURL,
	}, nil
}

func (cfg ChatOSSConfig) SignGet(objectKey string, tokenID int, now time.Time) (string, error) {
	if !ChatObjectKeyAllowed(objectKey, tokenID) {
		return "", fmt.Errorf("object key is not allowed")
	}
	return cfg.Presign("GET", objectKey, "", now.Add(chatFileGETTTL))
}

func (cfg ChatOSSConfig) Presign(method, objectKey, contentType string, expiresAt time.Time) (string, error) {
	if objectKey == "" {
		return "", fmt.Errorf("object key is required")
	}
	expires := expiresAt.Unix()
	resource := "/" + cfg.Bucket + "/" + objectKey
	stringToSign := method + "\n\n" + contentType + "\n" + fmt.Sprintf("%d", expires) + "\n" + resource
	mac := hmac.New(sha1.New, []byte(cfg.AccessKeySecret))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	query := url.Values{}
	query.Set("OSSAccessKeyId", cfg.AccessKeyID)
	query.Set("Expires", fmt.Sprintf("%d", expires))
	query.Set("Signature", signature)
	return "https://" + cfg.host() + "/" + encodeObjectKey(objectKey) + "?" + query.Encode(), nil
}

func (cfg ChatOSSConfig) host() string {
	if strings.HasPrefix(cfg.Endpoint, cfg.Bucket+".") {
		return cfg.Endpoint
	}
	return cfg.Bucket + "." + cfg.Endpoint
}

func safeChatFileName(name string) string {
	base := path.Base(strings.ReplaceAll(name, "\\", "/"))
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == "/" {
		return "file"
	}
	var b strings.Builder
	for _, r := range base {
		if r < 32 || r == 127 || strings.ContainsRune("<>:\"|?*", r) {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	out := strings.Trim(b.String(), ". ")
	if out == "" {
		return "file"
	}
	return out
}

func encodeObjectKey(objectKey string) string {
	parts := strings.Split(objectKey, "/")
	encoded := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		encoded = append(encoded, url.PathEscape(part))
	}
	return strings.Join(encoded, "/")
}
