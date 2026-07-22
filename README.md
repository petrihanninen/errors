# errors

Automated production error triage and fixing for the duunitori5 Django backend.

A single Go binary with three modes:

- `fetch` — pulls unresolved error groups from New Relic (last 24 hours) into Postgres
- `fix` — clones the duunitori repo, runs the `claude` CLI on each todo error, and pushes a fix branch per error
- `serve` — web UI for browsing errors and fix attempts

The schema (`internal/db/schema.sql`) is applied automatically on every run, so no separate migration step is needed.

## How it runs

Two scheduled GitHub Actions workflows (previously two Kubernetes CronJobs):

- `.github/workflows/fetch.yml` — daily at 06:00 UTC, runs `go run ./cmd/errors fetch`
- `.github/workflows/fix.yml` — daily at 07:00 UTC, installs the `claude` CLI and runs `go run ./cmd/errors fix`

Both can also be triggered manually via workflow_dispatch. The database is a
[Neon](https://neon.tech) Postgres instance.

## Required repo secrets

| Secret | Purpose |
| --- | --- |
| `DATABASE_URL` | Neon Postgres connection string |
| `NEW_RELIC_ACCOUNT_ID` | New Relic account ID (fetch) |
| `NEW_RELIC_API_KEY` | New Relic API key (fetch) |
| `DUUNITORI_PUSH_TOKEN` | PAT with push access to the duunitori repo (mapped to the `GITHUB_TOKEN` env var; GitHub reserves the secret name `GITHUB_TOKEN`) |
| `DUUNITORI_REPO` | HTTPS URL of the duunitori repo to fix |
| `DUUNITORI_BASE_BRANCH` | Base branch to branch fixes from |
| `CLAUDE_CODE_OAUTH_TOKEN` | Auth for the `claude` CLI, created with `claude setup-token` |

## Serve mode (local only)

The web UI is not hosted anywhere; run it locally when needed:

```bash
DATABASE_URL=... go run ./cmd/errors serve
```

The Dockerfile is kept for running serve mode (or the other modes) in a container locally:

```bash
docker build -t errors .
docker run --rm -p 8080:8080 -e DATABASE_URL=... errors serve
```
