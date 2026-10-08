# Security Policy

## Reporting a vulnerability

Please do not open a public issue for security problems.

Email **fleetml.maintainers@gmail.com** with:

- a description of the issue and its impact,
- the FleetML version or commit you tested,
- steps to reproduce (a proof of concept if you have one).

We aim to acknowledge reports within 7 days and to agree a disclosure date with you once a fix is ready. If you do not hear back, please send a reminder to the same address.

## Supported versions

FleetML is pre-1.0. Security fixes go into the latest release only.

| Version | Supported |
|---|---|
| 0.2.x (latest) | Yes |
| Older | No |

## Threat model (summary)

1. **Admin API keys** guard every `/api/v1` REST route; only their sha256 is stored, they are shown once and can be revoked. Anyone holding one has full control of the fleet.
   **Dashboard sessions**: the dashboard exchanges a key for a session (`POST /api/v1/auth/login`) held in an `httpOnly`, `SameSite=Strict` cookie, `Secure` when the request arrived over HTTPS (directly or with `X-Forwarded-Proto: https`); the key is not kept in the browser. Sessions expire after `SESSION_TTL` (12 hours), can be revoked from Settings, and end when their API key is revoked. Requests that change state with a session cookie must carry the session's CSRF token (`X-CSRF-Token`), an HMAC of the session token with `JWT_SECRET`.
2. **Enrollment keys** are per project and only allow a device to register into that project; rotate or revoke them if they leak (devices already enrolled keep working).
3. **Device tokens** (HS256, signed with `JWT_SECRET`, at least 32 characters, 30-day default) are checked against the hash stored for the device, so re-registration, refresh and `POST /devices/:id/revoke` invalidate old tokens immediately. A device only receives deployments and experiments of its own project, and its telemetry is only accepted for that project's model versions.
4. **Presigned URLs** are bearer credentials for one object until they expire: an upload URL (15 minutes) lets anyone write that model blob, a download URL (1 hour) lets anyone read it. Devices verify the blob's sha256 before use.
5. **No TLS termination is included**: the REST API, gRPC and object storage listen in plaintext. Put them behind a TLS-terminating proxy or load balancer before exposing them beyond a trusted network. `deploy/docker-compose.prod.yml` requires `JWT_SECRET` from your shell; change its other default secrets (Postgres and object store) too.
6. **Unauthenticated endpoints**: `GET /livez`, `GET /readyz`, `GET /healthz` and `GET /metrics` on the REST port and `grpc.health.v1` on the gRPC port need no credentials. They reveal dependency status and request counts, not fleet data; keep `/metrics` on a network only your monitoring can reach. gRPC reflection is off unless `GRPC_REFLECTION=true`.
