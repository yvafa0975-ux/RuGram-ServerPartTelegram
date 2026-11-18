#!/bin/sh
set -eu

umask 077

command_name="${1##*/}"

require_secret() {
  name="$1"
  value="$(printenv "$name" 2>/dev/null || true)"
  normalized="$(printf '%s' "$value" | tr '[:upper:]' '[:lower:]')"
  case "$normalized" in
    ""|*changeme*|*change-me*|*replace-me*)
      echo "rugram: required secret $name is missing or still uses a placeholder" >&2
      exit 64
      ;;
  esac
}

require_value() {
  name="$1"
  value="$(printenv "$name" 2>/dev/null || true)"
  normalized="$(printf '%s' "$value" | tr '[:upper:]' '[:lower:]')"
  case "$normalized" in
    ""|*changeme*|*change-me*|*replace-me*)
      echo "rugram: required setting $name is missing or still uses a placeholder" >&2
      exit 64
      ;;
  esac
}

initialize_server_key() {
  private_key="${RUGRAM_RSA_KEY:-/var/lib/rugram/server_rsa.pem}"
  identity_mode="$(printf '%s' "${RUGRAM_RSA_IDENTITY_MODE:-generated}" | tr '[:upper:]' '[:lower:]')"
  embedded_private_key=/usr/share/rugram/keys/test-server-rsa.pem.b64
  key_dir=$(dirname -- "$private_key")

  mkdir -p "$key_dir"
  case "$identity_mode" in
    generated) ;;
    test)
      if [ ! -f "$private_key" ]; then
        if [ ! -r "$embedded_private_key" ]; then
          echo "rugram: test RSA identity requested, but this image does not contain the published test key; use the server-test target or set RUGRAM_RSA_IDENTITY_MODE=generated" >&2
          exit 66
        fi
        temporary_key="$private_key.tmp.$$"
        trap 'rm -f "$temporary_key"' EXIT HUP INT TERM
        base64 -d "$embedded_private_key" >"$temporary_key"
        chmod 0600 "$temporary_key"
        mv "$temporary_key" "$private_key"
        trap - EXIT HUP INT TERM
        echo "rugram: WARNING using the published main test RSA identity; its private key is public" >&2
      fi
      ;;
    *)
      echo "rugram: RUGRAM_RSA_IDENTITY_MODE must be test or generated" >&2
      exit 64
      ;;
  esac

  if [ -f "$private_key" ]; then
    if ! openssl rsa -in "$private_key" -check -noout >/dev/null 2>&1; then
      echo "rugram: $private_key is not a valid RSA private key" >&2
      exit 65
    fi
    chmod 0600 "$private_key"
  fi
}

case "$command_name" in
  rugram)
    require_value RUGRAM_ADVERTISE_IP
    require_value RUGRAM_PUBLIC_BASE_URL
    require_value RUGRAM_PUBLIC_WEB_BASE_URL
    require_secret RUGRAM_POSTGRES_DSN
    require_secret RUGRAM_REDIS_PASSWORD
    require_secret RUGRAM_ADMIN_API_TOKEN
    require_secret RUGRAM_TURN_SECRET
    initialize_server_key
    ;;
  rugram-admin)
    require_secret RUGRAM_POSTGRES_DSN
    require_secret RUGRAM_ADMIN_API_TOKEN
    require_secret RUGRAM_ADMIN_SESSION_KEY
    admin_password="$(printenv RUGRAM_ADMIN_UI_PASSWORD 2>/dev/null || true)"
    admin_token="$(printenv RUGRAM_ADMIN_UI_TOKEN 2>/dev/null || true)"
    if [ -n "$admin_password" ]; then
      require_secret RUGRAM_ADMIN_UI_PASSWORD
    elif [ -n "$admin_token" ]; then
      require_secret RUGRAM_ADMIN_UI_TOKEN
    else
      echo "rugram: RUGRAM_ADMIN_UI_PASSWORD or RUGRAM_ADMIN_UI_TOKEN is required" >&2
      exit 64
    fi
    ;;
esac

exec "$@"
