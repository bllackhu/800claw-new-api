#!/usr/bin/env bash
# Verify WeChat Pay v3 environment for new-api under systemd (CentOS / RHEL family).
#
# Prefers the *live* process environment (same as the running binary sees).
# Falls back to parsing EnvironmentFile if the service is not active.
#
# Usage (on the server, as root):
#   sudo bash ./check-koooyooo-newapi-wechatpay-env.sh
#
# Optional overrides:
#   SERVICE=koooyooo-newapi ENV_FILE=/opt/koooyooo-newapi/.env SERVICE_USER=koooyooo-newapi \
#     bash ./check-koooyooo-newapi-wechatpay-env.sh

set -euo pipefail

SERVICE="${SERVICE:-koooyooo-newapi}"
ENV_FILE="${ENV_FILE:-/opt/koooyooo-newapi/.env}"
SERVICE_USER="${SERVICE_USER:-koooyooo-newapi}"

RED=$'\033[0;31m'
GRN=$'\033[0;32m'
YLW=$'\033[0;33m'
RST=$'\033[0m'

ok() { echo "${GRN}OK${RST}  $*"; }
warn() { echo "${YLW}WARN${RST} $*"; }
bad() { echo "${RED}MISSING or INVALID${RST} $*"; }

get_from_proc_environ() {
  local pid="$1" key="$2"
  tr '\0' '\n' <"/proc/${pid}/environ" 2>/dev/null | awk -F= -v k="$key" '$1==k { sub(/^[^=]+=/, ""); print; exit }'
}

