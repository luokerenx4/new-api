# OpenAlice Account Adaptation Notes

Alice AI Gateway is operated as the AI execution data plane for OpenAlice. This
document records the current account/quota structure and the intended
OpenAlice-specific adaptation boundary.

## Current Core Structures

`model.User` is the central account row. It currently carries:

- login identity: username, password, email, OAuth ids, session-facing role and status;
- operator access: `access_token`, used by dashboard/admin API authentication;
- execution accounting: wallet `quota`, `used_quota`, `request_count`, and `group`;
- SaaS/payment state: invite fields, Stripe customer id, top-up/subscription settings.

`model.Token` is the API-key row under a user. It currently carries:

- token identity: key, name, status, creation/access/expiry timestamps;
- execution limits: `remain_quota`, `used_quota`, `unlimited_quota`;
- routing restrictions: group, model limits, allowed IPs, cross-group retry.

Request execution currently gates both the token and the selected funding
source. The billing session pre-consumes quota before forwarding, settles the
delta after actual usage is known, and refunds on failure.

## OpenAlice Interpretation

For OpenAlice, the gateway account is not the product account. It is an
execution account created, funded, frozen, and reconciled by OpenAlice Cloud.

Mapping:

- OpenAlice Cloud account -> Alice AI Gateway user.
- OpenAlice managed credential -> Alice AI Gateway token.
- OpenAlice entitlement/credits event -> Alice AI Gateway quota operation.
- Alice AI Gateway usage/log data -> OpenAlice Cloud reconciliation snapshot.

The gateway should not decide whether a customer has paid, which plan they are
on, or whether their subscription is active. It should only execute the account
and token state that OpenAlice Cloud provisions into it.

## Managed Mode

`OPENALICE_MANAGED_MODE=true` is the first adaptation step.

When enabled:

- self-service register is blocked;
- password self-register is forced off during env initialization;
- consumer top-up, redeem, checkout, and subscription purchase entrypoints are
  blocked;
- subscription plan listing returns an empty public list;
- operator login, admin APIs, gateway token execution, quota enforcement, and
  usage logs remain available.

This preserves the mature execution path while removing the gateway as an
independent consumer SaaS surface.

## Provisioning Boundary

The first Cloud-facing provisioning surface lives under:

```text
/api/openalice/provisioning/*
```

It is disabled until `OPENALICE_PROVISIONING_TOKEN` is configured. Requests must
send either `Authorization: Bearer <token>` or
`X-OpenAlice-Provisioning-Token: <token>`.

Current endpoints:

- `POST /api/openalice/provisioning/users/upsert`
- `POST /api/openalice/provisioning/tokens`
- `POST /api/openalice/provisioning/tokens/:id/quota`
- `POST /api/openalice/provisioning/tokens/:id/status`
- `GET /api/openalice/provisioning/accounts/:external_account_id/snapshot`

Every mutating request requires `operation_id`. Duplicate operation ids are
rejected to prevent Cloud retries from double-creating tokens or double-granting
quota.

The intended long-term boundary is still narrow:

- create or find a gateway user for an external OpenAlice account id;
- issue a token for that user and return the raw key once;
- increase or set token/user quota through an auditable operation;
- freeze or revoke tokens;
- disable a user;
- return reconciliation snapshots: user quota, token quota, used quota, request
  count, token status, and recent usage totals.

Implemented model additions:

- `external_account_id` on gateway users, unique and nullable;
- `provisioning_operations` append-only audit table.

Possible later addition:

- `provisioning_service_tokens` or a stricter service-token auth layer separate
  from the environment-level bootstrap token.

## Safety Notes

- Keep the existing atomic wallet pre-consume path. It is the correct admission
  pattern for concurrent quota use.
- Token quota and user quota both matter today. If OpenAlice Cloud wants one
  balance concept, prefer provisioning both consistently rather than bypassing
  one layer without a migration.
- The existing `access_token` header auth is useful for bootstrap, but should
  not become the final long-term Cloud integration contract without scoped
  service permissions and audit events.
- Do not move OpenAlice Cloud billing, Stripe subscription truth, or private
  entitlement logic into this repository.
