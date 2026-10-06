# Maintaining wspace

Notes for maintainers. Contributors do not need any of this; see
[CONTRIBUTING.md](../CONTRIBUTING.md).

## Repository settings

Already applied while the repository is private:

- Squash merge only; the squash title is the pull request title and the body
  keeps the commit messages, so `Signed-off-by` trailers survive.
- Head branches are deleted after merge; "Update branch" is offered.
- Wiki and Projects are disabled.
- The default `GITHUB_TOKEN` is read-only and Actions cannot approve pull
  requests.

Branch protection, fork workflow approval and private vulnerability
reporting are not available for private repositories on the organization's
current plan, so they are applied when the repository goes public.

## Going public checklist

1. Send a test message to conduct@kivoradigital.com (the
   [Code of Conduct](../CODE_OF_CONDUCT.md) contact) and confirm it arrives.
2. Make sure every maintainer has two-factor authentication, then require it
   for the organization (**Organization settings → Authentication
   security**).
3. Run a final sweep for secrets (`gitleaks git --log-opts=--all .`),
   private data and paid-app references.
4. Make the repository public:

   ```sh
   gh repo edit kivoradigital/wspace --visibility public --accept-visibility-change-consequences
   ```

5. Protect `main` and the release tags with the rulesets in
   [rulesets/](rulesets):

   ```sh
   gh api -X POST repos/kivoradigital/wspace/rulesets --input .github/rulesets/main.json
   gh api -X POST repos/kivoradigital/wspace/rulesets --input .github/rulesets/release-tags.json
   ```

6. Require approval before running workflows from every external
   contributor:

   ```sh
   gh api -X PUT repos/kivoradigital/wspace/actions/permissions/fork-pr-contributor-approval \
     -f approval_policy=all_external_contributors
   ```

7. Enable private vulnerability reporting (used by SECURITY.md):

   ```sh
   gh api -X PUT repos/kivoradigital/wspace/private-vulnerability-reporting
   ```

8. In **Settings → Code security**, enable secret scanning with push
   protection and Dependabot alerts.
9. Open a test pull request from a fork and confirm that CI waits for
   approval, the DCO check runs, and the merge button is blocked until every
   required check passes.

## Rulesets

`main.json` protects the default branch: no deletion, no force push, linear
history, changes only through pull requests (squash), resolved review
threads, and these required checks: `test (ubuntu-latest)`,
`test (macos-latest)`, `test (windows-latest)`, `lint` and `DCO`. If a job
name changes in a workflow, update the ruleset too, or pull requests will
wait forever for a check that no longer exists.

`release-tags.json` lets only organization admins create, move or delete
`v*` tags.

Organization admins can bypass both rulesets for emergencies. Use that
sparingly; normal work goes through pull requests like everyone else's.

## Commit identity

Commit with your GitHub noreply address (`<id>+<login>@users.noreply.github.com`)
so personal email addresses never appear in the public history, and sign off
every commit (`git commit -s`) like any contributor.

## Releases

The Chocolatey package needs the `choco` binary, so the release job that runs
GoReleaser must run on Windows (or skip Chocolatey). Check the configuration
with `goreleaser check` and a local dry run with
`goreleaser release --snapshot --clean --skip=publish`.