# Best-effort parse of KEY=value from EnvironmentFile (only if live env unavailable).
# Does not support multiline values; strips optional surrounding quotes.
get_from_env_file() {
  local key="$1"
  local line
  line="$(grep -E "^[[:space:]]*${key}=" "$ENV_FILE" 2>/dev/null | tail -n1)" || true
  [[ -n "$line" ]] || return 1
  local val="${line#*=}"
  val="${val#"${val%%[![:space:]]*}"}"
  val="${val%"${val##*[![:space:]]}"}"
  if [[ "$val" == \"*\" ]]; then val="${val#\"}"; val="${val%\"}"; fi
  if [[ "$val" == \'*\' ]]; then val="${val#\'}"; val="${val%\'}"; fi
  printf '%s' "$val"
}

echo "==> Service: ${SERVICE}"
echo "==> EnvironmentFile (from unit): ${ENV_FILE}"

if ! systemctl cat "${SERVICE}.service" &>/dev/null; then
  echo "${RED}Unit ${SERVICE}.service not found.${RST}"
  exit 1
fi

ACTIVE="$(systemctl is-active "${SERVICE}" 2>/dev/null || true)"
MAINPID="$(systemctl show "${SERVICE}" -p MainPID --value 2>/dev/null || echo 0)"
# MainPID may be 0 or empty when stopped
if [[ -z "${MAINPID}" || "${MAINPID}" == "0" ]]; then
  MAINPID=""
fi

SOURCE=""
if [[ "${ACTIVE}" == "active" && -n "${MAINPID}" && -r "/proc/${MAINPID}/environ" ]]; then
  SOURCE="process:${MAINPID}"
  echo "==> Reading env from ${SOURCE} (authoritative)"
else
  SOURCE="file:${ENV_FILE}"
  echo "==> Service not running or no MainPID; reading ${SOURCE} (fallback)"
  if [[ ! -f "${ENV_FILE}" ]]; then
    echo "${RED}Env file missing: ${ENV_FILE}${RST}"
    exit 1
  fi
fi

lookup() {
  local key="$1"
  if [[ "${SOURCE}" == process:* ]]; then
    local pid="${SOURCE#process:}"
    get_from_proc_environ "$pid" "$key"
  else
    get_from_env_file "$key" || true
  fi
}

# Serial: support alternate name used in code
SERIAL="$(lookup WECHATPAY_MCH_CERTIFICATE_SERIAL)"
if [[ -z "${SERIAL}" ]]; then
  SERIAL="$(lookup WECHATPAY_MCH_CERTIFICATE_SERIAL_NUMBER)"
fi

APP_ID="$(lookup WECHATPAY_APP_ID)"
MCH_ID="$(lookup WECHATPAY_MCH_ID)"
APIV3="$(lookup WECHATPAY_MCH_API_V3_KEY)"
KEY_PATH="$(lookup WECHATPAY_MCH_PRIVATE_KEY_PATH)"
KEY_INLINE="$(lookup WECHATPAY_MCH_PRIVATE_KEY)"
PUB_ID="$(lookup WECHATPAY_PUBLIC_KEY_ID)"
PUB_PATH="$(lookup WECHATPAY_PUBLIC_KEY_PATH)"
PUB_INLINE="$(lookup WECHATPAY_PUBLIC_KEY)"

echo ""
echo "--- Required variables ---"

fail=0

check_nonempty() {
  local name="$1" val="$2"
  if [[ -n "${val// }" ]]; then
    ok "${name} is set"
  else
    bad "${name} empty or whitespace only"
    fail=1
  fi
}

check_nonempty "WECHATPAY_APP_ID" "$APP_ID"
check_nonempty "WECHATPAY_MCH_ID" "$MCH_ID"

if [[ -n "${SERIAL// }" ]]; then
  ok "WECHATPAY_MCH_CERTIFICATE_SERIAL (or _NUMBER) is set"
else
  bad "WECHATPAY_MCH_CERTIFICATE_SERIAL / WECHATPAY_MCH_CERTIFICATE_SERIAL_NUMBER"
  fail=1
fi

check_nonempty "WECHATPAY_MCH_API_V3_KEY" "$APIV3"

if [[ -n "$APIV3" ]]; then
  n="${#APIV3}"
  if [[ "$n" -eq 32 ]]; then
    ok "WECHATPAY_MCH_API_V3_KEY length is 32 (expected for WeChat v3 key)"
  else
    warn "WECHATPAY_MCH_API_V3_KEY length is ${n} (WeChat expects 32 bytes/characters)"
  fi
fi

echo ""
echo "--- Private key ---"

if [[ -n "${KEY_PATH// }" ]]; then
  ok "WECHATPAY_MCH_PRIVATE_KEY_PATH is set: ${KEY_PATH}"
  if [[ -f "$KEY_PATH" ]]; then
    ok "File exists"
    if sudo -u "${SERVICE_USER}" test -r "$KEY_PATH"; then
      ok "Readable by ${SERVICE_USER} (sudo -u ${SERVICE_USER} test -r)"
    else
      bad "Not readable by ${SERVICE_USER} — fix ownership/permissions (e.g. chown root:${SERVICE_USER} chmod 640)"
      fail=1
    fi
  else
    bad "Path is not a regular file"
    fail=1
  fi
elif [[ -n "${KEY_INLINE// }" ]]; then
  ok "WECHATPAY_MCH_PRIVATE_KEY is set (inline PEM, not printing)"
else
  bad "Neither WECHATPAY_MCH_PRIVATE_KEY_PATH nor WECHATPAY_MCH_PRIVATE_KEY"
  fail=1
fi

echo ""
echo "--- WeChat Pay 微信支付公钥 (optional, avoids platform cert download) ---"
if [[ -n "${PUB_ID// }" ]]; then
  ok "WECHATPAY_PUBLIC_KEY_ID is set"
  if [[ "$PUB_ID" != PUB_KEY_ID_* ]]; then
    warn "WECHATPAY_PUBLIC_KEY_ID should start with PUB_KEY_ID_ (per WeChat docs)"
  fi
  if [[ -n "${PUB_PATH// }" ]]; then
    ok "WECHATPAY_PUBLIC_KEY_PATH is set: ${PUB_PATH}"
    if [[ -f "$PUB_PATH" ]]; then
      if sudo -u "${SERVICE_USER}" test -r "$PUB_PATH"; then
        ok "pub_key.pem readable by ${SERVICE_USER}"
      else
        bad "WECHATPAY_PUBLIC_KEY_PATH not readable by ${SERVICE_USER}"
        fail=1
      fi
    else
      bad "WECHATPAY_PUBLIC_KEY_PATH is not a file"
      fail=1
    fi
  elif [[ -n "${PUB_INLINE// }" ]]; then
    ok "WECHATPAY_PUBLIC_KEY is set (inline PEM, not printing)"
  else
    bad "WECHATPAY_PUBLIC_KEY_ID set but neither WECHATPAY_PUBLIC_KEY_PATH nor WECHATPAY_PUBLIC_KEY"
    fail=1
  fi
elif [[ -n "${PUB_PATH// }" ]] || [[ -n "${PUB_INLINE// }" ]]; then
  bad "WECHATPAY_PUBLIC_KEY_ID required when public key path/PEM is set"
  fail=1
else
  ok "Public key mode not configured (legacy platform-certificate auto-download)"
fi

echo ""
echo "--- Non-secret preview ---"
[[ -n "$APP_ID" ]] && echo "  WECHATPAY_APP_ID=${APP_ID}"
[[ -n "$MCH_ID" ]] && echo "  WECHATPAY_MCH_ID=${MCH_ID}"
[[ -n "$SERIAL" ]] && echo "  WECHATPAY_MCH_CERTIFICATE_SERIAL=${SERIAL}"
[[ -n "$APIV3" ]] && echo "  WECHATPAY_MCH_API_V3_KEY=<${#APIV3} chars, hidden>"
[[ -n "$KEY_PATH" ]] && echo "  WECHATPAY_MCH_PRIVATE_KEY_PATH=${KEY_PATH}"
[[ -n "$KEY_INLINE" ]] && echo "  WECHATPAY_MCH_PRIVATE_KEY=<inline PEM, ${#KEY_INLINE} chars, hidden>"
[[ -n "$PUB_ID" ]] && echo "  WECHATPAY_PUBLIC_KEY_ID=${PUB_ID}"
[[ -n "$PUB_PATH" ]] && echo "  WECHATPAY_PUBLIC_KEY_PATH=${PUB_PATH}"
[[ -n "$PUB_INLINE" ]] && echo "  WECHATPAY_PUBLIC_KEY=<${#PUB_INLINE} chars PEM, hidden>"

echo ""
if [[ "$fail" -eq 0 ]]; then
  echo "${GRN}All required WeChat Pay v3 checks passed.${RST}"
  echo "If checkout still fails, restart after editing .env: systemctl restart ${SERVICE}"
  echo ""
  echo "--- Traffic sanity (if curl uses a public hostname) ---"
  echo "This script only inspected THIS machine's ${SERVICE} process or ${ENV_FILE}."
  echo "If https://api.ai.koooyooo.com/... still returns errors, confirm DNS / LB targets THIS host:"
  echo "  dig +short YOUR_API_HOSTNAME"
  echo "  curl -4sS ifconfig.me || curl -4sS icanhazip.com   # this server's public IPv4"
  exit 0
else
  echo "${RED}Fix the items above, then: systemctl restart ${SERVICE}${RST}"
  exit 1
fi
