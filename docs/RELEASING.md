# Releasing

How a FleetML release is cut, in order. Releases are made by the maintainers
from `main`.

## The public repository

The repository was private up to v0.6.0, which was tagged and released
there like the versions before it, and has been public since 2026-10-23.
Every release follows all the steps below. The settings made when it became
public, and the branch rule on `main`, are recorded in sections 6 and 7;
they are made once and stay in place.

## Release checklist

Every release, in this order; the numbered sections below have the details.

- [ ] `CHANGELOG.md` section dated; versions bumped where they changed
      (dashboard, `__version__` of the Python client, `sdkVersion` in
      `android/gradle.properties`, `FleetML.sdkVersion` in the iOS package);
      the CLI and the agent take theirs from the tag.
- [ ] CI green on `main` at the release commit (Go, CLI on Windows, Python
      client, Dashboard, Android SDK, Docs site, Helm chart, Shell scripts,
      iOS).
- [ ] Annotated tag `vX.Y.Z` on that commit, pushed; tags are never moved.
- [ ] GitHub release from the tag, with the changelog section as its body.
- [ ] Binaries: the **Release** workflow attached the eight archives and
      `SHA256SUMS` to that release.
- [ ] Images: the same workflow's **images** job pushed
      `ghcr.io/fleetml/fleetml-server:X.Y.Z` and
      `ghcr.io/fleetml/fleetml-dashboard:X.Y.Z` (and `latest`).
- [ ] Check that `docker pull ghcr.io/fleetml/fleetml-server:X.Y.Z` works
      without logging in: the Helm chart's default values pull the images
      anonymously. Both packages are public (section 6); a package for a new
      image starts private and has to be made public the same way.
- [ ] **Windows run** workflow green with the release tag as `tag`, on
      `windows-latest` and on `windows-11-arm`.
- [ ] Android: run the **Android publish** workflow with `X.Y.Z` when
      `sdkVersion` changed, then check that
      `https://repo1.maven.org/maven2/dev/fleetml/fleetml-sdk/X.Y.Z/` answers.
- [ ] Python: run the **Python publish** workflow with `X.Y.Z`, then check
      that `https://pypi.org/project/fleetml/X.Y.Z/` answers.
- [ ] iOS: nothing to publish; Swift Package Manager reads the tag.
- [ ] Announce only after the release, the Android artifact and the docs site
      are live.

## 1. Check the release candidate

On an up-to-date `main` with a clean working tree:

```bash
make lint test
make test-integration
make e2e
(cd dashboard && npm ci && npx tsc && npm run build)
(cd android && ./gradlew :fleetml-sdk:testDebugUnitTest)
make ios-test
make docs-site
make release-binaries VERSION=0.1.0
./scripts/demo.sh
```

That is golangci-lint and the Go unit tests with `-race`; the integration
tests against the dev stack; the end-to-end suite against a fresh server; the
dashboard, Android and iOS checks; the docs site, built strict with its links
checked; the release binaries into `dist/`; and the 60-second story from a
fresh dev stack.

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
user-visible change since the previous release. The dashboard
(`dashboard/package.json`) and the Python client (`__version__` in
`python/src/fleetml/__init__.py`, the only place its version is written;
`pyproject.toml` reads it) carry the release's version. The CLI and the
Linux agent have no version in the source: the release binaries get it from
the tag (`internal/version`, set with `-ldflags -X` by
`scripts/release-binaries.sh`), and a build from source reports `dev`, or the
tag's version when built from a clean checkout of the tag. Bump the SDK
versions only if the SDKs changed (`sdkVersion` in
`android/gradle.properties`, which is both `BuildConfig.SDK_VERSION` and the
Maven Central version, and `FleetML.sdkVersion` in the iOS package). Commit:

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

Pushing the tag starts the **Release** workflow
(`.github/workflows/release.yml`). It builds the binaries from the tag with
`scripts/release-binaries.sh`, the same files `make release-binaries` writes
to `dist/`:

- the CLI: `fleetml-linux-amd64.tar.gz`, `fleetml-linux-arm64.tar.gz`,
  `fleetml-darwin-amd64.tar.gz`, `fleetml-darwin-arm64.tar.gz`,
  `fleetml-windows-amd64.zip` and `fleetml-windows-arm64.zip`;
- the Linux agent, with its installer: `fleetml-agent-linux-amd64.tar.gz` and
  `fleetml-agent-linux-arm64.tar.gz`;
- `SHA256SUMS` for all eight.

The script archives every file owned by `0:0`, without the builder's user
and group names and without extended attributes, with GNU tar on Linux and
BSD tar on macOS alike. A tarball built by hand on a Mac (a stand-in for a
test, say) needs the same, or it carries AppleDouble `._*` files and
`LIBARCHIVE.xattr` headers that GNU tar warns about when extracting:
`COPYFILE_DISABLE=1 tar --no-xattrs --owner=0 --group=0 -czf <archive>.tar.gz <files>`.

