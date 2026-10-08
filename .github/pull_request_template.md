## What and why

<!-- What this changes and the problem it solves. Link the issue if there is one. -->

## How it was tested

<!-- Commands you ran and what you checked by hand. -->

- [ ] `make test` and `make lint` pass
- [ ] Integration or e2e tests added or updated where behaviour changed (`make test-integration`, `make e2e`)
- [ ] SDK or agent changes: unit tests pass (`cd android && ./gradlew :fleetml-sdk:testDebugUnitTest`, `make ios-test`), and the emulator, simulator or `scripts/signing-e2e.sh agent` check was run if the device protocol changed
- [ ] Migrations, if any, have a working `down` and are listed in `CHANGELOG.md`
- [ ] No keys, tokens, hostnames or personal paths in the diff, logs or screenshots
- [ ] Docs updated (README, `docs/`, `docs/CONFIG.md` for new environment variables)

## Notes for reviewers

<!-- Breaking changes, migrations, follow-ups. -->
