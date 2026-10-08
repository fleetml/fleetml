# FleetML Helm chart

Installs the FleetML server and dashboard and, unless you turn them off, a
single-replica Postgres (TimescaleDB), NATS (with JetStream) and SeaweedFS
(S3-compatible model store). The bundled services suit evaluation and small
fleets; for production, use managed ones (`postgres.enabled=false`,
`nats.enabled=false`, `seaweedfs.enabled=false`).

```bash
helm install fleetml deploy/helm/fleetml \
  --set secrets.jwtSecret=$(openssl rand -base64 32) \
  --set secrets.signingMasterKey=$(openssl rand -base64 32) \
  --set secrets.adminKey=$(openssl rand -hex 32) \
  --set secrets.s3SecretKey=$(openssl rand -hex 16) \
  --set secrets.postgresPassword=$(openssl rand -hex 16) \
  --set s3.publicEndpoint=models.example.com:443,s3.useSSL=true
kubectl port-forward svc/fleetml-server 18080 & curl -s localhost:18080/readyz
helm test fleetml
```

Keep `secrets.signingMasterKey`: it encrypts the artifact signing keys
(see [CONFIG.md](../../../docs/CONFIG.md)). Instead of `--set secrets.*`, you
can name an existing Secret with the keys `JWT_SECRET`, `SIGNING_MASTER_KEY`,
`FLEETML_ADMIN_KEY`, `S3_ACCESS_KEY`, `S3_SECRET_KEY` and `POSTGRES_PASSWORD`
(`secrets.existingSecret`).

Devices download models straight from the object store, so it must be
reachable from them at `s3.publicEndpoint` (an Ingress or LoadBalancer in
front of the `fleetml-s3` service, or your external store). Devices reach the
gRPC API on the `fleetml-server` service's port 19090 and the dashboard's
users the `fleetml-dashboard` service; expose them the same way, with TLS in
front (FleetML does not terminate TLS).

| Value | Default | Description |
|---|---|---|
| `server.image.repository`, `.tag` | `ghcr.io/fleetml/fleetml-server`, the app version | Server image |
| `server.env` | `{}` | Extra server settings ([CONFIG.md](../../../docs/CONFIG.md)), e.g. `DASHBOARD_URL` |
| `server.replicas` | `1` | Server replicas (they share the database and the telemetry stream) |
| `dashboard.enabled` | `true` | The dashboard, with `/api` proxied to the server |
| `secrets.*` | | See above; `jwtSecret`, `signingMasterKey`, `s3SecretKey` and (bundled Postgres) `postgresPassword` are required |
| `s3.publicEndpoint` | (required) | `host:port` devices and clients download models from |
| `s3.endpoint`, `s3.useSSL`, `s3.bucket` | | The external store (`seaweedfs.enabled=false`) |
| `postgres.enabled`, `.url`, `.storage` | `true`, , `10Gi` | Bundled TimescaleDB, or the URL of yours |
| `nats.enabled`, `.url`, `.storage` | `true`, , `2Gi` | Bundled NATS, or yours (JetStream required) |
| `seaweedfs.enabled`, `.storage` | `true`, `10Gi` | Bundled SeaweedFS |

`helm uninstall` keeps the volume claims of the bundled services, so the
data survives a reinstall; delete them (`kubectl delete pvc -l
app.kubernetes.io/instance=fleetml`) to remove it.

`scripts/helm-e2e.sh` installs the chart into a throwaway kind cluster, runs
`helm test` and the end-to-end suite against it, and uninstalls it.
