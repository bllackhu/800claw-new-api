// Package wechatpay wraps github.com/wechatpay-apiv3/wechatpay-go (Tencent-maintained API v3 SDK).
//
// Configuration (environment):
//   WECHATPAY_APP_ID                 — WeChat appid (required for Native prepay)
//   WECHATPAY_MCH_ID                 — merchant id
//   WECHATPAY_MCH_CERTIFICATE_SERIAL — merchant API cert serial number
//   WECHATPAY_MCH_API_V3_KEY         — API v3 key (32 bytes)
//   WECHATPAY_MCH_PRIVATE_KEY_PATH   — path to apiclient_key.pem (preferred)
//   WECHATPAY_MCH_PRIVATE_KEY        — PEM text (alternative to PATH)
//
// Optional — 微信支付公钥模式 (see https://pay.weixin.qq.com/doc/v3/merchant/4012154180 ):
//   WECHATPAY_PUBLIC_KEY_ID          — 微信支付公钥ID (must include PUB_KEY_ID_ prefix)
//   WECHATPAY_PUBLIC_KEY_PATH        — path to pub_key.pem from merchant platform
//   WECHATPAY_PUBLIC_KEY             — PEM text of that public key (alternative to PATH)
//
// When WECHATPAY_PUBLIC_KEY_ID is set, the client uses WithWechatPayPublicKeyAuthCipher (no platform
// certificate download). Callback verification uses the same public key only; during WeChat’s 7-day
// callback gray period, notifies still signed with the old platform certificate may fail until the
// merchant completes the switch (or set env unset to use legacy Auto mode temporarily).
package wechatpay

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

// Config holds WeChat Pay merchant settings loaded from the environment.
type Config struct {
	AppID                      string
	MchID                      string
	MchCertificateSerialNumber string
	MchAPIv3Key                string
	PrivateKey                 *rsa.PrivateKey
	// WeChat Pay 微信支付公钥 (optional). When WechatpayPublicKeyID is non-empty, public-key mode is active.
	WechatpayPublicKeyID string
	WechatpayPublicKey   *rsa.PublicKey
}

func LoadConfigFromEnv() (*Config, error) {
	appID := strings.TrimSpace(os.Getenv("WECHATPAY_APP_ID"))
	mchID := strings.TrimSpace(os.Getenv("WECHATPAY_MCH_ID"))
	serial := strings.TrimSpace(os.Getenv("WECHATPAY_MCH_CERTIFICATE_SERIAL"))
	if s := strings.TrimSpace(os.Getenv("WECHATPAY_MCH_CERTIFICATE_SERIAL_NUMBER")); s != "" {
		serial = s
	}
	apiV3 := strings.TrimSpace(os.Getenv("WECHATPAY_MCH_API_V3_KEY"))
	keyPath := strings.TrimSpace(os.Getenv("WECHATPAY_MCH_PRIVATE_KEY_PATH"))
	keyPEM := strings.TrimSpace(os.Getenv("WECHATPAY_MCH_PRIVATE_KEY"))

	pubKeyID := strings.TrimSpace(os.Getenv("WECHATPAY_PUBLIC_KEY_ID"))
	pubKeyPath := strings.TrimSpace(os.Getenv("WECHATPAY_PUBLIC_KEY_PATH"))
	pubKeyStr := strings.TrimSpace(os.Getenv("WECHATPAY_PUBLIC_KEY"))

	if (pubKeyPath != "" || pubKeyStr != "") && pubKeyID == "" {
		return nil, errors.New("WECHATPAY_PUBLIC_KEY_ID is required when WECHATPAY_PUBLIC_KEY_PATH or WECHATPAY_PUBLIC_KEY is set")
	}

	if appID == "" || mchID == "" || serial == "" || apiV3 == "" {
		return nil, nil // not configured — caller treats as disabled
	}

	var pk *rsa.PrivateKey
	var err error
	if keyPath != "" {
		pk, err = loadPrivateKeyFromFile(keyPath)
	} else if keyPEM != "" {
		pk, err = loadPrivateKeyFromPEM([]byte(keyPEM))
	} else {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var wechatPub *rsa.PublicKey
	if pubKeyID != "" {
		if pubKeyPath != "" {
			wechatPub, err = utils.LoadPublicKeyWithPath(pubKeyPath)
		} else if pubKeyStr != "" {
			wechatPub, err = utils.LoadPublicKey(pubKeyStr)
		} else {
			return nil, errors.New("WECHATPAY_PUBLIC_KEY_ID is set: provide WECHATPAY_PUBLIC_KEY_PATH or WECHATPAY_PUBLIC_KEY (PEM)")
		}
		if err != nil {
			return nil, fmt.Errorf("load WECHATPAY public key: %w", err)
		}
	}

	return &Config{
		AppID:                      appID,
		MchID:                      mchID,
		MchCertificateSerialNumber: serial,
		MchAPIv3Key:                apiV3,
		PrivateKey:                 pk,
		WechatpayPublicKeyID:       pubKeyID,
		WechatpayPublicKey:         wechatPub,
	}, nil
}

func (c *Config) IsComplete() bool {
	if c == nil || c.AppID == "" || c.MchID == "" || c.MchCertificateSerialNumber == "" ||
		c.MchAPIv3Key == "" || c.PrivateKey == nil {
		return false
	}
	if c.WechatpayPublicKeyID != "" {
		return c.WechatpayPublicKey != nil
	}
	return true
}

// UseWechatPayPublicKeyMode is true when 微信支付公钥 mode is configured (no platform cert auto-download for API).
func (c *Config) UseWechatPayPublicKeyMode() bool {
	return c != nil && c.WechatpayPublicKeyID != "" && c.WechatpayPublicKey != nil
}
