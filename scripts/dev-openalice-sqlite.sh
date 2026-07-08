#!/usr/bin/env bash
set -euo pipefail

port="${PORT:-6080}"
sqlite_path="${SQLITE_PATH:-one-api.db}"
provisioning_token="${OPENALICE_PROVISIONING_TOKEN:-dev-provisioning-secret}"

cat <<EOF
Starting Alice AI Gateway for OpenAlice Cloud local development:

  data plane:        http://localhost:${port}/v1/*
  control plane:     http://localhost:${port}/api/openalice/provisioning/*
  sqlite database:   ${sqlite_path}
  managed mode:      true
  redis:             disabled

EOF

exec env \
  PORT="${port}" \
  SQL_DSN= \
  LOG_SQL_DSN= \
  REDIS_CONN_STRING= \
  SQLITE_PATH="${sqlite_path}" \
  OPENALICE_MANAGED_MODE=true \
  OPENALICE_PROVISIONING_TOKEN="${provisioning_token}" \
  go run .
