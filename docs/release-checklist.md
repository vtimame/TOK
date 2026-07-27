# Release notes and validation checklist

## Release truthfulness rule

- The checklist is a draft only until all required checks are complete.
- Never mark a check as passed unless its command output or CI evidence explicitly shows success.
- If a check was not run, leave it as `not run`, `N/A`, or `pending`.

## Draft release process template

Use this file for each release candidate. Keep it in sync with executed validation.

- Release version:
- Release candidate:
- Release commit/ref:
- GitHub CI workflow URL:
- CI status (`pass`/`fail`/`pending`):
- Prepared by:
- Date:

### Required checks (must be completed before publishing)

1. **Release commit and CI**
   - Record release commit/ref and the exact CI run URL tied to that commit.
   - CI status must be explicitly captured as `pass`, `fail`, or `pending`.

2. **Backend validation**
   - `make test`
   - `go test ./...` (if run outside Makefile)

3. **File-budget / drift checks**
   - `make quality`
   - `./scripts/check-file-budgets.sh`
   - `cd web && pnpm api:check`

4. **Web checks**
   - `cd web && pnpm lint`
   - `cd web && pnpm test`
   - `cd web && pnpm build`

5. **Release artifact checksums**
   - If release artifacts are generated locally, verify checksums for the exact files you will publish.
   - Example command:
     - `sha256sum <artifact-1> <artifact-2> ...`
   - Record the generated digest output.

Do not move a release draft to a published artifact until every required check is
captured as passed with evidence.

## v0.2.1 draft (not published)

### Candidate v0.2.1-rc.1 (Draft)

- Release commit/ref: `pending commit`
- Local candidate base: `main` at `392d5d75570098a8d0f80464b19a6495441ca0f8` plus uncommitted release candidate changes.
- CI workflow URL: `pending commit/push`
- CI status: `pending`
- Prepared by: `Codex`
- Date: `2026-07-27`

#### Executed validation

- Backend tests:
  - Command: `make test`
  - Result: `passed locally on 2026-07-27`
- OpenAPI + generated client drift:
  - Command: `cd web && pnpm api:check`
  - Result: `passed locally on 2026-07-27`
- Web lint/test/build:
  - Command: `cd web && pnpm lint`
  - Result: `passed locally on 2026-07-27`
  - Command: `cd web && pnpm test`
  - Result: `passed locally on 2026-07-27`
  - Command: `cd web && pnpm build`
  - Result: `passed locally on 2026-07-27`
- File-budget:
  - Command: `make quality`
  - Result: `passed locally on 2026-07-27`
  - Command: `./scripts/check-file-budgets.sh`
  - Result: `passed locally on 2026-07-27 as part of make quality`
- Workflow/docs checks:
  - Command: `python3 YAML parse for .github/workflows/ci.yml and .github/workflows/release.yml`
  - Result: `passed locally on 2026-07-27`
  - Command: `cd web && pnpm exec prettier --check ../.github/workflows/ci.yml ../.github/workflows/release.yml ../docs/release-checklist.md ../docs/install.md ../README.md`
  - Result: `passed locally on 2026-07-27`
  - Command: `git diff --check`
  - Result: `passed locally on 2026-07-27`
- Checksums:
  - Command: `local SHA256SUMS dry-run using sha256sum and install-doc grep verification`
  - Result: `passed locally on 2026-07-27`

### Draft release notes (must not claim unchecked items)

```text
## TOK v0.2.1

- [ ] Release commit/ref recorded above.
- [ ] Full CI on release commit: pending commit/push.
- [x] Backend tests: `make test` passed locally on 2026-07-27.
- [x] OpenAPI/client drift check: `cd web && pnpm api:check` passed locally on 2026-07-27.
- [x] Web lint/test/build: `pnpm lint`, `pnpm test`, and `pnpm build` passed locally on 2026-07-27.
- [x] Quality/file-budget checks: `make quality` passed locally on 2026-07-27.
- [x] Checksum workflow/docs dry-run: local SHA256SUMS verification passed on 2026-07-27.
```

- Keep this section editable until all checks are complete.
- Promote to a published release note only after converting all `TODO`/`pending`/`not run` entries to verified pass values.
