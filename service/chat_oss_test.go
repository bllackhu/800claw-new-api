package service

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestChatObjectKeyUsesTokenIDAndShanghaiDate(t *testing.T) {
	// 2026-10-14 16:30 UTC is 2026-10-15 in Asia/Shanghai.
	now := time.Date(2026, 10, 14, 16, 30, 0, 0, time.UTC)
	key, err := ChatObjectKey(7, `../hello world.docx`, now)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(key, "chat-temp/7/2026/10/15/"))
	require.True(t, strings.HasSuffix(key, "/hello world.docx"))
	require.NotContains(t, key, "sk-")
	require.True(t, ChatObjectKeyAllowed(key, 7))
	require.False(t, ChatObjectKeyAllowed(key, 8))
	require.False(t, ChatObjectKeyAllowed("chat-temp/7/../8/file", 7))
}

func TestChatOSSPresignGET(t *testing.T) {
	cfg := ChatOSSConfig{
		Endpoint:        "oss-cn-hangzhou.aliyuncs.com",
		Bucket:          "bucket",
		AccessKeyID:     "id",
		AccessKeySecret: "secret",
	}
	expires := time.Unix(1_700_000_000, 0).UTC()
	got, err := cfg.Presign("GET", "chat-temp/7/2026/10/14/abc/a.docx", "", expires)
	require.NoError(t, err)
	resource := "/bucket/chat-temp/7/2026/10/14/abc/a.docx"
	stringToSign := "GET\n\n\n1700000000\n" + resource
	mac := hmac.New(sha1.New, []byte("secret"))
	_, _ = mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	parsed, err := url.Parse(got)
	require.NoError(t, err)
	require.Equal(t, "bucket.oss-cn-hangzhou.aliyuncs.com", parsed.Host)
	require.Equal(t, "/chat-temp/7/2026/10/14/abc/a.docx", parsed.Path)
	require.Equal(t, "id", parsed.Query().Get("OSSAccessKeyId"))
	require.Equal(t, "1700000000", parsed.Query().Get("Expires"))
	require.Equal(t, signature, parsed.Query().Get("Signature"))
}

func TestGrantRejectsOversize(t *testing.T) {
	cfg := ChatOSSConfig{
		Endpoint:        "oss-cn-hangzhou.aliyuncs.com",
		Bucket:          "bucket",
		AccessKeyID:     "id",
		AccessKeySecret: "secret",
	}
	_, err := cfg.Grant(7, "a.docx", MaxChatFileBytes+1, time.Now())
	require.EqualError(t, err, "file exceeds 1GB")
	grant, err := cfg.Grant(7, "a.docx", 12, time.Date(2026, 10, 14, 1, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(grant.ObjectKey, "chat-temp/7/2026/10/14/"))
	require.Equal(t, "application/octet-stream", grant.PutHeaders["Content-Type"])
	require.Contains(t, grant.PutURL, "https://bucket.oss-cn-hangzhou.aliyuncs.com/")
	require.NotEmpty(t, grant.GetURL)
}
