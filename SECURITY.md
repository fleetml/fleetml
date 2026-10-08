# Security Policy

## Reporting a vulnerability

Please do not open a public issue for security problems.

Email **security@fleetml.dev** (or, if that bounces, **fleetml.maintainers@gmail.com**) with:

- a description of the issue and its impact,
- the FleetML version or commit you tested,
- steps to reproduce (a proof of concept if you have one).

We aim to acknowledge reports within 7 days and to agree a disclosure date with you once a fix is ready. If you do not hear back, please send a reminder to the other address.

## Supported versions

FleetML is pre-1.0. Security fixes go into the latest release only.

| Version | Supported |
|---|---|
| 0.3.x (latest) | Yes |
| Older | No |

## Threat model (summary)

1. **API keys** guard every `/api/v1` REST route; only their sha256 is stored, they are shown once and can be revoked. Each key acts as a user: a global admin (full control of every project, users and keys; `FLEETML_ADMIN_KEY` belongs to one) or a user with a viewer, operator or admin role per project. A permission matrix covers every route and refuses unlisted ones; requests for other projects' resources get 403 whether or not they exist. Every change and every refused request is in the audit log (`GET /audit`).
   **Dashboard sessions**: the dashboard exchanges a key for a session (`POST /api/v1/auth/login`) held in an `httpOnly`, `SameSite=Strict` cookie, `Secure` when the request arrived over HTTPS (directly or with `X-Forwarded-Proto: https`); the key is not kept in the browser. Sessions expire after `SESSION_TTL` (12 hours), can be revoked from Settings, and end when their API key is revoked. Requests that change state with a session cookie must carry the session's CSRF token (`X-CSRF-Token`), an HMAC of the session token with `JWT_SECRET`.
2. **Enrollment keys** are per project and only allow a device to register into that project; rotate or revoke them if they leak (devices already enrolled keep working).
3. **Device tokens** (HS256, signed with `JWT_SECRET`, at least 32 characters, 30-day default) are checked against the hash stored for the device, so re-registration, refresh and `POST /devices/:id/revoke` invalidate old tokens immediately. A device only receives deployments and experiments of its own project, and its telemetry is only accepted for that project's model versions.
4. **Presigned URLs** are bearer credentials for one object until they expire: an upload URL (15 minutes) lets anyone write that model blob, a download URL (1 hour) lets anyone read it. Devices verify the blob's sha256 before use, and the signature of the sha256 before downloading (below). A delta is checked against its own sha256 and the patched model against the model's.
5. **Artifact signing.** Each project has an ed25519 key. When a model variant is finalized, after the server has checked the uploaded file's sha256, the server signs `fleetml-artifact-v1\n<model_id>\n<version>\n<sha256>`. Devices get the project's public keys with every registration and heartbeat, and the Android and iOS SDKs and the Linux agent refuse an assignment whose signature does not verify, reporting `SIGNATURE_INVALID`. This protects against a changed file in the object store and against a database write that changes a variant's hash, file or version, by anyone who does not have `SIGNING_MASTER_KEY`. The private keys are stored encrypted with AES-256-GCM under a key derived from `SIGNING_MASTER_KEY` (HKDF-SHA256), bound to their key ID. What it does not protect against: a compromised server process, which holds the master key and can sign anything; and a device that talks to an impostor server, since devices trust the keys the server they are enrolled with sends (use TLS). Rotating (`POST /projects/{id}/signing-key/rotate`) signs every variant whose signature verifies with the old key again with the new one; the old key stays valid on devices until the grace period ends, even offline, and is refused after it. A device accepts unsigned assignments only while it knows no keys (a server from before signing).
6. **No TLS termination is included**: the REST API, gRPC and object storage listen in plaintext. Put them behind a TLS-terminating proxy or load balancer before exposing them beyond a trusted network. `deploy/docker-compose.prod.yml` requires `JWT_SECRET` and `SIGNING_MASTER_KEY` from your shell; change its other default secrets (Postgres and object store) too. The Helm chart refuses to render without its secrets.
7. **Unauthenticated endpoints**: `GET /livez`, `GET /readyz`, `GET /healthz` and `GET /metrics` on the REST port and `grpc.health.v1` on the gRPC port need no credentials. They reveal dependency status and request counts, not fleet data; keep `/metrics` on a network only your monitoring can reach. gRPC reflection is off unless `GRPC_REFLECTION=true`.
