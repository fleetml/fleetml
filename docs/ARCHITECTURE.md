# Architecture

FleetML is one Go server (`cmd/server`) with two APIs, three background loops
and four dependencies, plus device SDKs, a CLI and a web dashboard.

```mermaid
flowchart LR
  subgraph clients[Operators]
    dash[Dashboard<br/>React]
    cli[CLI<br/>fleetml]
  end
  subgraph devices[Devices]
    android[Android SDK]
    ios[iOS SDK]
  end
  subgraph server[FleetML server]
    rest[REST API :18080<br/>Gin]
    grpc[gRPC API :19090]
    reg[Registry]
    dep[Deploy engine]
    intel[Intelligence<br/>profiler, selector]
    exp[Experiments]
    tel[Telemetry]
    drift[Drift]
    alert[Alerts]
  end
  pg[(Postgres +<br/>TimescaleDB)]
  s3[(S3 object store<br/>SeaweedFS)]
  nats[[NATS]]
  hook([Webhook receiver])

  dash -- admin API key --> rest
  cli -- admin API key --> rest
  android -- device token --> grpc
  ios -- device token --> grpc
  android -. presigned GET .-> s3
  ios -. presigned GET .-> s3
  cli -. presigned PUT .-> s3
  rest --> reg & dep & exp & drift
  grpc --> intel & exp & tel
  reg --> pg & s3
  dep --> pg
  intel --> pg
  tel --> nats --> tel
  tel --> pg
  drift --> pg
  dep & drift --> alert
  alert --> nats & hook
```

| Package | Responsibility |
|---|---|
| `internal/api/rest` | Admin REST API under `/api/v1`, probes and `/metrics` |
| `internal/api/grpc` | Device gRPC API, device-token interceptor, `grpc.health.v1` |
| `internal/registry` | Projects, models, versions, variants; upload and integrity check |
| `internal/device` | Registration, tokens, heartbeats |
| `internal/intelligence` | Device profiles, variant selection, deployment-to-assignment planning |
| `internal/rollout` | The consistent-hash ring behind canary and rolling cohorts |
| `internal/deploy` | Deployment state machine, health gate, rollback |
| `internal/experiment` | A/B cohorts, arm IDs, Welch test, conclusion |
| `internal/telemetry` | Ingest, NATS pipeline, queries |
| `internal/drift` | PSI drift checks and history |
| `internal/alert` | Rollback and drift alerts on NATS and a signed webhook |
| `internal/auth` | Admin API keys and enrollment keys |
| `internal/health`, `internal/metrics` | Readiness checks and Prometheus metrics |

## Request paths

### Publishing a model

Model files never pass through the server: the client uploads straight to the
object store with a presigned URL, and the server verifies the result.

```mermaid
sequenceDiagram
  participant C as CLI / API client
  participant R as REST API
  participant S as Object store
  participant D as Postgres
  C->>R: POST /models/{id}/versions
  C->>R: POST /versions/{id}/variants (runtime, size, sha256)
  R->>D: variant UPLOADING
  R-->>C: presigned PUT URL (15 min)
  C->>S: PUT model file
  C->>R: POST /variants/{id}/finalize
  R->>S: stream the object, compute sha256
  alt hash and size match
    R->>D: variant READY
  else mismatch
    R->>D: variant FAILED (422 INTEGRITY_ERROR)
  end
```

Only READY variants are ever assigned to devices.

### Device heartbeat

The heartbeat is the control loop: the device reports what it runs and gets
back what it should run.

```mermaid
sequenceDiagram
  participant SDK as Device SDK
  participant G as gRPC API
  participant DS as Device service
  participant E as Experiment planner
  participant B as Deployment planner
  participant S as Object store
  SDK->>G: Heartbeat(loaded models) + device token
  G->>G: verify token signature and stored hash
  G->>DS: record heartbeat and loaded variants
  DS->>E: running experiments of the device's project
  E-->>DS: arm per experiment, arm models
  DS->>B: active deployments of the device's project
  B-->>DS: newest matching deployment per model,<br/>if the device is inside its cohort
  DS->>DS: pick one READY variant per model<br/>(runtime, RAM, device score)
  DS-->>SDK: LOAD / UPDATE / UNLOAD with presigned GET URLs,<br/>experiment arm IDs, config
  SDK->>S: download (resumable), verify sha256
  SDK->>SDK: validate the model, activate it,<br/>keep the previous as last-known-good
```

Planning rules:

- A device only sees deployments and experiments of the project its
  enrollment key belongs to.