It attaches them to the tag's GitHub release, creating the release as a draft
if there is none yet, and it never changes a release's notes. The binaries get
their version from the tag.

Its **images** job builds the server and dashboard images from the tag's two
Dockerfiles for linux/amd64 and linux/arm64 and pushes them to ghcr.io as
`ghcr.io/fleetml/fleetml-server:X.Y.Z` and `ghcr.io/fleetml/fleetml-dashboard:X.Y.Z`,
plus `latest` unless the tag is a pre-release (`v0.5.0-rc.1`); these are the
Helm chart's default images. Both packages are public; a package for a new
image starts private and has to be made public by a maintainer, in the
browser (section 6).

## 4. Release notes

On GitHub, open **Releases**: the Release workflow has usually created a draft
for the tag by now; edit it (or **Draft a new release** and choose the tag if
there is none), title it `FleetML v0.1.0`, and paste the version's
`CHANGELOG.md` section as the notes, followed by:

```markdown
**Try it:** see the [quickstart](https://github.com/fleetml/fleetml#quickstart) or run `./scripts/demo.sh`.
```

Mark pre-1.0 releases as the latest release (not a pre-release) once they are
the recommended version. Before publishing, check that the release lists the
eight archives and `SHA256SUMS`: the install instructions download them from
the latest release.

If the Release workflow failed, or to attach binaries to a release made before
it existed, run it by hand: **Actions → Release → Run workflow** on `main`,
with the tag (`v0.1.0`). It builds that tag's source and replaces the assets
(`gh release upload --clobber`); the notes stay as they are. v0.4.0 was
released before the workflow existed: run it once by hand with `v0.4.0` to
back-fill its binaries (they report 0.4.0, the version in that tag's source).

## 5. Publish the Android SDK and the Python client

### Android SDK

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

### Python client

Every release publishes the Python client with the release's version: run the
**Python publish** workflow from the Actions tab (**Run workflow** on `main`)
with that version. The workflow checks that it equals `__version__` in
`python/src/fleetml/__init__.py`, builds the sdist and the wheel, runs
`twine check`, and publishes them to [PyPI](https://pypi.org/project/fleetml/)
as `fleetml`. A version on PyPI can never be replaced.

It uses PyPI's Trusted Publishing: no token is stored in the repository.
PyPI trusts the workflow through a publisher entry on the `fleetml` project
(owner `fleetml`, repository `fleetml`, workflow `python-publish.yml`,
environment `pypi`), and the workflow runs in the repository's `pypi`
environment. Before the first publication, that entry is created as a
pending publisher (PyPI account, **Publishing → Add a new pending
publisher**), which also reserves the name. The first version published to
PyPI was 0.6.0.

### iOS

Nothing has to be published for iOS: the Swift package's manifest is at the
repository root, so Swift Package Manager picks up each `vX.Y.Z` tag as a
version as soon as it is pushed.

## 6. The public repository's settings

The repository became public on 2026-10-23, after v0.6.0, with the CI
workflow green for `main` (**Settings → General → Danger Zone → Change
repository visibility**). These settings were made then and stay as they are:

- **Settings → Code security:** private vulnerability reporting (so the
  security policy's reports can also arrive there), the dependency graph,
  Dependabot alerts and Dependabot security updates, secret scanning and
  push protection.
- **Settings → Actions → General:** workflows on pull requests from forks
  wait for a maintainer's approval when the contributor is new to the
  repository, and the workflow token is read-only by default; a workflow
  that writes asks for it in its own `permissions` (the Release workflow).
- **Packages:** `ghcr.io/fleetml/fleetml-server` and
  `ghcr.io/fleetml/fleetml-dashboard` are public. A new package starts
  private: open it under the organisation's **Packages**, then **Package
  settings → Danger Zone → Change visibility → Public**. The organisation's
  own setting (**Settings → Packages**, package creation **Public**) had to
  be allowed first; without it the option is not offered.

## 7. The branch rule on `main`

In place since the repository became public (a private repository of a
free organisation has no branch protection or rulesets). The rule on `main`:

- a pull request before merging, with no required approvals;
- the CI checks `Go`, `CLI on Windows`, `Dashboard`, `Android SDK`,
  `Docs site` and `Helm chart` must pass; not `iOS SDK`, which runs only
  when `ios/` changes and would otherwise block other pull requests;
- no force pushes and no deletion of the branch;
- the maintainers' account `fleetml-maintainers` may bypass the rule (for
  example for the release commit and the changelog's next section).

Every other change to `main` goes through a pull request.

## 8. Announce

Post the announcement with the 60-second story (the README's recording of
`scripts/demo-agent.sh`) and a link to the release. Answer issues and
discussions from the project's maintainer account.

## After the release

Add a section for the next version at the top of `CHANGELOG.md`
(`## [0.4.0] - Unreleased`) with its compare link at the bottom.
