# Security Policy

## Scope

`terraform-provider-discord` is an OpenTofu / Terraform provider that talks to the Discord REST API on your behalf. Because it handles a **Discord bot token** — which can manage every guild the bot belongs to — security issues here can have direct impact on production communities.

While the provider is **pre-1.0**, the supported "version" is the **latest release** (and the tip of `main`). Once it stabilises, supported versions will be listed here.

## Reporting a Vulnerability

**Please do not file a public GitHub issue for security problems.**

In the context of this provider, a "vulnerability" typically means:

- Leaking the Discord bot token (e.g. into logs, state, or error messages beyond what's unavoidable).
- Insecure handling of credentials or secrets in transit.
- A resource that fails to validate input and lets an attacker reach unintended Discord API calls.
- A dependency that introduces a known CVE.

Use one of the following private channels:

1. **GitHub Private Vulnerability Reporting** (preferred): open a private advisory at <https://github.com/kirchDev/terraform-provider-discord/security/advisories/new>.
2. **Email**: [titus.kirch@kirch.dev](mailto:titus.kirch@kirch.dev). PGP available on request.

Please include:

- A description of the vulnerability and its impact.
- Steps to reproduce.
- Any suggested fix, if you have one.

### What to expect

| Stage                        | Target timeline                                   |
| :--------------------------- | :------------------------------------------------ |
| Acknowledgement of report    | within **3 business days**                        |
| Initial assessment & triage  | within **7 business days**                        |
| Patch released (if accepted) | depends on severity — critical issues prioritised |
| Public disclosure & advisory | coordinated with reporter after the patch ships   |

## Credit

Reporters who follow this process responsibly are credited in the [CHANGELOG](CHANGELOG.md) and the corresponding GitHub Security Advisory, unless they prefer to remain anonymous.

---

Maintained by [Titus Kirch](https://github.com/TitusKirch/) / [IT-Dienstleistungen Titus Kirch](https://kirch.dev).
