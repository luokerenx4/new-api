<div align="center">

![Alice AI Gateway](/web/default/public/logo.png)

# Alice AI Gateway

**OpenAlice-operated AI gateway data plane, based on New API.**

</div>

## Project Role

Alice AI Gateway is the OpenAlice AI execution gateway. It is designed to run
as a separately deployed data-plane service behind OpenAlice Cloud, not as a
standalone consumer SaaS or a public self-service billing portal.

In the OpenAlice deployment model:

- **OpenAlice Cloud** owns accounts, login, subscriptions, entitlements,
  billing, activation, and staff operations.
- **Alice AI Gateway** owns model routing, upstream provider credentials,
  request forwarding, token-level quota execution, and usage accounting.
- **OpenAlice clients** receive managed AI credentials provisioned by
  OpenAlice Cloud and call this gateway endpoint directly.
- Core commercial permission changes are expected to arrive from an external
  orchestrator, currently OpenAlice Cloud. This service should treat those
  grants, token operations, freezes, and revocations as the source of truth for
  OpenAlice-managed users.

This separation keeps the private OpenAlice Cloud account system out of this
AGPL repository while keeping the AI gateway implementation open and auditable.

## OpenAlice Boundary

Alice AI Gateway should be operated as a closed execution surface controlled by
OpenAlice Cloud:

- Do not put OpenAlice Cloud secrets, Stripe secrets, customer billing logic, or
  private account entitlement code in this repository.
- Prefer internal provisioning APIs, admin automation, or database-backed
  operations that OpenAlice Cloud can call with explicit service credentials.
- Built-in end-user recharge, subscription, and checkout surfaces are not the
  OpenAlice product source of truth. They may be disabled, hidden, or retained
  only for internal maintenance if needed.
- Gateway API keys are execution credentials. Product-level ownership,
  lifecycle, and paid access live in OpenAlice Cloud.

## What This Service Still Does

Alice AI Gateway remains a full AI gateway/proxy:

- OpenAI-compatible, Claude-compatible, Gemini-compatible, and other relay
  surfaces inherited from New API.
- Multi-provider channel management and model routing.
- Token-level limits, groups, model restrictions, and request accounting.
- Quota execution for Cloud-issued managed AI credentials.
- Operational logs and usage data needed for reconciliation with OpenAlice
  Cloud.

## Deployment Sketch

Local Docker Compose:

```bash
docker compose up -d
```

Build the container image locally:

```bash
docker build -t alice-ai-gateway:latest .
```

Run a standalone container with SQLite:

```bash
docker run --name alice-ai-gateway -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v ./data:/data \
  alice-ai-gateway:latest
```

For production, run the gateway as its own service with a dedicated database,
dedicated Redis, internal admin/service credentials, and a public HTTPS endpoint
used by OpenAlice clients.

For OpenAlice production, set:

```bash
OPENALICE_MANAGED_MODE=true
```

Managed mode disables consumer self-service signup, top-up, and subscription
purchase entrypoints. Operator login, admin APIs, gateway tokens, quota
execution, and reconciliation data remain available.

To let OpenAlice Cloud provision accounts and tokens, set a long random service
secret:

```bash
OPENALICE_PROVISIONING_TOKEN=...
```

Cloud should call `/api/openalice/provisioning/*` with either
`Authorization: Bearer <token>` or `X-OpenAlice-Provisioning-Token: <token>`.
The provisioning API is disabled when this variable is empty.
OpenAlice Cloud uses this private boundary for managed account/token work and
for operator-owned rate-limit policy sync.

For public Gateway hostnames, restrict the exposed surface to model/data-plane
routes while keeping dashboard, admin, and provisioning on private/control
hostnames:

```bash
OPENALICE_CONTROL_PLANE_HOSTS=alice-ai-gateway-staging.railway.internal
OPENALICE_PUBLIC_DATA_PLANE_HOSTS=alpha-gateway.wond.dev
```

Requests whose `Host` matches `OPENALICE_CONTROL_PLANE_HOSTS` can access the
full operator surface. Other hosts, including `OPENALICE_PUBLIC_DATA_PLANE_HOSTS`
and accidental provider-generated domains, can only access relay routes such as
`/v1/*`, `/v1beta/*`, `/mj/*`, `/suno/*`, and `/api/status`. Account, admin,
setup, and `/api/openalice/provisioning/*` routes remain reachable only through
private/internal hostnames.

## Development

Backend:

```bash
go test ./...
go build -o alice-ai-gateway
```

Frontend:

```bash
cd web/default
bun install
bun run build
```

The Go module path is currently kept as `github.com/QuantumNous/new-api` to
avoid a high-risk import-path rewrite. Public service naming, Docker naming,
and OpenAlice documentation use `alice-ai-gateway`.

## OpenAlice Adaptation Notes

The first OpenAlice-specific adaptation target is the account and quota boundary:

- Treat OpenAlice Cloud as the authoritative account and entitlement system.
- Add or expose a narrow provisioning boundary for creating gateway users,
  issuing tokens, adding quota, freezing tokens, revoking tokens, and fetching
  reconciliation snapshots.
- Keep user-facing recharge/subscription flows out of the OpenAlice product
  path unless they are explicitly repurposed as internal operator tools.
- Run with `OPENALICE_MANAGED_MODE=true` to close consumer self-service account
  and billing entrypoints while preserving operator and execution surfaces.
- Set `OPENALICE_PROVISIONING_TOKEN` to enable the Cloud-facing
  `/api/openalice/provisioning/*` service boundary.
- Preserve request-level quota safety. Atomic quota admission and clear
  settle/refund behavior matter more than UI convenience.

See [OpenAlice Account Adaptation Notes](./docs/openalice-account-adaptation.md)
for the current user/token/quota structure and the provisioning boundary we
intend to build next.

## Upstream Attribution And License

Alice AI Gateway is a modified distribution of
[New API](https://github.com/QuantumNous/new-api), which is licensed under the
[GNU Affero General Public License v3.0](./LICENSE).

Additional terms under AGPLv3 Section 7 apply. Modified versions must preserve
the author attribution notice `Frontend design and development by New API
contributors.` in the appropriate legal notices and in any prominent about,
legal, footer, or attribution location presented by the user interface.

Modified versions that present a user interface must also preserve a visible
link to the original project: <https://github.com/QuantumNous/new-api>.

New API itself is based on [One API](https://github.com/songquanpeng/one-api)
(MIT License). See [NOTICE](./NOTICE) and
[THIRD-PARTY-LICENSES.md](./THIRD-PARTY-LICENSES.md) for attribution and
third-party license information.
