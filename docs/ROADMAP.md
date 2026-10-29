# Roadmap

What comes next, roughly in order. Nothing under Next or Ideas is in a
release yet; the [README](../README.md#features) lists what is. Each item
lands with tests and documentation, and the README only claims it once it
runs.

## Next

- **Sigstore** as an alternative to the built-in artifact signing keys.
- **Organisations and single sign-on.** Several organisations on one server,
  and dashboard login through OIDC next to API keys.

## Ideas

- React Native and Flutter wrappers over the native SDKs.
- Upstream sources from model registries such as MLflow, the Hugging Face Hub
  and Weights & Biases.

## Shipped

Items from earlier versions of this page, with the release that brought them
([CHANGELOG](../CHANGELOG.md) has the details):

- Battery and data policy in the Android and iOS SDKs: 0.1.0.
- The Linux device agent: 0.2.0.
- Dashboard sign-in with server-side sessions instead of a key in the
  browser: 0.2.0.
- Slack and email alert sinks: 0.2.0.
- Artifact signing with per-project ed25519 keys: 0.3.0 (Sigstore is under
  Next).
- Feature-level drift: 0.3.0.
- At-least-once telemetry through NATS JetStream, and a Grafana dashboard:
  0.3.0.
- Delta updates: 0.3.0.
- Users, per-project roles and the audit log: 0.3.0 (organisations are under
  Next).
- The Helm chart: 0.3.0.

Suggestions are welcome as [feature requests](https://github.com/fleetml/fleetml/issues/new/choose).
