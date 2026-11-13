# Architecture

## Request flow

```
Terraform CLI
     │ plugin protocol 6.0
     ▼
main.go ── providerserver.Serve ──► internal/provider
                                        │ schema, env fallback, Configure
                                        │ GET /api/runtime → Data
                                        ▼
                              internal/providerdata.Data
                                   │            │
                    ┌──────────────┘            └──────────────┐
                    ▼                                          ▼
          internal/resources/*                       internal/datasources/*
          15 kinds, one package each                 9 read-only lookups
                    │                                          │
                    └──────────► internal/client ◄──────────────┘
                                 HTTP, retries, error taxonomy
                                        │
                                        ▼
                              internal/configresource
                              Kind, Resource, id rules, JSON codec
                                        │
                                        ▼
                          agentgateway /api/config/resources
```

## Client

`internal/client.Client` is the only path to agentgateway. It speaks the
config resource API directly; there is no upstream SDK to wrap.

- **Endpoints** are `/api/runtime`, `/api/config/resources/{kind}` (collection)
  and `/api/config/resources/{kind}/{id}` (single resource), plus
  `/api/config/effective`.
- **Create vs update** are distinct operations. The collection `PUT` is an
  upsert by id and is rejected for policy kinds (C3); the single-resource `PUT`
  is update-only in `hybrid` mode for non-policy kinds (C4) and is the only way
  to create a policy. A resource never calls `Update` for a create.
- **Timeout** is a per-operation context from `providerdata.Data.Timeout`.
- **Retry** applies to every operation: the API's `GET`/`PUT`/`DELETE` are
  idempotent, so transient failures (429, 502, 503, 504, transport errors) are
  retried three times with 100ms/500ms/1s backoff. `400`, `403`, `409` and
  `422` are never retried.
- **Version probe** runs `GET /api/runtime` once during `Configure` and caches
  only a successful result. A failed probe is a warning, never a failure.
- **Redaction** — the client never logs request or response bodies. `tflog`
  carries kind, id, status and duration only. `agentgateway_effective_config`
  is marked `Sensitive` because the effective config contains secrets.
- **Collection `PUT` is not replace-all.** The server merges the prepared
  resources into the existing snapshot and never deletes omitted ids (C21). An
  acceptance test pins this: two providers, then a third, all three still
  present.

## Config resource model

`internal/configresource` owns everything that is the same for all fifteen
kinds:

- `Kind` — the fifteen supported kinds, with `IsPolicy` and `IsSingleton`.
- `Resource` — `{Kind, ID, Value, Revision, CreatedAt, UpdatedAt}`.
- `DeriveID` — the id rules from spec §7.0: `name` for the name-keyed kinds,
  `id` else `name` for `llm.model`, the server UUID for `llm.apiKey`, `default`
  for the three singletons and the policy key for the policy kinds.
- `StripManagedMetadata` / `HasReservedMetadataKey` — the `agentgateway.dev/`
  metadata the server owns on `llm.apiKey` (C5).
- `EncodeValue` / `DecodeValue` — the JSON codec. `null` and `[]` stay
  distinct: an empty list means "deny all" and is never collapsed into unset
  (C13).

## Resource packages

One package per kind. Each package owns only its schema and its value
mapping; everything else is shared:

- `internal/resourceutil.CRUD(kind, newModel)` provides Create, Read, Update,
  Delete and ImportState. It owns the timeouts, the file-mode singleton
  rejection, the pre-flight existence check, the create-vs-update path
  selection, 404 → remove from state, the conflict diagnostics, the import
  parsing and the computed `id`/`revision` attributes.
- `resourceutil.PolicySchema` / `PolicyModel` give the four policy resources
  one shared shape (`name` + JSON `spec`) instead of four copies.
- `resourceutil.FilterAndSort` gives the nine data sources one sorting and
  prefix-filtering implementation.

The model type of each package implements `resourceutil.KindResource`:
`Kind()`, `ID()`, `Value(ctx, diags)` and `SetValue(ctx, value, diags)`.
`SetValue` preserves attributes the server did not return.

## Provider configuration

`internal/provider/config.go` resolves attribute → environment → default,
infers nothing, and validates the cross-field rules before any request is made.

