#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="/opt/raglex-cloud/.env"
if [ ! -f "$ENV_FILE" ]; then
  echo "Missing $ENV_FILE" >&2
  exit 1
fi
# shellcheck disable=SC1090
source "$ENV_FILE"
API="https://cloud.raglex.sorai.tw"
CMD="${1:-help}"

case "$CMD" in
  create)
    MAX_USES="${2:-1}"
    NOTE="${3:-}"
    EXP_HOURS="${4:-0}"
    curl -sS -X POST "$API/api/v1/admin/invites" \
      -H "Authorization: Bearer $RAGLEX_CLOUD_ADMIN_TOKEN" \
      -H "Content-Type: application/json" \
      -d "{\"max_uses\":$MAX_USES,\"note\":\"$NOTE\",\"expires_hours\":$EXP_HOURS}"
    echo ""
    ;;
  list)
    curl -sS "$API/api/v1/admin/invites" \
      -H "Authorization: Bearer $RAGLEX_CLOUD_ADMIN_TOKEN"
    echo ""
    ;;
  revoke)
    CODE="${2:?Usage: $0 revoke RGLX-XXXX-XXXX}"
    curl -sS -X DELETE "$API/api/v1/admin/invites/$CODE" \
      -H "Authorization: Bearer $RAGLEX_CLOUD_ADMIN_TOKEN"
    echo ""
    ;;
  users)
    curl -sS "$API/api/v1/admin/users" \
      -H "Authorization: Bearer $RAGLEX_CLOUD_ADMIN_TOKEN"
    echo ""
    ;;
  *)
    cat <<EOF
Raglex Cloud Invite & User Manager
Usage:
  $0 create [max_uses=1] [note] [expires_hours=0]
  $0 list
  $0 revoke RGLX-XXXX-XXXX
  $0 users
EOF
    ;;
esac
