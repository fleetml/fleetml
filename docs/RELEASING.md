# Releasing

How a FleetML release is cut, in order. Releases are made by the maintainers
from `main`.

## Release plan up to the public launch

The repository stays private through v0.1.0 and v0.2.0. Those two versions
are tagged in the private repository, on the boundary commits recorded when
each version was closed, with steps 1 to 4 below. The visibility change
(steps 5 and 6) and the public launch (step 7) happen once, after v0.3.0 and
the launch material are done: branch protection is set right before the
repository is made public. From then on every release follows all the steps.

## 1. Check the release candidate

On an up-to-date `main` with a clean working tree:

```bash
make lint test                       # Go: golangci-lint, unit tests with -race
make test-integration                # Go: integration tests against the dev stack
make e2e                             # end-to-end suite against a fresh server
(cd dashboard && npm ci && npx tsc && npm run build)
(cd android && ./gradlew :fleetml-sdk:testDebugUnitTest)
make ios-test
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
user-visible change since the previous release. Bump the SDK versions if
they changed (`SDK_VERSION` in `android/fleetml-sdk/build.gradle.kts`,
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

## 5. Protect `main`

Once, right before the repository becomes public (after v0.3.0), in **Settings → Branches → Add
branch protection rule** (or a ruleset) for `main`:

- require a pull request before merging;
- require the CI checks to pass (`Go`, `Dashboard`, `Android SDK`); not
  `iOS SDK`, which runs only when `ios/` changes and would otherwise block other
  pull requests;
- block force pushes and deletion of the branch.

From then on every change to `main` goes through a pull request.

## 6. Make the repository public

Once, after v0.3.0 and the launch material are ready. In **Settings → General → Danger Zone → Change repository visibility**, make
it public. Then, in **Settings → Code security**, enable private
vulnerability reporting (so the security policy's reports can also arrive
there) and Dependabot alerts.

Check the public view: the README renders with its links, the Actions tab shows
the release commit green, and the release is listed.

## 7. Announce

Post the announcement with the 60-second story (the recording of
`scripts/demo.sh` from the README) and a link to the release. Answer issues and
discussions from the project's maintainer account.

## After the release

Add a new `## [Unreleased]` section at the top of `CHANGELOG.md` for the next
changes.