| Rule | Failure the provider catches early |
|---|---|
| `endpoint` must be an `http`/`https` URL with a host and no path | a path suffix such as `/ui` or `/api` |
| `token` and (`username` or `password`) are mutually exclusive | both auth modes at once |
| `password` requires `username` | basic auth without a user |
| basic auth over `http` | a password sent in clear text |
| `ca_cert` xor `ca_cert_file`, `client_cert` xor `client_cert_file`, `client_key` xor `client_key_file` | two sources for one certificate |
| `client_cert*` requires `client_key*` and vice versa | mTLS without a key |
| `timeout` must be a positive Go duration | `0` or `30` (no unit) |
| `storage_mode_check = error` with `configStoreMode = readOnly` | set `config.storage.mode` to `hybrid` (or `file`) and restart the gateway |

`Configure` never dials the data plane and never writes.

## Storage modes

| Mode | Behaviour |
|---|---|
| `hybrid` | The supported mode. Full CRUD for every kind, stable revisions and generation checks. |
| `file` | The gateway owns `config.yaml`. Name-keyed kinds work but every write rewrites the file (C25). `llm.settings`, `mcp.settings` and `modelCatalog` are rejected because the list response omits them (C24). API keys are readable only when Terraform created them. |
| `readOnly` | Every write returns `403 UI is configured as read-only`. `storage_mode_check = error` (the default) fails `Configure` before any write is attempted. |

## Error taxonomy

`internal/client` maps status codes to sentinels; resources translate the
classification into diagnostics:

| Status / condition | Sentinel | Resource behaviour |
|---|---|---|
| `404` | `ErrNotFound` | Read: remove from state. Delete: treat as success. Update: "resource no longer exists; refresh". |
| `400`, `422` | `ErrInvalid` | Attribute-level diagnostic when the message names a field, else resource-level. |
| `403` read-only | `ErrReadOnly` | Diagnostic naming `config.storage.mode`. |
| `409` generation conflict | `ErrConflict` | One re-read and retry, then a diagnostic. |
| `409` file-owned / missing apiKey policy / name conflict | `ErrConflict` | Diagnostic naming the prerequisite or the conflicting owner. |
| `429`, `502`, `503`, `504`, transport error | `ErrTransient` | Retry three times with exponential backoff. |
| `5xx` | `ErrServer` | Operation-scoped error with the server message. |
| id exists but not in state | – (local) | `{kind}/{id} already exists outside Terraform state; import it with terraform import <address> {kind}/{id}`. |
| JSON decode failure | `ErrServer` | Diagnostic that names the kind and id, never the body. |

## Test layers

| Layer | What it proves |
|---|---|
| L1 — pure unit | id derivation, import parsing, JSON build/split, policy-name validation, error classification, metadata stripping |
| L2 — resource logic | CRUD against a scripted fake `Client` (create-vs-update path, 404 removal, rename, preserved attributes) |
| L3 — protocol | `internal/client` against `internal/agentgatewaytest`, a real `httptest` server implementing the hybrid semantics |
| L4 — acceptance | real agentgateway in Docker, `TF_ACC=1` |

Unit tests live in the same package as the code they cover. Acceptance tests
live next to them and must compile with `go vet ./...` even when they are not
run locally.

The acceptance environment runs three gateway services, because one hybrid
instance cannot produce every negative path: `hybrid` (port 15000) for the
happy path, `readonly` (15001) for `403`, and `fileowned` (15002) for the
`409 file-owned` settings conflict. `AGENTGATEWAY_EXPECT_VERSION` makes
`PreCheck` fail when the gateway is not the release under test.

## Dependency rules

1. `internal/client` imports only stdlib and `internal/configresource`.
2. `internal/configresource` imports only stdlib and framework types.
3. `internal/providerdata` imports `internal/client` and framework `diag`.
4. `internal/resources/*` and `internal/datasources/*` import `client`,
   `configresource`, `providerdata`, `resourceutil` and their local models only.
5. Nothing under `internal/resources` imports `internal/provider`.
6. `internal/provider` imports every resource/data-source package and is the
   only place registration lives.
