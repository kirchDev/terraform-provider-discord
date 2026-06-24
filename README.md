<div align="center">

# 💬 terraform-provider-discord

**Manage your Discord guild infrastructure as code — roles, channels, permissions, members, webhooks, events and moderation, reconciled by OpenTofu**

[![Release](https://img.shields.io/github/v/release/kirchDev/terraform-provider-discord?style=flat-square&label=release&color=5865F2)](https://github.com/kirchDev/terraform-provider-discord/releases/latest)
[![OpenTofu Registry](https://img.shields.io/badge/opentofu-kirchdev%2Fdiscord-FFDA18?style=flat-square&logo=opentofu&logoColor=black)](https://search.opentofu.org/provider/kirchdev/discord/latest)
[![Terraform Registry](https://img.shields.io/badge/terraform-kirchdev%2Fdiscord-7b42bc?style=flat-square&logo=terraform&logoColor=white)](https://registry.terraform.io/providers/kirchDev/discord/latest)
[![Tests](https://img.shields.io/github/actions/workflow/status/kirchDev/terraform-provider-discord/ci.yml?branch=main&style=flat-square&label=tests)](https://github.com/kirchDev/terraform-provider-discord/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/github/license/kirchDev/terraform-provider-discord?style=flat-square&color=5865F2)](LICENSE)

</div>

---

```hcl
resource "discord_role" "moderators" {
  server_id   = "123456789012345678"
  name        = "Moderators"
  color       = data.discord_color.blurple.dec
  permissions = data.discord_permission.mod.allow_bits
  hoist       = true
}
```

Roles, channels, permission overwrites, members, webhooks and events declared in HCL and reconciled by OpenTofu — not clicked together in the Discord UI. One bot token manages every guild the bot is in. **Scope is guild infrastructure, not message content.**

> [!IMPORTANT]
> **Pre-1.0 / beta.** Built from scratch against the Discord REST API (v10) with `terraform-plugin-framework`. The client honours Discord's rate limits; the schema and behaviour may still change — pin an exact version and test before relying on it.

## 📦 Install & run

```hcl
terraform {
  required_providers {
    discord = {
      source  = "kirchdev/discord"
      version = "~> 0.1"
    }
  }
}

provider "discord" {
  token = var.discord_token # or set DISCORD_TOKEN
}

resource "discord_text_channel" "general" {
  server_id = "123456789012345678"
  name      = "general"
  topic     = "Welcome!"
}
```

```bash
export DISCORD_TOKEN="your-bot-token"   # Discord Developer Portal → Bot → Token
tofu plan
```

The bot must be a member of every guild you manage, with the permissions for what you change (`Manage Roles`, `Manage Channels`, …).

## ✨ Features

- **💬 Discord as code** — roles, channels, permission overwrites, members, webhooks, invites, events and moderation in HCL.
- **🧩 Broad API coverage** — ~24 resources + 9 data sources across the guild-management surface.
- **🔐 Ergonomic permissions** — the `discord_permission` data source turns named permission keys into the decimal bitfields Discord wants.
- **🚦 Rate-limit aware** — the client transparently honours `429` `retry_after` and retries transient errors.
- **🚀 OpenTofu & Terraform** — published as `kirchdev/discord` on both registries.
- **⚡ Modern stack** — `terraform-plugin-framework`; docs generated from the schema.

## 🗺️ Coverage

Scope is **guild infrastructure, not message content** (no `discord_message`). Snowflake ids and permission bitfields are modelled as strings to preserve 64-bit precision.

<details>
<summary>Full coverage</summary>

- **Guild** — `discord_managed_server` (manage an existing guild, import-first), `discord_role`, `discord_role_everyone`, `discord_emoji`, `discord_guild_ban`, `discord_guild_widget`, `discord_welcome_screen`, `discord_server_onboarding`, `discord_scheduled_event`, `discord_auto_moderation_rule`.
- **Channels** — `discord_category_channel`, `discord_text_channel`, `discord_voice_channel`, `discord_news_channel`, `discord_stage_channel`, `discord_forum_channel` (with tags), `discord_media_channel`, `discord_thread`, `discord_channel_permission`, `discord_webhook`, `discord_invite`.
- **Members** — `discord_member_roles` (authoritative), `discord_member_nickname`.
- **Application** — `discord_application_command` (global or guild).
- **Data sources** — `discord_permission`, `discord_color`, `discord_local_image`, `discord_server`, `discord_role`, `discord_member`, `discord_user`, `discord_channel`, `discord_invite`.

</details>

## 📚 Documentation

Per-resource docs live under [`docs/`](docs/), generated from the schema with `make docs` (build + export schema + tfplugindocs).

## 🤝 Contributing

PRs welcome. Conventional Commits required (enforced via commitlint). Husky runs the linters/formatters on `git commit`.

> [!TIP]
> Run `make build && go vet ./...` before pushing — CI will catch what husky missed.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full workflow.

## 🛣️ Versioning

[Semantic Versioning](https://semver.org/) via [release-please](https://github.com/googleapis/release-please) — see [CHANGELOG.md](CHANGELOG.md).

## 📄 License

[MIT](LICENSE) © [Titus Kirch](https://github.com/TitusKirch/) / [IT-Dienstleistungen Titus Kirch](https://kirch.dev)
