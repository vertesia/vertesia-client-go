# Agent Instructions

This repository contains the Go client for the Vertesia API. It combines a
small hand-written facade with a committed OpenAPI-generated client.

## Repository Layout

- `facade.go` is the hand-written high-level `Client` facade. Prefer making
  public client behavior changes here.
- `doc.go` contains package-level Go documentation for the public facade.
- `openapi/` is generated OpenAPI client code.
- `spec/vertesia-openapi.json` and `spec/vertesia-openapi.yaml` are the tracked
  OpenAPI contract used by generation.
- `openapi-generator-config.yaml` configures OpenAPI Generator for the Go
  client.
- `scripts/patch-openapi-permissive-decode.sh` applies required post-generation
  compatibility and security patches to generated output.
- `*_test.go` files contain Go tests. `client_test.go` includes opt-in live
  integration tests.
- `.github/workflows/` contains test, release, generated-code guard, and CodeQL
  workflow security checks.

## What You May Modify

Normal hand-written changes may touch:

- `facade.go`
- `doc.go`
- `*_test.go`
- `README.md`, `AGENTS.md`, `CLAUDE.md`, and other docs
- `go.mod` and `go.sum` for dependency or Go version updates
- `openapi-generator-config.yaml` when changing generator configuration
- `scripts/`, especially post-generation compatibility patches
- `.github/` workflow and repository automation files

When changing public behavior, update or add Go tests and keep README examples
accurate.

## What You Must Not Modify Manually

Do not manually edit or commit files under:

- `openapi/`
- `spec/`

Those files are owned by internal generation automation. CI rejects pull
requests that change either directory. If generated output is wrong, change the
OpenAPI source upstream, generator configuration, or post-generation patching
logic, then let automation regenerate the client.

If a user explicitly asks for an urgent generated-code fix, also update
`scripts/patch-openapi-permissive-decode.sh` so the change is reproducible after
regeneration, and call out that the generated-code guard may reject normal PRs.

Do not commit local secrets or credentials:

- `.env` is for local development only.
- Live test credentials such as `VERTESIA_API_KEY` must never appear in tracked
  files, logs, fixtures, or examples.

## Generation Rules

The committed generated client exists so consumers can install releases with
normal Go module versioning.

Regeneration is not part of normal feature work. If regeneration is explicitly
needed, refresh the tracked spec files, run OpenAPI Generator with
`openapi-generator-config.yaml`, then run:

```sh
scripts/patch-openapi-permissive-decode.sh openapi
gofmt -w openapi
go test ./...
```

Generated changes should be staged only by the dedicated generation automation,
using the expected generated paths and module metadata:

```sh
git add openapi spec go.mod go.sum
```

The post-generation patch script currently preserves forward-compatible response
decoding and ensures generated debug logging does not dump sensitive request or
response data.

## Testing And Checks

For a normal local check:

```sh
go test ./...
go vet ./...
```

In restricted sandboxes, the default Go build cache may be unwritable. Use a
writable cache path if needed:

```sh
GOCACHE=/tmp/vertesia-client-go-gocache go test ./...
GOCACHE=/tmp/vertesia-client-go-gocache go vet ./...
```

Some tests use `httptest` loopback listeners. If a sandbox blocks local port
binding, state that clearly and rerun only with appropriate approval.

Live integration tests are skipped unless both conditions are true:

- `VERTESIA_LIVE_TESTS=1`
- `VERTESIA_API_KEY` is set to a non-placeholder `sk-` secret key

Live tests can create and delete Vertesia resources. Do not enable them unless
the user explicitly asks and provides an appropriate environment.

## Go Style

- Support the Go version declared in `go.mod`.
- Run `gofmt` on changed Go files.
- Prefer the standard library for facade logic unless a dependency is already
  part of the package contract.
- Avoid broad dependency additions for a small facade feature.
- Keep generated code changes out of normal commits.
- Keep exported names, comments, and examples coherent with `go doc` output.

## Client Design Notes

- `NewClient` is the recommended user entry point.
- The facade routes Studio and Store APIs through generated API groups.
- `APIKey` performs STS token exchange and requires an `sk-` secret key.
- `Token` uses an existing bearer token and bypasses STS.
- `APIKey` and `Token` are mutually exclusive.
- Custom split endpoints using `APIKey` require `TokenServerURL` unless STS can
  be safely derived from a Vertesia `api*` host.
- The default `x-api-version` header is part of the client contract; update it
  deliberately and test header behavior.
- The raw generated client remains available from the `openapi` package and
  through `Client.Studio` and `Client.Store`.
- Generated response decoding is patched for forward compatibility with new
  server fields.
- Debug logging must not print bearer tokens, API keys, cookies, query secrets,
  request bodies, or response bodies.

## Release And Version Rules

Releases are created from git tags. For release version changes, keep these
values in sync:

- `openapi-generator-config.yaml` `packageVersion`
- `spec/vertesia-openapi.json` `info.version`

`version_test.go` and the release workflow verify this relationship. Since
`spec/` is generated-owned, coordinate version changes with the generation
automation instead of editing spec files manually in a normal PR.

## GitHub Actions Security

Workflows are audited by `zizmor` and CodeQL.

- Pin third-party GitHub Actions by full commit SHA.
- Keep a trailing comment with the exact corresponding tag, for example
  `# v4.35.5`.
- For annotated tags, pin the peeled commit SHA, not the tag object SHA.
- Keep `permissions` minimal and prefer `permissions: {}` at workflow scope.
- Use `persist-credentials: false` for checkout unless a workflow explicitly
  needs push credentials.
- Move GitHub expression values used in shell scripts into `env:` variables
  before referencing them from `run:` blocks.
- After workflow changes, run:

```sh
zizmor .github/workflows
```

## Dependency Updates

Runtime dependencies live in `go.mod` and checksums live in `go.sum`.

- Use `go get <module>@<version>` for dependency updates.
- Run `go mod tidy` when dependency graph changes.
- Run `go test ./...` and `go vet ./...` after dependency updates.
- Avoid broad major version changes unless the user asks for that risk.

Dependabot also manages GitHub Actions updates. When updating workflow action
pins, resolve and pin the actual commit for the desired tag and update the tag
comment at the same time.

## Agent Workflow

- Check `git status --short --branch` before editing.
- Keep generated-code changes out of normal commits.
- Preserve unrelated user changes in the worktree.
- Prefer small, focused commits and describe verification performed.
- If you cannot run a relevant check because a tool, dependency, network access,
  or sandbox permission is missing, state that clearly.
