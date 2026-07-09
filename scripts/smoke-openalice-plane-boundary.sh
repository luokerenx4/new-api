#!/usr/bin/env bash
set -euo pipefail

port="${PORT:-6091}"
tmpdir="$(mktemp -d)"
pid=""

cleanup() {
  if [ -n "${pid}" ] && kill -0 "${pid}" >/dev/null 2>&1; then
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  fi
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

echo "==> start local Alice AI Gateway plane-boundary smoke on port ${port}"
env \
  PORT="${port}" \
  SQL_DSN= \
  LOG_SQL_DSN= \
  REDIS_CONN_STRING= \
  SQLITE_PATH="${tmpdir}/gateway.db" \
  OPENALICE_MANAGED_MODE=true \
  OPENALICE_PROVISIONING_TOKEN=local-provisioning-secret \
  OPENALICE_CONTROL_PLANE_HOSTS=control.localhost,localhost,127.0.0.1 \
  OPENALICE_PUBLIC_DATA_PLANE_HOSTS=alpha-gateway.wond.dev \
  go run . >"${tmpdir}/gateway.log" 2>&1 &
pid="$!"

node --input-type=module - "${port}" <<'NODE'
import http from 'node:http'

const port = Number(process.argv[2])

function request({ name, method = 'GET', host, path, body }) {
  return new Promise((resolve, reject) => {
    const payload = body === undefined ? undefined : JSON.stringify(body)
    const req = http.request(
      {
        hostname: '127.0.0.1',
        port,
        method,
        path,
        headers: {
          Host: host,
          Authorization: 'Bearer local-provisioning-secret',
          ...(payload
            ? {
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(payload),
              }
            : {}),
        },
      },
      (res) => {
        let text = ''
        res.setEncoding('utf8')
        res.on('data', (chunk) => {
          text += chunk
        })
        res.on('end', () => {
          resolve({ name, status: res.statusCode, body: text })
        })
      },
    )
    req.on('error', reject)
    if (payload) req.write(payload)
    req.end()
  })
}

async function waitForReady() {
  for (let attempt = 0; attempt < 80; attempt += 1) {
    try {
      const response = await request({
        name: 'ready',
        host: 'alpha-gateway.wond.dev',
        path: '/api/status',
      })
      if (response.status === 200) return
    } catch {}
    await new Promise((resolve) => setTimeout(resolve, 250))
  }
  throw new Error('gateway did not become ready')
}

await waitForReady()

const cases = [
  {
    name: 'public root blocked',
    host: 'alpha-gateway.wond.dev',
    path: '/',
    want: 404,
  },
  {
    name: 'public provisioning blocked',
    method: 'POST',
    host: 'alpha-gateway.wond.dev',
    path: '/api/openalice/provisioning/users/upsert',
    body: { operation_id: 'e2e-public-block', external_account_id: 'acct_public' },
    want: 404,
  },
  {
    name: 'public setup blocked',
    host: 'alpha-gateway.wond.dev',
    path: '/setup',
    want: 404,
  },
  {
    name: 'public dashboard blocked',
    host: 'alpha-gateway.wond.dev',
    path: '/dashboard',
    want: 404,
  },
  {
    name: 'public playground blocked',
    method: 'POST',
    host: 'alpha-gateway.wond.dev',
    path: '/pg/chat/completions',
    body: { model: 'glm-5.2', messages: [{ role: 'user', content: 'ping' }] },
    want: 404,
  },
  {
    name: 'public user api blocked',
    host: 'alpha-gateway.wond.dev',
    path: '/api/user/self',
    want: 404,
  },
  {
    name: 'public admin channel api blocked',
    host: 'alpha-gateway.wond.dev',
    path: '/api/channel/',
    want: 404,
  },
  {
    name: 'public token management api blocked',
    host: 'alpha-gateway.wond.dev',
    path: '/api/token/',
    want: 404,
  },
  {
    name: 'public subscription admin api blocked',
    host: 'alpha-gateway.wond.dev',
    path: '/api/subscription/admin/plans',
    want: 404,
  },
  {
    name: 'public option root api blocked',
    host: 'alpha-gateway.wond.dev',
    path: '/api/option/',
    want: 404,
  },
  {
    name: 'public status allowed',
    host: 'alpha-gateway.wond.dev',
    path: '/api/status',
    want: 200,
  },
  {
    name: 'public slashless usage route blocked',
    host: 'alpha-gateway.wond.dev',
    path: '/api/usage/token',
    want: 404,
  },
  {
    name: 'public usage read-only route token-authenticated',
    host: 'alpha-gateway.wond.dev',
    path: '/api/usage/token/',
    want: 401,
  },
  {
    name: 'public models route token-authenticated',
    host: 'alpha-gateway.wond.dev',
    path: '/v1/models',
    want: 401,
  },
  {
    name: 'public relay route allowed but token-authenticated',
    method: 'POST',
    host: 'alpha-gateway.wond.dev',
    path: '/v1/chat/completions',
    body: { model: 'glm-5.2', messages: [{ role: 'user', content: 'ping' }] },
    want: 401,
  },
  {
    name: 'public gemini route token-authenticated',
    method: 'POST',
    host: 'alpha-gateway.wond.dev',
    path: '/v1beta/models/gemini-2.0-flash:generateContent',
    body: { contents: [{ parts: [{ text: 'ping' }] }] },
    want: 401,
  },
  {
    name: 'unknown railway host treated as public data plane',
    method: 'POST',
    host: 'random-railway-domain.up.railway.app',
    path: '/api/openalice/provisioning/users/upsert',
    body: { operation_id: 'e2e-unknown-block', external_account_id: 'acct_unknown' },
    want: 404,
  },
  {
    name: 'control provisioning allowed',
    method: 'POST',
    host: 'control.localhost',
    path: '/api/openalice/provisioning/users/upsert',
    body: { operation_id: 'e2e-control-allow', external_account_id: 'acct_control', group: 'free' },
    want: 200,
  },
]

for (const testCase of cases) {
  const response = await request(testCase)
  console.log(`${testCase.name}: ${response.status}`)
  if (response.status !== testCase.want) {
    console.error(response.body.slice(0, 500))
    throw new Error(`${testCase.name}: got ${response.status}, want ${testCase.want}`)
  }
}

console.log('gateway plane-boundary smoke ok')
NODE
