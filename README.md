# Vertesia Go Client

Go SDK for the Vertesia API.

```sh
go get github.com/vertesia/vertesia-client-go
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    vertesia "github.com/vertesia/vertesia-client-go"
)

func main() {
    client, err := vertesia.NewClient(vertesia.ClientOptions{
        APIKey: "sk-...",
    })
    if err != nil {
        panic(err)
    }

    account, _, err := client.AccountsAPI.GetCurrentAccount(context.Background()).Execute()
    if err != nil {
        panic(err)
    }
    fmt.Println(account.GetName())
}
```

`NewClient` is the recommended entry point. It configures the generated clients,
routes Studio and Store APIs, exchanges secret keys for short-lived tokens, and
sets the stable `x-api-version` header.

## Authentication

Use an `sk-` secret key when you want the SDK to exchange credentials through
STS:

```go
client, err := vertesia.NewClient(vertesia.ClientOptions{
    APIKey: "sk-...",
})
```

Use `Token` when you already have a bearer token:

```go
client, err := vertesia.NewClient(vertesia.ClientOptions{
    Token: "eyJ...",
})
```

Set either `APIKey` or `Token`, not both.

## Endpoints

By default the client uses the unified global API:

```go
client, err := vertesia.NewClient(vertesia.ClientOptions{})
```

Use `Region` for a hosted regional API:

```go
client, err := vertesia.NewClient(vertesia.ClientOptions{
    Region: "us1",
    APIKey:  "sk-...",
})
```

Use `Preview` when you need the hosted preview API:

```go
client, err := vertesia.NewClient(vertesia.ClientOptions{
    Region:  "us1",
    Preview: true,
    APIKey:  "sk-...",
})
```

For public SDK usage, prefer the default global API or a hosted Vertesia region.
Studio and Store requests are routed through the same public API host.

`Site` is available as an advanced override when you need to provide the exact
Vertesia API host:

```go
client, err := vertesia.NewClient(vertesia.ClientOptions{
    Site:   "api.us1.vertesia.io",
    APIKey: "sk-...",
})
```

When using `APIKey` with custom split `ServerURL` and `StoreURL` endpoints, set
`TokenServerURL` unless STS can be inferred from a Vertesia `api*` host. Direct
`Token` authentication does not require STS.

The facade exposes both generated clients:

```go
client.Studio.ProjectsAPI
client.Store.ObjectsAPI
```

It also exposes convenience aliases such as `client.AccountsAPI` and
`client.ObjectsAPI` where routing is unambiguous.

## Raw Generated Client

The OpenAPI generated client remains available for advanced use:

```go
import "github.com/vertesia/vertesia-client-go/openapi"

cfg := openapi.NewConfiguration()
cfg.Servers = openapi.ServerConfigurations{{
    URL: "https://api.us1.vertesia.io/api/v1",
}}
cfg.AddDefaultHeader("x-api-version", "20260319")

raw := openapi.NewAPIClient(cfg)
ctx := context.WithValue(context.Background(), openapi.ContextAccessToken, "YOUR_TOKEN")
account, _, err := raw.AccountsAPI.GetCurrentAccount(ctx).Execute()
```

## Testing

Unit tests run without credentials:

```sh
go test ./...
```

Live integration tests are opt-in. They run only when `VERTESIA_LIVE_TESTS=1`
and `VERTESIA_API_KEY` is set to a non-placeholder `sk-` secret key. For
Vertesia developers running the SDK tests locally:

```sh
cp .env.example .env
# Edit .env and set VERTESIA_LIVE_TESTS=1 plus VERTESIA_API_KEY=sk-...
go test ./...
```

Without `VERTESIA_LIVE_TESTS=1`, live integration tests are skipped even when a
local `.env` file exists.

The `.env` file is for local development only and should not be committed.

## Generation

This repository is generated from the Vertesia OpenAPI specification. Generated
code is committed so Go consumers can use normal module versioning:

```sh
go get github.com/vertesia/vertesia-client-go@vX.Y.Z
```

The public OpenAPI contract for the committed SDK is tracked at
`spec/vertesia-openapi.json` and `spec/vertesia-openapi.yaml`. The JSON file is
the source used to generate the committed client source; the YAML file is
included for tools and readers that prefer YAML.

To regenerate locally, refresh the tracked spec files, then run OpenAPI
Generator with `openapi-generator-config.yaml`. The generator reads
`spec/vertesia-openapi.json` and writes directly to `./openapi`. After
generation, run `scripts/patch-openapi-permissive-decode.sh` so generated
response decoding stays forward-compatible with new server fields. Generated
files should not be moved manually.

Generated files under `openapi/` and `spec/` are owned by internal generation
automation. Do not edit or commit them manually. Pull requests that change either
directory are rejected by CI.

Hand-written SDK surface changes, such as `NewClient`, tests, examples, README,
workflows, and generator configuration, should go through normal pull request
review.

The generation automation should regenerate the client, run tests, and push the
generated diff directly to `main` using a dedicated bot or GitHub App. The sync
job should run `scripts/patch-openapi-permissive-decode.sh` after OpenAPI
Generator and before tests. It should stage only the expected generated files and
module metadata:

```sh
git add openapi spec go.mod go.sum
```

## Release

Releases are created from git tags. The release workflow runs tests against the
preview environment, verifies the tag matches the OpenAPI spec `info.version`
and generator `packageVersion`, creates an annotated tag, and publishes a GitHub
Release.
