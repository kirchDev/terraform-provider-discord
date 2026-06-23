# Security Policy

## Scope

`scaffold` is a **template repository** — it contains configuration files, GitHub workflows, and meta documents that are copied into new repositories. It is not a runtime package and has no users in the traditional sense.

The supported "version" is always the **tip of `main`**. There are no historical branches to back-port fixes to; downstream repositories should re-pull the relevant file(s) from `main` if a vulnerability is discovered in the shipped templates.

## Reporting a Vulnerability

**Please do not file a public GitHub issue for security problems.**

In the context of this template, a "vulnerability" typically means:

- An insecure default in a shipped workflow (e.g. overly broad `permissions`).
- A misconfigured Action that could leak secrets.
- A dependency in `package.json` that introduces a known CVE.

Use one of the following private channels:

1. **GitHub Private Vulnerability Reporting** (preferred): open a private advisory at <https://github.com/TitusKirch/scaffold/security/advisories/new>.
2. **Email**: [titus.kirch@kirch.dev](mailto:titus.kirch@kirch.dev). PGP available on request.

Please include:

- A description of the vulnerability and its impact on downstream repositories.
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
