package wechatpay

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/downloader"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
)

func loadPrivateKeyFromFile(path string) (*rsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadPrivateKeyFromPEM(b)
}

func loadPrivateKeyFromPEM(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("invalid PEM")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		pk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("expected RSA private key, got %T", k)
		}
		return pk, nil
	default:
		return nil, fmt.Errorf("unsupported PEM type %q", block.Type)
	}
}

var (
	clientMu   sync.Mutex
	clientInst *core.Client
	clientCfg  *Config
	clientErr  error
)

// Client returns a singleton WeChat Pay API v3 client, or an error if not configured.
func Client(ctx context.Context) (*core.Client, *Config, error) {
	clientMu.Lock()
	defer clientMu.Unlock()
	if clientInst != nil {
		return clientInst, clientCfg, nil
	}
	if clientErr != nil {
		return nil, nil, clientErr
	}
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		clientErr = err
		return nil, nil, err
	}
	if cfg == nil || !cfg.IsComplete() {
		clientErr = errors.New("wechat pay is not configured")
		return nil, nil, clientErr
	}
	var c *core.Client
	if cfg.UseWechatPayPublicKeyMode() {
		c, err = core.NewClient(ctx, option.WithWechatPayPublicKeyAuthCipher(
			cfg.MchID, cfg.MchCertificateSerialNumber, cfg.PrivateKey,
			cfg.WechatpayPublicKeyID, cfg.WechatpayPublicKey,
		))
	} else {
		c, err = core.NewClient(ctx, option.WithWechatPayAutoAuthCipher(
			cfg.MchID, cfg.MchCertificateSerialNumber, cfg.PrivateKey, cfg.MchAPIv3Key,
		))
	}
	if err != nil {
		clientErr = err
		return nil, nil, err
	}
	clientInst = c
	clientCfg = cfg
	return clientInst, clientCfg, nil
}

// ResetClientForTests clears the singleton (tests only).
func ResetClientForTests() {
	clientMu.Lock()
	defer clientMu.Unlock()
	clientInst = nil
	clientCfg = nil
	clientErr = nil
}

// NativePrepay creates a Native (QR) order and returns the code_url.
func NativePrepay(ctx context.Context, cfg *Config, client *core.Client, notifyURL, outTradeNo, description string, totalFen int64) (codeURL string, err error) {
	if totalFen <= 0 {
		return "", errors.New("invalid amount")
	}
	svc := native.NativeApiService{Client: client}
	resp, _, err := svc.Prepay(ctx, native.PrepayRequest{
		Appid:       core.String(cfg.AppID),
		Mchid:       core.String(cfg.MchID),
		Description: core.String(description),
		OutTradeNo:  core.String(outTradeNo),
		NotifyUrl:   core.String(notifyURL),
		Amount: &native.Amount{
			Total:    core.Int64(totalFen),
			Currency: core.String("CNY"),
		},
	})
	if err != nil {
		return "", err
	}
	if resp == nil || resp.CodeUrl == nil {
		return "", errors.New("empty code_url from wechat pay")
	}
	return *resp.CodeUrl, nil
}

// QueryTransactionByOutTradeNo loads order state from WeChat by merchant out_trade_no.
func QueryTransactionByOutTradeNo(ctx context.Context, cfg *Config, client *core.Client, outTradeNo string) (*payments.Transaction, error) {
	if outTradeNo == "" {
		return nil, errors.New("empty out_trade_no")
	}
	if cfg == nil || client == nil {
		return nil, errors.New("wechat pay client not configured")
	}
	svc := native.NativeApiService{Client: client}
	resp, _, err := svc.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(outTradeNo),
		Mchid:      core.String(cfg.MchID),
	})
	return resp, err
}

// ParsePaymentNotify verifies and decrypts a payment notification into payments.Transaction.
// Legacy (platform certificate) mode: call Client() once before notifies so the certificate downloader is registered.
// 微信支付公钥 mode: verifies with WECHATPAY_PUBLIC_KEY only (see Config.UseWechatPayPublicKeyMode).
func ParsePaymentNotify(ctx context.Context, cfg *Config, r *http.Request) (*notify.Request, *payments.Transaction, error) {
	var handler *notify.Handler
	if cfg != nil && cfg.UseWechatPayPublicKeyMode() {
		handler = notify.NewNotifyHandler(cfg.MchAPIv3Key,
			verifiers.NewSHA256WithRSAPubkeyVerifier(cfg.WechatpayPublicKeyID, *cfg.WechatpayPublicKey))
	} else {
		certVisitor := downloader.MgrInstance().GetCertificateVisitor(cfg.MchID)
		handler = notify.NewNotifyHandler(cfg.MchAPIv3Key, verifiers.NewSHA256WithRSAVerifier(certVisitor))
	}
	tx := new(payments.Transaction)
	nreq, err := handler.ParseNotifyRequest(ctx, r, tx)
	if err != nil {
		return nil, nil, err
	}
	return nreq, tx, nil
}
