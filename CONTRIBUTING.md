# Contributing to FleetML

Thanks for helping. Bug reports, documentation fixes and code are all welcome.
For anything larger than a small fix, please open an issue first so we can
agree on the approach before you spend time on it.

Please report security problems privately, as described in
[SECURITY.md](SECURITY.md), not in a public issue. Everyone taking part is
expected to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## Development setup

| Part | Needs |
|---|---|
| Server, CLI | Go 1.25, Docker (for Postgres/TimescaleDB, SeaweedFS and NATS) |
| Dashboard | Node.js 22 |
| Android SDK | JDK 17 and the Android SDK (an emulator for the instrumented test) |
| iOS SDK | Xcode 16 or later |

```bash
make dev            # creates .env, starts the dev stack, runs the server on :18080 / :19090
cd dashboard && npm ci && npm run dev   # dashboard on :5173
```

[docs/CONFIG.md](docs/CONFIG.md) lists every environment variable.

## Tests

CI (`.github/workflows/ci.yml`) runs these on every pull request; please run
the ones that cover your change before pushing:

| Command | What it runs |
|---|---|
| `make test` | Go unit tests with the race detector |
| `make lint` | golangci-lint (formatting included: `golangci-lint fmt` fixes it) |
| `make test-integration` | Go tests against the dev stack's Postgres and NATS |
| `make e2e` | The end-to-end suite against a real server ([test/e2e](test/e2e)) |
| `cd dashboard && npx tsc && npm run build` | Dashboard type-check and build |
| `cd android && ./gradlew :fleetml-sdk:testDebugUnitTest` | Android SDK unit tests |
| `make ios-test` | iOS SDK tests |
| `make docs-site` | The documentation site in `site/` (MkDocs Material in Docker, strict) and an offline check of its links |

A change in behaviour should come with a test that fails without it. The
Android and iOS guides ([docs/ANDROID_SDK.md](docs/ANDROID_SDK.md),
[docs/IOS_SDK.md](docs/IOS_SDK.md)) describe the emulator and simulator runs.

## Changing the device protocol or the database

- The gRPC protocol lives in `proto/`. Regenerate the Go code with
  `make proto` and the Swift code with `make proto-swift`; the Android SDK
  generates its code at build time. Add fields, do not renumber or reuse them:
  devices in the field run older SDKs.
- Database changes are new numbered migrations in
  `internal/db/migrations`; never edit a migration that has been released.

## Commits and pull requests

- Keep pull requests focused on one change, and explain why in the
  description.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat:`, `fix:`, `docs:`, `test:`, `ci:`, `refactor:`), in the imperative,
  with a body that says why when it is not obvious.
- The person submitting a commit is its author. Please leave out
  tool-generated attribution trailers, such as `Co-Authored-By` lines added by
  coding assistants.
- New environment variables go into `docs/CONFIG.md`; user-visible changes
  into the relevant guide in `docs/`.

## License

FleetML is licensed under the [Apache License 2.0](LICENSE). Contributions are
accepted under the same license.
