# FleetML Helm chart

The chart installs the FleetML server and dashboard from the images published
with every release (`ghcr.io/fleetml/fleetml-server` and
`ghcr.io/fleetml/fleetml-dashboard`, tagged with the chart's `appVersion`) and,
unless you turn them off, a single-replica Postgres (TimescaleDB), NATS (with
JetStream) and SeaweedFS (the S3-compatible model store). The bundled services
suit evaluation and small fleets; for production, see
[Production](#production). The chart installs from a clone of the
repository: there is no chart repository or OCI package.

Needs a Kubernetes cluster, `kubectl`, Helm 3 and `openssl`, and for the
last steps the [`fleetml` CLI](../../../docs/QUICKSTART.md#get-the-cli) and
`jq`. No cluster yet? [kind](https://kind.sigs.k8s.io/) makes one in Docker:

```bash
kind create cluster --name fleetml
```

## Install

From the root of a clone. First the secrets, generated once into a file
only you can read:

```bash
(umask 077; cat > fleetml-secrets.yaml <<EOF
secrets:
  jwtSecret: "$(openssl rand -base64 32)"
  signingMasterKey: "$(openssl rand -base64 32)"
  adminKey: "$(openssl rand -hex 32)"
  s3SecretKey: "$(openssl rand -hex 16)"
  postgresPassword: "$(openssl rand -hex 16)"
EOF
)
```

**Keep this file, and never generate it again for a release that has data.**
Every install and upgrade of this release passes it. With new values:
`postgresPassword` no longer opens the database (Postgres sets it when its
volume is created, and the server then fails with `password authentication
failed`), `signingMasterKey` no longer decrypts the stored signing keys (so
publishing fails), and a new `jwtSecret` invalidates every device token (the
devices register again with their enrollment key). A new `s3SecretKey` is
safe: the store restarts with it. `adminKey` is stored on the first start
only. Instead of the file, a Secret of your own with the keys `JWT_SECRET`,
`SIGNING_MASTER_KEY`, `FLEETML_ADMIN_KEY`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`
and `POSTGRES_PASSWORD` works the same way: `--set secrets.existingSecret=<name>`,
except that a change to your Secret rolls nothing: after one, run
`kubectl -n fleetml rollout restart statefulset/fleetml-s3 deploy/fleetml-server`.
The chart refuses a `jwtSecret`, `signingMasterKey` or `adminKey` shorter
than 32 characters (the server would not start with one), and the first two
must differ. The file name is in the repository's `.gitignore`, so a clone
does not commit it; keep a copy outside the clone as well.

Then the chart, into its own namespace. The first line names the local
ports this machine reaches the release on, the
[quickstart's](../../../docs/QUICKSTART.md) REST, gRPC and model-file ports;
if its compose stack runs here (or anything else holds them), make it
`REST=28080 GRPC=29090 S3=29100`, and every later line, `Try it` included,
uses those:

```bash
REST=18080 GRPC=19090 S3=19100
helm install fleetml deploy/helm/fleetml -n fleetml --create-namespace \
  -f fleetml-secrets.yaml --set s3.publicEndpoint=localhost:$S3
kubectl -n fleetml rollout status deploy/fleetml-server --timeout=5m
kubectl -n fleetml port-forward svc/fleetml-server $REST:18080 $GRPC:19090 >/dev/null &
kubectl -n fleetml port-forward svc/fleetml-s3 $S3:8333 >/dev/null &
until curl -sf localhost:$REST/readyz; do sleep 1; done; echo
helm test fleetml -n fleetml
```

`/readyz` answers `{"db":"connected","nats":"connected","s3":"connected","status":"ok","version":"0.9.0"}`,
and `helm test` ends with `Phase: Succeeded`. The first install pulls the
images, which takes a minute or two (the server waits up to
`server.dependencyTimeout`, 180 s, for Postgres meanwhile);
`kubectl -n fleetml get pods` then shows every pod `Running` with no
restarts.

`s3.publicEndpoint` is the one value the chart needs besides the secrets:
the `host:port` where devices and the CLI reach the object store. The server
hands out every model as a download URL signed for that address, and
`fleetml push` uploads to it. Here it is `localhost:$S3`, which the second
port-forward serves, so it works from this machine only; a device elsewhere
needs an address it reaches (see [Production](#production)). The URLs are
plain `http` unless `s3.publicUseSSL=true`; `s3.useSSL` is the server's own
connection to the store, and the bundled SeaweedFS speaks plain HTTP inside
the cluster.

### Try it

The CLI, a model, an enrollment key, 20 simulated devices and a deployment,
as in the [tour](../../../docs/QUICKSTART.md#tour), from the clone's root:

```bash
export FLEETML_SERVER=http://localhost:$REST FLEETML_API_KEY=$(sed -n 's/^  adminKey: "\(.*\)"$/\1/p' fleetml-secrets.yaml)
P=$(fleetml project create --name helm-tour -o json | jq -r .id)
fleetml push models/iris/iris_v1.tflite --project $P --model iris --version 1.0.0
V1=$(fleetml ver list --model $(fleetml model list --project $P -o json | jq -r '.[0].id') -o json | jq -r '.[0].id')
EK=$(fleetml enrollment-key create --project $P --name helm -o json | jq -r .key)
fleetml simulate --enrollment-key $EK --grpc localhost:$GRPC --devices 20 --label app=helm --seed helm-tour > /tmp/fleetml-helm-sim.log 2>&1 &
SIM=$!
fleetml deploy start $(fleetml deploy create --project $P --version $V1 --target '{"app":"helm"}' -o json | jq -r .id)
sleep 10; tail -1 /tmp/fleetml-helm-sim.log
```

The deployment starts as `COMPLETE` and the simulator's last line reads
`simulator: iris 1.0.0 on 20`. `kill $SIM` stops the simulator.
`kubectl -n fleetml port-forward svc/fleetml-dashboard 8080:80` serves the
dashboard at http://localhost:8080 (log in with the `adminKey`).

## Upgrading

Change a value, or move the clone to a new release (`git checkout v<version>`)
for a new chart and images, then upgrade with the same secrets file:

```bash
helm upgrade fleetml deploy/helm/fleetml -n fleetml -f fleetml-secrets.yaml --reuse-values \
  --set server.env.DEPLOY_TICK_INTERVAL=10s
kubectl -n fleetml rollout status deploy/fleetml-server --timeout=5m
```

`--reuse-values` keeps what the install set (`s3.publicEndpoint` here);
without it, pass every `--set` again. A changed server value or secret rolls
the server pods one at a time; a changed `s3SecretKey` also restarts
SeaweedFS, which reads its credentials at start (Postgres keeps its
password, see [Install](#install)). The data stays in the volumes, the
server applies any new database migrations when it starts, and devices
reconnect by themselves. A port-forward ends when the pod behind it is
replaced: start the two `port-forward` lines again. To rotate a secret that
may change (`s3SecretKey`, or `jwtSecret` at the cost of the devices
registering again), edit it in `fleetml-secrets.yaml` and run the same
upgrade. From 0.6.0 to 0.9.0 no
upgrade needs anything else.

## Production

- **TLS in front.** FleetML does not terminate TLS. Put an Ingress (or a
  LoadBalancer with TLS) in front of three services: `fleetml-dashboard`
  (port 80, which also proxies `/api` to the server), `fleetml-server` port
  19090 for devices (gRPC: the Ingress must speak HTTP/2 to it, the Service
  is marked `appProtocol: grpc`; devices then set TLS on), and `fleetml-s3`
  port 8333 for model files. Set `s3.publicEndpoint` to the object store's
  public host and port, for example
  `--set s3.publicEndpoint=models.example.com:443,s3.publicUseSSL=true`:
  the download URLs are then `https://models.example.com/...`, while the
  server keeps reaching the bundled store over plain HTTP inside the cluster.
- **External services.** `postgres.enabled=false` with `postgres.url` (or
  `DATABASE_URL` in your Secret): the database must have the TimescaleDB
  extension, which many managed Postgres offerings lack
  ([CONFIG.md](../../../docs/CONFIG.md#required)). `nats.enabled=false` with
  `nats.url`: JetStream must be enabled. `seaweedfs.enabled=false` with
  `s3.endpoint`, `s3.useSSL` and `s3.bucket` for any S3-compatible store.
- **What the chart does not set.** No Ingress, `securityContext`,
  PodDisruptionBudget, `imagePullSecrets`, `nodeSelector` or tolerations,
  and resources only for the server (`server.resources`); add them with a
  post-renderer or your own manifests.
- **Backups.** The Postgres volume holds the projects, devices, telemetry
  and signing keys (encrypted with `signingMasterKey`: back up the secrets
  file with it); the SeaweedFS volume holds the model files; NATS holds
  telemetry not yet written to the database.

## Values

Every value is in [values.yaml](values.yaml), with a comment. The ones you
are most likely to set:

| Value | Default | Description |
|---|---|---|
| `secrets.*`, `secrets.existingSecret` | | See [Install](#install) |
| `s3.publicEndpoint` | (required) | `host:port` devices and clients reach the object store at |
| `s3.publicUseSSL` | `false` | Whether the download and upload URLs are `https` |
| `s3.useSSL`, `s3.endpoint`, `s3.bucket` | `false`, , `fleetml-models` | The server's connection to the store (`s3.endpoint` with `seaweedfs.enabled=false`) |
| `server.image.repository`, `.tag`, `.pullPolicy` | `ghcr.io/fleetml/fleetml-server`, the app version, `IfNotPresent` | Server image |
| `server.replicas` | `1` | Server replicas (they share the database and the telemetry stream) |
| `server.dependencyTimeout` | `180s` | How long the server waits at start-up for each of Postgres, the object store and NATS (`DEPENDENCY_TIMEOUT`) |
| `server.env` | `{}` | Extra server settings ([CONFIG.md](../../../docs/CONFIG.md)), e.g. `DASHBOARD_URL`; a name the chart sets itself is replaced |
| `server.resources` | `{}` | Requests and limits for the server |
| `server.service.type`, `.restPort`, `.grpcPort` | `ClusterIP`, `18080`, `19090` | The server's Service |
| `dashboard.enabled`, `dashboard.image.*`, `dashboard.service.*` | `true`, the published image, `ClusterIP` port `80` | The dashboard, with `/api` proxied to the server |
| `postgres.enabled`, `.url`, `.image`, `.storage` | `true`, , TimescaleDB 2.30 on Postgres 16, `10Gi` | Bundled Postgres, or the URL of yours |
| `nats.enabled`, `.url`, `.image`, `.storage` | `true`, , NATS 2.12, `2Gi` | Bundled NATS, or yours (JetStream required) |
| `seaweedfs.enabled`, `.image`, `.storage`, `.service.type` | `true`, SeaweedFS 4.48, `10Gi`, `ClusterIP` | Bundled object store |
| `tests.image` | `busybox:1.37` | The image `helm test` checks `/readyz` with |

To try changes to the server or dashboard that are not released yet, build
the two images from the clone (`docker build -t fleetml-server:local .` and
`docker build -t fleetml-dashboard:local dashboard`), load them into the
cluster (`kind load docker-image fleetml-server:local fleetml-dashboard:local --name fleetml`)
and add `--set server.image.repository=fleetml-server,server.image.tag=local,dashboard.image.repository=fleetml-dashboard,dashboard.image.tag=local`.

## Uninstall

```bash
helm uninstall fleetml -n fleetml
```

This keeps the volume claims of the bundled services, so the data survives a
reinstall with the same secrets file; the Secret the chart made is deleted
with the release, which is why the file matters. To delete the data too:
`kubectl -n fleetml delete pvc -l app.kubernetes.io/instance=fleetml`, and
`kubectl delete namespace fleetml` removes the namespace that
`--create-namespace` made (with anything left in it).
`kind delete cluster --name fleetml` removes a kind cluster with everything
in it.

`scripts/helm-e2e.sh` installs the chart into a throwaway kind cluster, runs
`helm test` and the end-to-end suite against it, and uninstalls it.
