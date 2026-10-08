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
| 0.1.x (latest) | Yes |
| Older | No |

## Threat model (summary)

1. **Admin API keys** guard every `/api/v1` REST route; only their sha256 is stored, they are shown once and can be revoked. Anyone holding one has full control of the fleet.
2. **Enrollment keys** are per project and only allow a device to register into that project; rotate or revoke them if they leak (devices already enrolled keep working).
3. **Device tokens** (HS256, signed with `JWT_SECRET`, 30-day default) are checked against the hash stored for the device, so re-registration, refresh and `POST /devices/:id/revoke` invalidate old tokens immediately.
4. **Presigned URLs** are bearer credentials for one object until they expire: an upload URL (15 minutes) lets anyone write that model blob, a download URL (1 hour) lets anyone read it. Devices verify the blob's sha256 before use.
5. **No TLS termination is included**: the REST API, gRPC and object storage listen in plaintext. Put them behind a TLS-terminating proxy or load balancer before exposing them beyond a trusted network, and change every default secret in `deploy/docker-compose.prod.yml`.
