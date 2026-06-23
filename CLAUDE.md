# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

`scaffold` is a **GitHub template repository**, not an application. It ships the meta layer (lint, format, commit hooks, CI, CodeQL, Dependabot, release-please, issue/PR templates, standard meta docs) that every new kirchDev repo should start with. There is no application code — the project code can be anything (PHP, Go, Rust, Vue, shell). Only the meta layer lives here.

Implication: when changing files, ask "does this default make sense for _every_ future repo created from this template?" — not just for one project type.

## Commands

| Command           | What it does                                               |
| :---------------- | :--------------------------------------------------------- |
| `pnpm install`    | Install deps and wire husky hooks via the `prepare` script |
| `pnpm lint`       | `oxlint . --deny-warnings`                                 |
| `pnpm format`     | `oxfmt --check .` (note: `format` is the check, not fix)   |
| `pnpm check`      | Runs `lint` + `format` — the CI gate                       |
| `pnpm lint:fix`   | Auto-fix lint                                              |
| `pnpm format:fix` | Auto-fix format                                            |
| `pnpm check:fix`  | Auto-fix lint + format                                     |
| `pnpm taze`       | Interactive dependency upgrade check                       |
| `pnpm taze:w`     | Write upgrade results                                      |

There is no test suite — this is config-only. CI runs `pnpm lint` and `pnpm format` on PR.

## Architecture / conventions

- **Node 24, pnpm 11.** Pinned via `.nvmrc`, `engines`, and `packageManager`. `.npmrc` enforces `minimumReleaseAge=4320` (3-day cooldown), `trustPolicy=no-downgrade`, isolated node-linker. Don't loosen these without reason.
- **oxc, not eslint/prettier.** Linting via `oxlint`, formatting via `oxfmt`. Configs live in `.oxlintrc.json` / `.oxfmtrc.json`. `oxlint` uses `unicorn` + `oxc` plugins; rules deliberately minimal.
- **Husky hooks** (`.husky/pre-commit`, `.husky/commit-msg`) run `lint-staged` and `commitlint`. `lint-staged.config.js` excludes `README.md` (free-form prose) and `pnpm-lock.yaml`. `oxlint --fix --deny-warnings` then `oxfmt` on JS; `oxfmt` only on JSON/YAML/MD.
- **Conventional Commits enforced** via `@commitlint/config-conventional`. Don't `--no-verify` unless explicitly asked.
- **release-please is included** (unlike many templates that omit it). Files: `release-please-config.json`, `.release-please-manifest.json`, `.github/workflows/release-please.yml`. Config uses `release-type: simple` (language-agnostic), `include-v-in-tag: true`. Downstream repos start at `0.0.0` and reset via the steps in README → _Resetting release-please_.
- **Workflows** use `actions/checkout@v6`, `actions/setup-node@v6`, `pnpm/action-setup@v6`, `github/codeql-action/{init,analyze}@v4`. Keep these pinned to major versions; Dependabot bumps them monthly.
- **CodeQL** scans `actions` + `javascript-typescript` with `security-extended,security-and-quality` queries, gated by path filters so non-code changes don't trigger it.
- **Dependabot** groups all minor/patch updates per ecosystem into a single PR (`npm-minor-patch`, `actions-minor-patch`). Majors come as separate PRs.

## House style for READMEs and meta files

`/write-readme` skill encodes the canonical structure. Key rules: hero block wrapped in `<div align="center">`, prescribed section emojis (✨ Features, 🚀 Setup, 🤝 Contributing, 🛣️ Versioning, 📄 License), license footer always reads `[MIT](LICENSE) © [Titus Kirch](https://github.com/TitusKirch/) / [IT-Dienstleistungen Titus Kirch](https://kirch.dev)`. Use GitHub callouts (`> [!TIP]`, `> [!IMPORTANT]`), never plain blockquotes.

## When editing this template

- Every file referencing `TitusKirch/scaffold` is a placeholder that downstream users will replace. Keep the references consistent so a single `grep -rn "TitusKirch/scaffold"` catches them all.
- `forgemap` (sibling repo at `../forgemap`) is the de-facto reference implementation of these conventions. When unsure about a config choice, check what forgemap does.
- The template's own `package.json` is `"private": true` and `"name": "scaffold"` — not published anywhere.
