# Roadmap

What comes after v0.1.0, roughly in order. Nothing here is available yet;
the [README](../README.md#features) lists what is. Each item lands with tests
and documentation, and the README only claims it once it runs.

## Next

- **Linux device agent.** A single static Go binary on the same protocol as the
  SDKs, running as a systemd service, for Jetson, Raspberry Pi, x86 edge boxes,
  kiosks and robots.
- **Battery and background policy in the SDKs.** Heartbeats on a foreground
  timer, background checks through WorkManager (Android) and
  BGAppRefreshTask (iOS), downloads on an unmetered network or while
  charging unless a deployment is marked urgent, all tunable from the
  server's device configuration.
- **More alert sinks.** Slack and email next to NATS and the signed webhook.
- **Dashboard sign-in.** Exchange the admin API key for a session instead of
  keeping it in the browser's local storage.

## Later

- **Artifact signing.** Per-project ed25519 keys (optionally Sigstore),
  verified on the device before a model is activated.
- **Feature-level drift.** Input histograms from devices and PSI per feature,
  next to today's latency and confidence drift.
- **At-least-once telemetry.** NATS JetStream with acknowledgement after the
  database insert, and Grafana dashboards for the Prometheus metrics.
- **Delta updates** for large models.
- **Roles and organisations.** Multiple organisations and projects with roles,
  and the audit log in the dashboard.
- **Helm chart** and a Kubernetes reference deployment.

## Ideas

- React Native and Flutter wrappers over the native SDKs.
- Upstream sources from model registries such as MLflow, the Hugging Face Hub
  and Weights & Biases.

Suggestions are welcome as [feature requests](https://github.com/fleetml/fleetml/issues/new/choose).
