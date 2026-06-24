# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

A first-party **OpenTofu / Terraform provider for [Discord](https://discord.com)**, owned by kirchDev. It manages Discord *guild infrastructure* (roles, channels, permission overwrites, members, webhooks, events, moderation, …) as code so a Discord estate can live in the same IaC workflow as the rest of the kirchDev infrastructure.

- **Provider type (HCL):** `discord` → `provider "discord" {}`
- **OpenTofu registry address:** `kirchdev/discord`
- **Go module:** `github.com/kirchDev/terraform-provider-discord`
- **SDK:** `terraform-plugin-framework` (NOT the legacy SDKv2)

The repo name format `NAMESPACE/terraform-provider-NAME` is **mandatory** for the OpenTofu/Terraform registry — it's not a style choice.

**Scope is guild infrastructure, not message content** — there is no `discord_message` resource. The original scoping notes live in `temp_ai.md` (gitignored).

## Current state

Two layers coexist:

- **Node meta layer** (from the `scaffold` template): pnpm + oxlint + oxfmt + husky + commitlint + release-please + CI/CodeQL/Dependabot + issue/PR templates. Gates config/docs (JSON/YAML/MD).
- **Go provider**: `go.mod` (`terraform-plugin-framework`), `main.go`, `internal/client/` (a flat-JSON Discord REST client: `Get`/`List`/`Write`/`Delete`, rate-limit aware), and `internal/provider/` with **~24 resources + 9 data sources** covering the guild-management surface (guild settings, roles, every channel kind, permission overwrites, members, invites, webhooks, threads, emojis, bans, scheduled events, auto-moderation, welcome screen, onboarding, widget, application commands; plus the `discord_permission` / `discord_color` / `discord_local_image` helper data sources and read-only lookups).

## API shape (Discord REST v10)

- Base `https://discord.com/api/v10`. Auth `Authorization: Bot <token>`. A descriptive `User-Agent` is required.
- **Bodies are FLAT JSON** (Discord is not JSON:API). A single read returns the object; a collection returns a JSON array. The id is the string snowflake in `id`.
- **Snowflake ids and permission bitfields are strings.** A 64-bit permission field would lose precision as a JSON number, so `role.permissions`, `role_everyone.permissions` and channel-overwrite `allow`/`deny` are modelled as decimal strings. `role.color` is an int.
- **Rate limits are real.** `internal/client/client.go` transparently honours `429` (`retry_after` from the body, falling back to the header) and retries transient `5xx` with backoff, so resources never see a `429`.
- The `discord_permission` data source is the ergonomics linchpin: named permission keys → `allow_bits`/`deny_bits`. The full key list + their bit shifts live in `internal/provider/permissions.go`.

## Commands

Go (via `GNUmakefile`; needs **Go ≥ 1.25** — `terraform-plugin-framework v1.19` requires it):

| Command         | What it does                                                                  |
| :-------------- | :---------------------------------------------------------------------------- |
| `make build`    | `go build -o terraform-provider-discord`                                      |
| `make tidy`     | `go mod tidy`                                                                 |
| `make fmt`      | `gofmt -s -w .`                                                               |
| `make vet`      | `go vet ./...`                                                                |
| `make docs`     | render `docs/` from the schema (build + export + tfplugindocs)               |
| `make test`     | `go test ./...`                                                               |
| `make testacc`  | `TF_ACC=1 go test ./...` — mock acceptance tests; needs a TF binary, no token |

Node meta layer: `pnpm install` (wires husky hooks), `pnpm check` / `pnpm check:fix`. CI (`.github/workflows/ci.yml`) runs a **Go job** (build·vet·gofmt·test + `TF_ACC` mock acceptance tests, OpenTofu installed) and a **Lint job** (oxlint + oxfmt).

> [!NOTE]
> Generated files are excluded from oxfmt via `.prettierignore` (`docs/` from tfplugindocs, `CHANGELOG.md` from release-please).

## Patterns & gotchas

- `internal/client/client.go` — flat-JSON REST client: `Get`, `List` (returns `[]json.RawMessage`), `Write(method, path, body, &out)`, `Delete`, `client.NotFound(err)`, `APIError`.
- `internal/provider/helpers.go` — `notFound(err)` (covers a real 404 **and** a list-scan miss), `findInList` (read an item out of a collection where Discord has no clean single-item read, e.g. a role within a guild), `strSet`/`setOfStrings`/`strPtrOrNil`.
- Exemplars new entities follow: `role_resource.go` (CRUD), `text_channel_resource.go` (channel kinds — share `channel_common.go`), `server_data_source.go` (API read), `color_data_source.go` (compute-only), `permission_data_source.go`.
- **Manage-not-create resources** (`discord_managed_server`, `discord_role_everyone`, singletons like welcome screen / onboarding): a bot can't create/delete these, so Create *adopts* via PATCH and Delete is a no-op. Import-first; pair `discord_managed_server` with `prevent_destroy`.
- **`discord_role_everyone.id == server_id`** (the @everyone role id is the guild id).
- **Role position** has its own endpoint (modify-role-positions, a `[{id, position}]` PATCH on the guild role collection) — not the role-modify body.
- **`sync_perms_with_category`** copies the parent category's overwrites onto the channel; it conflicts with explicit `discord_channel_permission` overwrites on the same channel (own them one way or the other).
- **`discord_member_roles`** is authoritative (owns the member's full role set minus `@everyone`); it reads the live `roles` array back on every Read for accurate drift detection.
- **Deeply-nested config** (`discord_server_onboarding` prompts, `discord_application_command` options) is passed through as raw JSON in a `*_json` attribute — write-only, not refreshed.
- **Images** (guild icon/splash, emoji, webhook avatar) are base64 **data URIs** via `*_data_uri` attributes — build them with the `discord_local_image` data source. They're write-only; Discord returns only hashes (`icon_hash`, …).

## Release & publishing

- **release-please** (`release-type: go`) owns versioning + CHANGELOG + tag + GitHub release, running on `main` under a GitHub App token (Bitwarden-stored PEM).
- When it cuts a release, a second job in the same workflow runs **goreleaser**: builds the cross-platform archives, **GPG-signs** the checksums (key + passphrase from Bitwarden SM), and **appends** them to the release (`release.mode: append`).
- The registry consumes the per-platform zips + `SHA256SUMS` + detached `.sig` + `manifest.json` (protocol `6.0`).

## Conventions

- **Conventional Commits enforced** via commitlint. Don't `--no-verify` unless explicitly asked.
- **House style** for READMEs/meta files: centered hero block, prescribed section emojis (✨ 📦 🚀 🤝 🛣️ 📄), GitHub callouts (`> [!TIP]`), license footer `[MIT](LICENSE) © [Titus Kirch](https://github.com/TitusKirch/) / [IT-Dienstleistungen Titus Kirch](https://kirch.dev)`.
- Sibling **`../terraform-provider-laravelforge`** is the reference implementation of these provider conventions — check it when unsure.