- Experiments take precedence over deployments for the models they cover.
- Per model, the newest active deployment that targets the device and exposes
  it wins; a version replaced by another arrives as UPDATE (new) plus UNLOAD
  (old), and the SDK keeps the old one until the new one validates.
- Cohorts come from a consistent-hash ring (`internal/rollout`): a device's
  position is the first four bytes of `sha256(device ID + deployment ID)`.
  Canary stages expose positions below a percentage, rolling batches below a
  growing threshold, so cohorts only grow and health statistics are computed
  over exactly the exposed devices.

### Telemetry

```mermaid
flowchart LR
  sdk[SDK queue<br/>persistent, bounded] -- ReportBatch --> v{valid, and the version<br/>belongs to the device's project?}
  v -- no --> rej[rejected count]
  v -- yes --> pub[publish to NATS]
  pub --> cons[consumer<br/>batches of 500 or 5 s]
  cons --> ht[(telemetry_events<br/>hypertable, 30-day retention)]
  ht --> agg[(telemetry_hourly_stats<br/>continuous aggregate)]
  pub -. NATS down .-> direct[direct insert] --> ht
```

Events for a model in an experiment carry the device's arm ID, so results are
computed per arm.

## Background loops

Three tickers run in the server; their intervals are configurable
([CONFIG.md](CONFIG.md#background-loops)) and each run is counted in
`fleetml_ticker_runs_total`.

| Loop | Default | What it does |
|---|---|---|
| Deploy | 30 s | For each ROLLING_OUT deployment: evaluate the health gate over the exposed cohort, then advance the stage or batch, complete, pause or roll back |
| Experiment | 60 s | Evaluate running experiments once both arms reach the minimum sample (Welch's t-test), conclude expired ones |
| Drift | 5 min | Compare each active version's last 24 h of latency and confidence with the 7 days before (PSI); record changes of severity; alert on a change into CRITICAL |

```mermaid
flowchart TD
  t[deploy tick] --> list[ROLLING_OUT deployments]
  list --> gate{health gate:<br/>error rate and p95 latency<br/>of the exposed cohort}
  gate -- not enough events --> wait[wait]
  gate -- fails --> fail{on_failure}
  fail -- ROLLBACK --> rb[ROLLED_BACK<br/>revert deployment + alert]
  fail -- PAUSE --> pause[PAUSED]
  gate -- passes --> time{stage time elapsed?}
  time -- no --> wait
  time -- yes --> last{last stage?}
  last -- no --> next[next canary stage /<br/>next rolling batch]
  last -- yes --> done[COMPLETE]
```

## Data model

```mermaid
erDiagram
  projects ||--o{ models : has
  projects ||--o{ enrollment_keys : has
  projects ||--o{ deployments : has
  projects ||--o{ experiments : has
  projects ||--o{ devices : enrolls
  models ||--o{ model_versions : has
  model_versions ||--o{ model_variants : "built as"
  model_versions ||--o{ deployments : "rolled out by"
  model_versions ||--o{ experiments : "control / treatment"
  model_versions ||--o{ telemetry_events : "reported for"
  model_versions ||--o{ drift_results : "checked in"
  devices ||--o{ telemetry_events : reports
  devices ||--o{ cohort_assignments : "assigned in"
  experiments ||--o{ cohort_assignments : has
  device_classes ||--o{ devices : classifies

  model_variants {
    uuid id
    string target_runtime
    string quantization
    string blob_sha256
    string status "UPLOADING, READY, FAILED"
  }
  deployments {
    uuid id
    string strategy "ALL_AT_ONCE, CANARY, ROLLING"
    string status
    jsonb target_selector
    jsonb canary_stages
    jsonb health_gate
    bigint rollout_threshold
    uuid previous_version_id
  }
  devices {
    uuid id
    uuid project_id
    jsonb labels_json
    string auth_token_hash
    jsonb loaded_variant_ids
  }
  telemetry_events {
    timestamptz recorded_at
    string event_type
    int latency_ms
    float confidence_score
    uuid experiment_arm_id "derived from experiment and arm"
  }
```

`api_keys` (admin keys, sha256 only) and `audit_logs` stand alone. Experiment
arm IDs are UUIDv5 values derived from the experiment ID and the arm name, not
rows.

## Device protocol

Defined in [`proto/fleetml/v1`](../proto/fleetml/v1).

| RPC | Credential | Purpose |
|---|---|---|
| `DeviceService/Register` | enrollment key | Register (or re-register) by hardware fingerprint; returns device ID, token and `DeviceConfig` |
| `DeviceService/Heartbeat` | device token | Report loaded models; receive assignments, experiment arms, next interval and the current `DeviceConfig` |
| `DeviceService/RefreshToken` | device token | Swap a token in its last 7 days for a new one |
| `TelemetryService/ReportBatch` | device token | Send up to a batch of events; returns accepted and rejected counts |

### What every client implements

The Android and iOS SDKs follow these rules, and any other client (such as a
device agent) must too:

- **Identity.** Register with the enrollment key and a stable fingerprint
  (64 hex characters); keep the token, refresh it in its last 7 days, and
  re-register when a call returns `UNAUTHENTICATED`.
- **Configuration.** Apply `DeviceConfig` from `Register` and from every
  heartbeat: `heartbeat_interval_seconds` while active,
  `background_heartbeat_interval_seconds` while in the background (one
  heartbeat and one telemetry flush per run), `telemetry_batch_size`,
  `telemetry_flush_interval_seconds`, `max_cache_size_mb`, and
  `download_policy`.
- **Downloads.** Download an assignment only when `download_policy` allows it
  (`UNMETERED_OR_CHARGING` by default: an unmetered network, or any network
  while charging; `UNMETERED_ONLY`; `ANY_NETWORK`), unless the assignment is
  `urgent`, which needs only a connection. Resume interrupted downloads with an
  HTTP `Range` request and check the sha256 before using the file.
- **Activation.** Validate a downloaded model before switching to it; on
  failure keep the current version (last-known-good) and report
  `MODEL_LOAD_FAILED`. A replacement arrives as `UPDATE` (new) plus `UNLOAD`
  (old): keep the old one until the new one is active.
- **Telemetry.** Queue events persistently, send them in batches (never one
  request per event), tag events of a model in an experiment with its
  `arm_id`, and back off after failures.

Deployments and the model lifecycle on a device:

```mermaid
stateDiagram-v2
  direction LR
  state "Deployment (server)" as server {
    [*] --> PENDING
    PENDING --> ROLLING_OUT: start
    PENDING --> CANCELLED: cancel
    ROLLING_OUT --> PAUSED: pause / gate
    PAUSED --> ROLLING_OUT: resume
    ROLLING_OUT --> COMPLETE: last stage
    ROLLING_OUT --> ROLLED_BACK: rollback / gate
    PAUSED --> ROLLED_BACK
    COMPLETE --> ROLLED_BACK: rollback
    ROLLING_OUT --> CANCELLED
    PAUSED --> CANCELLED
  }
  state "Model on the device (SDK)" as sdk {
    [*] --> Downloading: LOAD / UPDATE
    Downloading --> Downloading: interrupted, resume with Range
    Downloading --> Verifying
    Verifying --> Validating: sha256 matches
    Verifying --> Failed: mismatch
    Validating --> Active: model loads and runs
    Validating --> Failed: MODEL_LOAD_FAILED reported,<br/>retry after 1 h
    Active --> LastKnownGood: newer version activated
  }
```

## Trust boundaries

```mermaid
flowchart LR
  subgraph untrusted[Untrusted: devices in the field]
    dev[Device SDK]
  end
  subgraph ops[Operators]
    op[Dashboard, CLI, scripts]
  end
  subgraph edge[Your TLS proxy / load balancer]
    tls[TLS termination]
  end
  subgraph core[FleetML server]
    restb[REST /api/v1<br/>admin API key]
    probes["/livez, /readyz, /metrics<br/>no credentials"]
    grpcb[gRPC<br/>enrollment key or device token]
  end
  subgraph data[Private network]
    pgb[(Postgres)]
    natsb[[NATS]]
    s3b[(Object store)]
  end
  dev --> tls --> grpcb
  op --> tls --> restb
  monitor[Monitoring] --> probes
  dev -- presigned URL --> s3b
  op -- presigned URL --> s3b
  restb & grpcb --> pgb & natsb & s3b
```

| Credential | Held by | Grants | Lifetime |
|---|---|---|---|
| Admin API key | operators | every `/api/v1` route | until revoked |
| Enrollment key | apps (built in) | registering a device into one project | until rotated or revoked |
| Device token | one device | its own heartbeats, token refresh and telemetry, within its project | 30 days by default, refreshable, revocable |
| Presigned URL | one client | reading (1 h) or writing (15 min) one object | until it expires |
| Webhook secret | server and receiver | proves an alert came from this server (HMAC-SHA256) | until changed |

The server keeps only hashes of API keys, enrollment keys and device tokens.
Devices verify every model's sha256 before use, so a tampered object store or
download cannot activate a different file. TLS is not built in: terminate it in
front of the server (see [SECURITY.md](../SECURITY.md)).
