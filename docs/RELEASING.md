# Releasing

How a FleetML release is cut, in order. Releases are made by the maintainers
from `main`.

## Release plan up to the public launch

The repository stays private until v0.3.0. The three versions are tagged in
the private repository, on the boundary commits recorded when each version
was closed, with steps 1 to 4 below; the first CI run on GitHub's runners
happens with that push, while the repository is still private. The visibility
change (step 6), branch protection (step 7) and the announcement (step 8)
happen once, after v0.3.0, and only when `main` is green. GitHub does not
offer branch protection or rulesets on a private repository of a free
organisation, so `main` is protected right after it becomes public, before
anything is announced. From then on every release follows all the steps.

## 1. Check the release candidate

On an up-to-date `main` with a clean working tree:

```bash
make lint test                       # Go: golangci-lint, unit tests with -race
make test-integration                # Go: integration tests against the dev stack
make e2e                             # end-to-end suite against a fresh server
(cd dashboard && npm ci && npx tsc && npm run build)
(cd android && ./gradlew :fleetml-sdk:testDebugUnitTest)
make ios-test
make docs-site                       # docs site builds (strict) and its links resolve
./scripts/demo.sh                    # the 60-second story, from a fresh dev stack
```

Also:

- the CI workflow is green for the release commit, and the weekly E2E
  workflow's last run passed (run it by hand from the Actions tab if needed);
- with an emulator running, `./scripts/android-e2e.sh` passes
  ([Android runbook](ANDROID_SDK.md#emulator-runbook)); the
  [iOS simulator runbook](IOS_SDK.md#simulator-runbook) works;
- the README quickstart works from a fresh clone, and the
  [tour](QUICKSTART.md#tour) runs as written;
- `git grep` finds no credentials, internal hostnames or personal paths.

## 2. Update the changelog

In `CHANGELOG.md`, replace `Unreleased` in the release's heading with today's
date (`## [0.1.0] - YYYY-MM-DD`), and make sure the section describes every
user-visible change since the previous release. The CLI (`fleetml version`
in `internal/cli/root.go`), the Linux agent (`AgentVersion` in
`internal/agent/agent.go`) and the dashboard (`dashboard/package.json`) carry
the release's version. Bump the SDK versions only if the SDKs changed
(`sdkVersion` in `android/gradle.properties`, which is both
`BuildConfig.SDK_VERSION` and the Maven Central version, and
`FleetML.sdkVersion` in the iOS package). Commit:

```bash
git commit -am "chore: release v0.1.0"
```

## 3. Tag

Create an annotated tag on that commit and push the commit and the tag:

```bash
git tag -a v0.1.0 -m "FleetML v0.1.0"
git push origin main v0.1.0
```

Tags are never moved or deleted once pushed; a broken release is fixed with the
next patch version.

## 4. Release notes

On GitHub, **Releases → Draft a new release**, choose the tag, title it
`FleetML v0.1.0`, and paste the version's `CHANGELOG.md` section as the notes,
followed by:

```markdown
**Try it:** see the [quickstart](https://github.com/fleetml/fleetml#quickstart) or run `./scripts/demo.sh`.
```

Mark pre-1.0 releases as the latest release (not a pre-release) once they are
the recommended version.

## 5. Publish the Android SDK

When the Android SDK's version changed, run the **Android publish** workflow
from the Actions tab (**Run workflow** on `main`) with that version. The
workflow checks that it equals `sdkVersion` in `android/gradle.properties`,
then builds `dev.fleetml:fleetml-sdk`, signs it and publishes it to Maven
Central through the Central Portal, which releases it without a manual step.
A version on Maven Central can never be replaced, so an unchanged SDK is not
published again.

The workflow reads five repository secrets:

- `MAVEN_CENTRAL_USERNAME` and `MAVEN_CENTRAL_PASSWORD`: the user token
  generated on [central.sonatype.com](https://central.sonatype.com) for the
  account that owns the `dev.fleetml` namespace;
- `MAVEN_SIGNING_KEY`: the ASCII-armoured private OpenPGP key;
- `MAVEN_SIGNING_KEY_ID`: its short key ID (the last 8 hex characters of the
  fingerprint), the form Gradle's signing plugin accepts;
- `MAVEN_SIGNING_KEY_PASSWORD`: the key's passphrase.

The public key must be on a keyserver that Maven Central checks, such as
keyserver.ubuntu.com. The first version published to Maven Central was 0.3.0.

## 6. Make the repository public

Once, after v0.3.0, and only when the CI workflow is green for `main`. In
**Settings → General → Danger Zone → Change repository visibility**, make it
public. Then, in **Settings → Code security**, enable private vulnerability
reporting (so the security policy's reports can also arrive there) and
Dependabot alerts.

Check the public view: the README renders with its links, the Actions tab shows
the release commit green, and the release is listed.

## 7. Protect `main`

Immediately after step 6, before any announcement (branch protection and
rulesets are not available while the repository is private on the free
organisation plan). In **Settings → Branches → Add branch protection rule**
(or a ruleset) for `main`:

- require a pull request before merging;
- require the CI checks to pass (`Go`, `Dashboard`, `Android SDK`, `Docs site`,
  `Helm chart`); not `iOS SDK`, which runs only when `ios/` changes and would
  otherwise block other pull requests;
- block force pushes and deletion of the branch.

From then on every change to `main` goes through a pull request.

## 8. Announce

Post the announcement with the 60-second story (the README's recording of
`scripts/demo-agent.sh`) and a link to the release. Answer issues and
discussions from the project's maintainer account.

## After the release

Add a section for the next version at the top of `CHANGELOG.md`
(`## [0.4.0] - Unreleased`) with its compare link at the bottom.
