# AGENTS.md

An OpenTofu / Terraform provider for Discord guild infrastructure (`kirchdev/discord`), built on `terraform-plugin-framework`. The unit of work is a resource or data source in `internal/provider/`; the REST client is `internal/client/`.

## Commands

| Command                       | Note                                                                                                            |
| :---------------------------- | :-------------------------------------------------------------------------------------------------------------- |
| `pnpm check && go test ./...` | **The gate** — oxlint + oxfmt over JS/JSON/YAML/MD, then the Go unit tests. Run before every push.              |
| `make testacc`                | Mock acceptance tests against an in-memory Discord API — no token, but needs `tofu` or `terraform` on PATH.     |
| `make docs`                   | Regenerates `docs/` from the provider schema and `examples/` (tfplugindocs); needs `tofu`/`terraform` and `jq`. |
| `make vuln`                   | `govulncheck ./...` — reachable vulnerabilities, standard library included.                                     |

## Never

- **Never hand-edit `docs/`** — edit the schema descriptions in `internal/provider/` or the files in `examples/`, then run `make docs`. **Never hand-edit `CHANGELOG.md`** — release-please owns it.
- **Never use `terraform-plugin-sdk/v2`** — it is only an indirect dependency; resources use `terraform-plugin-framework`.
- **Never `--no-verify`** — commitlint (Conventional Commits) and lint-staged run on every commit.
- **Never add message-content resources** — the scope is guild infrastructure; there is no `discord_message`.

## Traps

- **Snowflake ids and permission bitfields are strings** — a 64-bit value loses precision as a JSON number; `role.color` is the one int. Bodies are flat JSON, not JSON:API.
- **Resources never handle `429`** — `internal/client/client.go` retries rate limits and transient `5xx` itself.
- **Some objects cannot be created or deleted by a bot** (`discord_managed_server`, `discord_role_everyone`, welcome screen, onboarding) — Create adopts via PATCH and Delete is a no-op; `discord_role_everyone.id` is the server id.
- **Role position has its own endpoint** — a `[{id, position}]` PATCH on the guild role collection, not the role-modify body.
- **Channel overwrites have one owner** — a channel's `permissions` attribute conflicts with `discord_channel_permission` and with `sync_perms_with_category = true`.
- **Deeply nested config and images are write-only** — nested config goes through a raw `*_json` attribute, images through `*_data_uri` (build them with `discord_local_image`); Discord returns only hashes, so neither is refreshed.
- **Raise `toolchain` in `go.mod` when a Go patch fixes the standard library** — CI and the release build with that line; keep it at or above the version the local `golangci-lint` was built with and one the CI `golangci-lint` (the `_ci-go.yml` default in `kirchDev/workflows`) supports.
- **README and meta files keep the kirchDev house style** — centered hero block, the section emojis `README.md` already uses, GitHub callouts, the MIT license footer; lint-staged skips `README.md` so oxfmt does not reflow it.
- **Only `feat` and `fix` reach a release** — release-please on `main` cuts the version, CHANGELOG and tag; goreleaser appends the signed archives.

## Where it is decided

- Contributor workflow — [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Exemplars for new entities — `internal/provider/role_resource.go` (CRUD), `text_channel_resource.go` with `channel_common.go` (channel kinds), `server_data_source.go` (API read), `color_data_source.go` (compute-only); permission keys in `permissions.go`
- Provider conventions shared across the kirchDev providers — the sibling `../terraform-provider-laravelforge`
