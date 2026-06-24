# Contributing to terraform-provider-discord

Thanks for taking the time to contribute! 🛠️ This document covers what you need to get a PR landed.

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you agree to uphold it.

## Reporting issues

- **Bugs**: open a [Bug report](https://github.com/kirchDev/terraform-provider-discord/issues/new?template=bug_report.yml) with a minimal reproduction if at all possible.
- **Feature requests**: open a [Feature request](https://github.com/kirchDev/terraform-provider-discord/issues/new?template=feature_request.yml).
- **Questions**: open a [Question](https://github.com/kirchDev/terraform-provider-discord/issues/new?template=question.yml).
- **Security vulnerabilities**: **do not** open a public issue. Follow [SECURITY.md](SECURITY.md).

## Development setup

Requirements:

- **Go 1.25+** (the provider — `terraform-plugin-framework v1.19` needs it) and `golangci-lint`
- Node **24+** and **pnpm 11** (the meta layer: lint/format/hooks/release tooling)
- `git`; OpenTofu or Terraform to load the built binary
- A Discord bot token (`DISCORD_TOKEN`) if you want to run against the live API

Clone and install:

```bash
git clone https://github.com/kirchDev/terraform-provider-discord.git
cd terraform-provider-discord
pnpm install   # wires husky hooks
make build     # builds the provider binary
```

## Running the suite

| Command          | What it does                              |
| :--------------- | :---------------------------------------- |
| `make build`     | Build the provider binary.                |
| `make vet`       | `go vet ./...`.                           |
| `make test`      | Go unit tests.                            |
| `make testacc`   | Mock acceptance tests (no token needed).  |
| `pnpm lint`      | oxlint across the repo.                   |
| `pnpm format`    | oxfmt check across JS / JSON / YAML / MD. |
| `pnpm check`     | Runs `lint` and `format`.                 |
| `pnpm check:fix` | Auto-fix lint + format issues.            |

The same commands run in CI — keep them green before you push.

## Branching & PRs

1. **Don't push directly to `main`.** Branch off `main` for every change.
2. **Conventional Commits required.** Commitlint enforces this on every commit. Examples:
   - `feat(role): add discord_role resource`
   - `fix(client): honour 429 retry_after`
   - `docs(readme): document the provider config block`
   - `chore(deps): bump terraform-plugin-framework`
   - Breaking changes: `feat!: ...` or include `BREAKING CHANGE:` in the body.
3. **One concern per PR.** Smaller PRs land faster.
4. **Update relevant docs.** README, CONTRIBUTING, or resource docs if you change behaviour.

## Style & quality gates

Husky runs the following on `git commit`:

- **JS / JSON / YAML / MD** → `oxlint` + `oxfmt`

For Go, run `make fmt && make vet` before pushing — CI gofmt-checks, vets, builds and tests. If a hook fails, fix the issue and commit again. **Don't `--no-verify`** unless I explicitly ask.

> [!TIP]
> Run `pnpm check:fix` and `make fmt` before opening a PR — saves a CI cycle.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
