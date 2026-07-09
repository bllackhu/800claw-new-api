# Payment paths in new-api (reference)

Native WeChat Pay uses **`github.com/wechatpay-apiv3/wechatpay-go`** (pinned in `go.mod`, Tencent-maintained API v3 SDK).

## EPay (易支付) — user balance top-up

- **Entry:** [`RequestEpay`](D:/dev/cd/claw_dev/800claw-new-api/controller/topup.go) — user picks amount and `payment_method` (e.g. `alipay`, `wxpay` here means the **aggregator’s** WeChat channel string, **not** native WeChat Pay API).
- **Order:** [`model.TopUp`](D:/dev/cd/claw_dev/800claw-new-api/model/topup.go), `trade_no` prefix `USR...`.
- **Notify:** [`EpayNotify`](D:/dev/cd/claw_dev/800claw-new-api/controller/topup.go) → `GET/POST /api/user/epay/notify` — verify with go-epay client, then **`model.IncreaseUserQuota`**.

## EPay — user subscription (fixed plan price)

- **Entry:** [`SubscriptionRequestEpay`](D:/dev/cd/claw_dev/800claw-new-api/controller/subscription_payment_epay.go).
- **Order:** `SubscriptionOrder`, `trade_no` prefix `SUBUSR...`.
- **Notify:** `/api/subscription/epay/notify` → **`model.CompleteSubscriptionOrder`**.

## Native WeChat Pay v3 — token pool subscription

- **Example bare-metal layout / `.env` template:** [`scripts/deploy/01-setup-server.sh`](../scripts/deploy/01-setup-server.sh) (includes `WECHATPAY_*` and pool flags); binary install: [`scripts/deploy/02-deploy-binary.sh`](../scripts/deploy/02-deploy-binary.sh).
- **Separate rail:** WeChat Pay API v3 via `wechatpay-go` (≥ v0.2.20 for public-key mode); merchant credentials from env (see `service/wechatpay` package). Optional **`WECHATPAY_PUBLIC_KEY_ID`** + **`WECHATPAY_PUBLIC_KEY_PATH`** enable [微信支付公钥](https://pay.weixin.qq.com/doc/v3/merchant/4012154180) instead of auto-downloaded platform certificates (avoids `平台证书已过期` class errors on API init when switching).
- **Order:** `token_pool_subscription_orders`; fulfillment updates **`token_pool_subscriptions`** for `(token_id, pool_id)` access windows.
- **First-request free window:** when a request with `require_pool_subscription = true` hits a priced pool for the very first time and no `token_pool_subscriptions` row exists yet, the relay path auto-grants a one-time trial (`period_end` at 23:59:59 Asia/Shanghai on `now + pool.billing_period_seconds` days, defaults to 30 days) instead of returning 402. See [`docs-new-api/pool-subscription-gate-rollout.md`](https://github.com/QuantumNous/800claw-admin/blob/main/docs-new-api/pool-subscription-gate-rollout.md#2a-first-request-free-window-auto-granted-trial) in the 800claw-admin repo for the full behavior and rollout notes.
- **Notify:** `POST /api/payment/wechat/notify` — SDK `notify.Handler` verify + decrypt; idempotent completion (reuse `LockOrder` pattern from top-up). In public-key mode, verification uses the configured **微信支付公钥** only (WeChat’s 7-day callback gray period may still send some platform-cert-signed notifies until migration completes; see WeChat doc §4.4).
