#!/bin/bash
# Example first-time server layout for bare-metal / VM (not Docker).
# Adjust APP_NAME, BASE, and the .env template for your environment.
# WeChat Pay v3: place apiclient_key.pem under ${BASE}/secrets/ (see .env block).
set -euo pipefail

APP_NAME='koooyooo-newapi'
APP_USER="${APP_NAME}"
BASE="/opt/${APP_NAME}"

echo "==> Create system user (no login shell)"
if ! id "${APP_USER}" &>/dev/null; then
  useradd --system --home-dir "${BASE}" --shell /sbin/nologin "${APP_USER}"
fi

echo "==> Directories"
install -d -o "${APP_USER}" -g "${APP_USER}" -m 0750 "${BASE}/data"
install -d -o "${APP_USER}" -g "${APP_USER}" -m 0750 "${BASE}/logs"
install -d -o "${APP_USER}" -g "${APP_USER}" -m 0750 "${BASE}/bin"
install -d -o "${APP_USER}" -g "${APP_USER}" -m 0750 "${BASE}/secrets"

echo "==> Environment file under app tree (edit secrets before start)"
ENV_FILE="${BASE}/.env"
if [[ ! -f "${ENV_FILE}" ]]; then
  install -m 0600 /dev/null "${ENV_FILE}"
  cat >"${ENV_FILE}" <<EOF
# --- edit all values ---
TZ=Asia/Shanghai
GIN_MODE=release

# MySQL — dedicated database (localhost:3306)
SQL_DSN=koooyooo_newapi:CHANGE_ME@tcp(127.0.0.1:3306)/koooyooo_newapi?charset=utf8mb4&parseTime=true&loc=Local

# Redis — localhost:6379, no password, logical DB 1 (not 0)
REDIS_CONN_STRING=redis://127.0.0.1:6379/1

# Optional: separate log DB (omit to use same DB as SQL_DSN)
# LOG_SQL_DSN=...

# Optional
# SESSION_SECRET=...
# SYNC_FREQUENCY=60

# Pool / coding plan (admin "Coding Plan" UI)
POOL_ENABLED=true
POOL_QUOTA_ENABLED=true
POOL_ROLLING_WINDOW_ENABLED=true

# --- Native WeChat Pay API v3 (token pool monthly subscription) ---
# Required only if you use POST /api/usage/token/pool/subscription/wechat/checkout.
# Notify URL registered at WeChat must be HTTPS and must match what the API
# advertises: {ServerAddress or custom callback from admin UI}/api/payment/wechat/notify
#
# Copy apiclient_key.pem from the merchant platform to:
#   ${BASE}/secrets/apiclient_key.pem
# then chown ${APP_USER}:${APP_USER} and chmod 0640 (or 0600).
WECHATPAY_APP_ID=
WECHATPAY_MCH_ID=
WECHATPAY_MCH_CERTIFICATE_SERIAL=
WECHATPAY_MCH_API_V3_KEY=
WECHATPAY_MCH_PRIVATE_KEY_PATH=${BASE}/secrets/apiclient_key.pem
# Alternative if you cannot use a file (less ideal):
# WECHATPAY_MCH_PRIVATE_KEY='-----BEGIN PRIVATE KEY-----...'

# --- 微信支付公钥 (recommended; avoids platform certificate download / 4012154180) ---
# Download pub_key.pem + 公钥ID from 商户平台 → 账户中心 → API安全 → 微信支付公钥
# WECHATPAY_PUBLIC_KEY_ID=PUB_KEY_ID_xxxxxxxx
# WECHATPAY_PUBLIC_KEY_PATH=${BASE}/secrets/wechatpay_pub_key.pem

EOF
  chown root:root "${ENV_FILE}"
  chmod 0600 "${ENV_FILE}"
  echo "Created ${ENV_FILE} — edit SQL_DSN, Redis, and WeChat Pay vars as needed."
fi

echo "==> Done under ${BASE}"
echo "Next: copy apiclient_key.pem to ${BASE}/secrets/, fix ownership, then:"
echo "      scp new-api to /tmp/new-api && bash scripts/deploy/02-deploy-binary.sh /tmp/new-api"
