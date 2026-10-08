#!/usr/bin/env bash
# Installs the Helm chart (deploy/helm/fleetml) into a throwaway kind cluster,
# runs `helm test` and the end-to-end suite (test/e2e) against it, uninstalls
# the release and deletes the cluster. Needs Docker, kind, helm, kubectl and
# Go. It uses its own kubeconfig, so your current kubectl context is not
# touched. Extra arguments are passed to `go test`.
set -euo pipefail
cd "$(dirname "$0")/.."

CLUSTER=${KIND_CLUSTER:-fleetml-e2e}
WORK=$(mktemp -d -t fleetml-helm.XXXXXX)
export KUBECONFIG="$WORK/kubeconfig"
PIDS=()
cleanup() {
  for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null || true; done
  kind delete cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT
random() { head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n'; }
step() { printf '\n== %s\n' "$*"; }

step "images"
docker build -q -t fleetml-server:kind . >/dev/null
docker build -q -t fleetml-dashboard:kind dashboard >/dev/null

step "kind cluster $CLUSTER"
kind create cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" --wait 120s >/dev/null
kind load docker-image fleetml-server:kind fleetml-dashboard:kind --name "$CLUSTER" >/dev/null

step "helm install"
KEY=$(random)
helm install fleetml deploy/helm/fleetml -f deploy/helm/fleetml/ci/kind-values.yaml \
  --set secrets.jwtSecret="$(random)",secrets.signingMasterKey="$(random)",secrets.adminKey="$KEY" \
  --set secrets.s3SecretKey="$(random)",secrets.postgresPassword="$(random)" \
  --wait --timeout 5m
kubectl get pods -o wide
helm test fleetml | grep -E "^TEST SUITE|^Phase|^Last Started" || true

step "port-forward"
kubectl port-forward svc/fleetml-server 18480:18080 19490:19090 >/dev/null 2>&1 & PIDS+=($!)
kubectl port-forward svc/fleetml-s3 19500:8333 >/dev/null 2>&1 & PIDS+=($!)
kubectl port-forward svc/fleetml-dashboard 18481:80 >/dev/null 2>&1 & PIDS+=($!)
for _ in $(seq 1 60); do curl -sf localhost:18480/readyz >/dev/null && curl -sf localhost:18481/ >/dev/null && break; sleep 1; done
echo "readyz: $(curl -s localhost:18480/readyz)"
echo "dashboard: $(curl -s -o /dev/null -w '%{http_code}' localhost:18481/login), /api through its proxy: $(curl -s -o /dev/null -w '%{http_code}' localhost:18481/api/v1/projects) (401 without a key)"

step "end-to-end suite against the chart"
# The suite's webhook receivers must be reachable from the pods.
CALLBACK=${FLEETML_E2E_CALLBACK_HOST:-}
if [ -z "$CALLBACK" ]; then
  if [ "$(uname)" = Darwin ]; then CALLBACK=host.docker.internal
  else CALLBACK=$(docker network inspect kind -f '{{range .IPAM.Config}}{{if .Gateway}}{{.Gateway}} {{end}}{{end}}' | tr ' ' '\n' | grep -m1 '\.'); fi
fi
FLEETML_E2E_ADMIN_KEY="$KEY" FLEETML_E2E_API=http://localhost:18480 FLEETML_E2E_GRPC=localhost:19490 \
  FLEETML_E2E_CALLBACK_HOST="$CALLBACK" go test -tags e2e -count=1 ./test/e2e "$@"

step "helm uninstall"
helm uninstall fleetml --wait >/dev/null
kubectl wait --for=delete pod -l app.kubernetes.io/instance=fleetml --timeout=120s >/dev/null 2>&1 || true
echo "left: $(kubectl get deploy,statefulset,svc,pod,secret,configmap -l app.kubernetes.io/instance=fleetml -o name | wc -l | tr -d ' ') objects;" \
  "kept by design: $(kubectl get pvc -o name | wc -l | tr -d ' ') volume claims of the bundled services"
