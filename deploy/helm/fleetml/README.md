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
last steps the `fleetml` CLI (step 3 of the
[README's Get started](../../../README.md#get-started)) and `jq`. [Get started](#get-started) makes the cluster with
[kind](https://kind.sigs.k8s.io/), in Docker.

## Get started

The chart on a [kind](https://kind.sigs.k8s.io/) cluster on this machine,
with twenty simulated devices: copy and paste, nothing to choose. Each step
says what you should see; if you see something else, stop there and look it
up in [Troubleshooting](../../../docs/ADVANCED.md#troubleshooting). Use one
terminal and keep it open until the end. Before you start: Docker, `kind`,
Helm 3, `kubectl`, `jq`, `openssl` and the `fleetml` CLI (step 3 of the
[README's Get started](../../../README.md#get-started)).

1. **Check the tools**, with this terminal's `kubectl` and `helm` pointed at
   a kubeconfig file of their own, so your other clusters and contexts stay
   as they are:

   <!-- golden: helm-tools -->
   ```bash
   export KUBECONFIG=$HOME/.kube/fleetml-kind.yaml
   kind version && helm version --short && kubectl version --client && jq --version && fleetml version
   ```

   You should see `kind v` and `✓ fleetml v`.

2. **A cluster**, written into that file:

   <!-- golden: helm-cluster -->
   ```bash
   [ -f deploy/docker-compose.prod.yml ] || { [ -d fleetml ] || git clone https://github.com/fleetml/fleetml.git; cd fleetml; }
   kind create cluster --name fleetml
   ```

   You should see `Set kubectl context to "kind-fleetml"`.

3. **The secrets**, generated once into a file only you can read (keep it:
   [why](#install)):

   <!-- golden: helm-secrets -->
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

   You should see nothing: the block prints no secret.

4. **Install the chart** and wait for it. The first line names the four
   local ports this machine reaches the release on (other than the
   quickstart's, so both can run here); they are variables of this shell,
   so in a new terminal (for [Upgrading](#upgrading)) set the same line
   again first:

   <!-- golden: helm-install -->
   ```bash
   REST=38080 GRPC=39090 S3=39100 DASH=38800
   helm install fleetml deploy/helm/fleetml -n fleetml --create-namespace \
     -f fleetml-secrets.yaml --set s3.publicEndpoint=localhost:$S3
   kubectl -n fleetml rollout status deploy/fleetml-server --timeout=5m
   kubectl -n fleetml port-forward svc/fleetml-server $REST:18080 $GRPC:19090 >/dev/null &
   kubectl -n fleetml port-forward svc/fleetml-s3 $S3:8333 >/dev/null &
   for i in $(seq 180); do curl -sf localhost:$REST/readyz && break; [ $i = 180 ] && echo "Not ready after 3 minutes: see Troubleshooting in docs/ADVANCED.md"; sleep 1; done; echo
   helm test fleetml -n fleetml
   ```

   You should see `"status":"ok"` and `Succeeded`. The chart's own notes,
   printed in the middle, show the same port-forwards for later: this block
   already started them.

5. **Put devices on it**: the CLI, the sample model, an enrollment key, 20
   simulated devices and a deployment, as in the
   [tour](../../../docs/QUICKSTART.md#tour):

   <!-- golden: helm-tryit -->
   ```bash
   export FLEETML_SERVER=http://localhost:$REST FLEETML_API_KEY=$(sed -n 's/^  adminKey: "\(.*\)"$/\1/p' fleetml-secrets.yaml)
   P=$(fleetml project create --name helm-tour -o json | jq -r .id)
   fleetml push models/iris/iris_v1.tflite --project $P --model iris --version 1.0.0
   V1=$(fleetml ver list --model $(fleetml model list --project $P -o json | jq -r '.[0].id') -o json | jq -r '.[0].id')
   export FLEETML_ENROLLMENT_KEY=$(fleetml enrollment-key create --project $P --name helm -o json | jq -r .key)
   fleetml simulate --grpc localhost:$GRPC --devices 20 --label app=helm --seed helm-tour > /tmp/fleetml-helm-sim.log 2>&1 &
   SIM=$!
   fleetml deploy start $(fleetml deploy create --project $P --version $V1 --target '{"app":"helm"}' -o json | jq -r .id)
   sleep 10; tail -1 /tmp/fleetml-helm-sim.log
   ```

   You should see `✓ Deployment start → COMPLETE` and `simulator: iris 1.0.0 on 20`.

6. **See it in the dashboard.** This forwards it to the fourth port of step
   4, puts the admin key on the clipboard (on a Mac, in WSL, or on Linux with
   `xclip`; elsewhere it prints it) and prints the address: open it and
   paste the key into the login box:

   <!-- golden: helm-dashboard manual -->
   ```bash
   kubectl -n fleetml port-forward svc/fleetml-dashboard $DASH:80 >/dev/null &
   printf %s "$FLEETML_API_KEY" | { pbcopy || clip.exe || xclip -selection clipboard || cat; } 2> /dev/null; echo
   echo "Dashboard: http://localhost:$DASH"
   ```

   You should see `Dashboard: http://localhost:38800`, and in the browser the
   Overview page (**Dashboard** in the menu); **Devices** lists the 20 devices.

7. **Clean up**: the simulator, the port-forwards, the cluster with all its
   data, its kubeconfig file and the secrets file:

   <!-- golden: helm-cleanup cleanup -->
   ```bash
   kill $(jobs -p) 2> /dev/null; wait 2> /dev/null
   kind delete cluster --name fleetml
   rm -f "$KUBECONFIG" fleetml-secrets.yaml /tmp/fleetml-helm-sim.log
   ```

   You should see `Deleting cluster "fleetml"`. The first line stops this
   terminal's background jobs: the simulator and the port-forwards.
   `kubectl` keeps a cache of the cluster's API in `~/.kube/cache`, which
   does no harm (`rm -rf ~/.kube/cache` removes it).

Then [Upgrading](#upgrading), [Production](#production) and the
[values](#values); a Secret of your own, other local ports and unreleased
images: [ADVANCED.md](../../../docs/ADVANCED.md#helm-chart-variants).

## Install

[Get started](#get-started)'s steps 3 and 4 are the install. About the
secrets file of step 3:

**Keep this file, and never generate it again for a release that has data.**
Every install and upgrade of this release passes it. With new values:
`postgresPassword` no longer opens the database (Postgres sets it when its
volume is created, and the server then fails with `password authentication
failed`), `signingMasterKey` no longer decrypts the stored signing keys (so
publishing fails), and a new `jwtSecret` invalidates every device token (the
devices register again with their enrollment key). A new `s3SecretKey` is
safe: the store restarts with it. `adminKey` is stored on the first start
only. A Secret of your own instead of the file: [ADVANCED.md](../../../docs/ADVANCED.md#helm-chart-variants).
The chart refuses a `jwtSecret`, `signingMasterKey` or `adminKey` shorter
than 32 characters (the server would not start with one), and the first two
must differ. The file name is in the repository's `.gitignore`, so a clone
does not commit it; keep a copy outside the clone as well.

About step 4: `/readyz` answers `{"db":"connected","nats":"connected","s3":"connected","status":"ok","version":"0.10.4"}`,
and `helm test` ends with `Phase: Succeeded`. The first install pulls the
images, which takes a minute or two (the server waits meanwhile, up to
`server.dependencyTimeout`, 180 s in total for Postgres, the object store and
NATS, and its `/livez` answers all along, so the probes leave it alone);
`kubectl -n fleetml get pods` then shows every pod `Running` with no
restarts. On a link slow enough that the images take longer than that, the
server exits with `start-up gave up: DEPENDENCY_TIMEOUT is the total wait
for all dependencies`, and each restart waits another
`server.dependencyTimeout`, so it comes up once the images are there; raise
`server.dependencyTimeout` to avoid the restarts.

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

[Get started](#get-started)'s step 5. The deployment starts as `COMPLETE` and the simulator's last line reads
`simulator: iris 1.0.0 on 20`. `kill $SIM` stops the simulator; step 6 serves the dashboard at
`http://localhost:$DASH` (38800 unless you changed step 4's line; log in with the `adminKey`).

## Upgrading

Change a value, or move the clone to a new release for a new chart and
images (`git fetch --tags && git checkout v<version>`, in the clone), then
upgrade with the same secrets file.

**First, once, for a release installed with chart 0.10.2 or older** (the
`CHART` column of `helm list -n fleetml` says `fleetml-0.10.2` or lower):
those charts put the chart version into the labels of the bundled Postgres,
NATS and SeaweedFS volume templates, a StatefulSet field Kubernetes does
not let an upgrade change, so the upgrade would stop with
`UPGRADE FAILED: cannot patch "fleetml-nats" with kind StatefulSet: ... spec.volumeClaimTemplates: Invalid value`.
Remove the three StatefulSets but not their pods and volumes (the upgrade
below recreates them, they take over the pods, and the data stays):

```bash
kubectl -n fleetml delete statefulset fleetml-postgres fleetml-nats fleetml-s3 --cascade=orphan
```

You should see three `statefulset.apps "fleetml-<name>" deleted from fleetml namespace` lines
(with a release name other than `fleetml`, the names start with it). The
same line also finishes an upgrade that already stopped with that error:
run it, then the upgrade again. Charts from 0.10.3 on keep those labels the
same, so later upgrades need nothing else. The upgrade:

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
reconnect by themselves. A port-forward stops working when the pod behind
it is replaced, but keeps its local port, so starting the same line again
stops with `address already in use`. After an upgrade (or an uninstall and
a new install), stop this terminal's port-forwards and start step 4's two
and step 6's dashboard one again:

```bash
kill $(jobs -p) 2> /dev/null; wait 2> /dev/null
kubectl -n fleetml port-forward svc/fleetml-server $REST:18080 $GRPC:19090 >/dev/null &
kubectl -n fleetml port-forward svc/fleetml-s3 $S3:8333 >/dev/null &
kubectl -n fleetml port-forward svc/fleetml-dashboard $DASH:80 >/dev/null &
for i in $(seq 30); do curl -sf localhost:$REST/readyz && break; sleep 1; done; echo
```

You should see the `/readyz` line; the dashboard is back on
`http://localhost:$DASH`. The first line also stops step 5's
simulator, a job of this terminal: start it again (step 5's
`fleetml simulate` line) if you want it. To rotate a secret that
may change (`s3SecretKey`, or `jwtSecret` at the cost of the devices
registering again), edit it in `fleetml-secrets.yaml` and run the same
upgrade.

`helm rollback fleetml <revision> -n fleetml` returns to an earlier
revision (`helm history fleetml -n fleetml` lists them). Back to a revision
made with chart 0.10.2 or older it stops with the same
`volumeClaimTemplates` error and leaves the release `failed`, half on each
version: run the upgrade above again to return to the current chart (the
data stays), and stay on charts from 0.10.3 on.

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

Every value is in [values.yaml](values.yaml), with a comment, and
[values.schema.json](values.schema.json) checks them on every `helm install`,
`upgrade`, `template` and `lint`: a misspelled key, a port that is not a
number or an `s3.publicEndpoint` with `http://` stops with the value's path
(`at '/server/replicas': got string, want integer`). The ones you are most
likely to set:

| Value | Default | Description |
|---|---|---|
| `secrets.*`, `secrets.existingSecret` | | See [Install](#install) |
| `s3.publicEndpoint` | (required) | `host:port` devices and clients reach the object store at, without `http://` or `https://` (`s3.publicUseSSL` makes the URLs https); a value with a scheme stops with `at '/s3/publicEndpoint': ... does not match pattern` |
| `s3.publicUseSSL` | `false` | Whether the download and upload URLs are `https` |
| `s3.useSSL`, `s3.endpoint`, `s3.bucket` | `false`, , `fleetml-models` | The server's connection to the store (`s3.endpoint` with `seaweedfs.enabled=false`) |
| `server.image.repository`, `.tag`, `.pullPolicy` | `ghcr.io/fleetml/fleetml-server`, the app version, `IfNotPresent` | Server image |
| `server.replicas` | `1` | Server replicas (they share the database and the telemetry stream) |
| `server.dependencyTimeout` | `180s` | How long the server waits at start-up for Postgres, the object store and NATS, in total (`DEPENDENCY_TIMEOUT`) |
| `server.env` | `{}` | Extra server settings ([CONFIG.md](../../../docs/CONFIG.md)), e.g. `DASHBOARD_URL`; a name the chart sets itself is replaced, except `REST_PORT` and `GRPC_PORT`, which the chart refuses (the pod listens on 18080 and 19090; `server.service.restPort`/`grpcPort` change the Service's ports) |
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
`--create-namespace` made (with anything left in it). Stop this terminal's
port-forwards too (`kill $(jobs -p)`): left running, they hold the local
ports, and a new install's port-forwards stop with `address already in use`.
`kind delete cluster --name fleetml` removes a kind cluster with everything
in it.

`scripts/helm-e2e.sh` installs the chart into a throwaway kind cluster, runs
`helm test` and the end-to-end suite against it, and uninstalls it.
