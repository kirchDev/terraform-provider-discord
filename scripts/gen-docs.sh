#!/usr/bin/env bash
# Generate the provider documentation under docs/ with tfplugindocs.
#
# tfplugindocs normally pulls the provider from the registry to read its schema.
# This provider isn't published yet, so we build it locally, export the schema
# via OpenTofu/Terraform + a dev_overrides config, normalize the provider
# address to the canonical registry.terraform.io/hashicorp/<name> key that
# tfplugindocs expects, and feed that JSON in with --providers-schema.
#
# Requires: go, jq, and tofu (or terraform) on PATH. Network access (the
# tfplugindocs tool is fetched via `go run ...@latest`).
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO"

PROVIDER_NAME="discord"
LOCAL_ADDR="kirchdev/${PROVIDER_NAME}"
LOCAL_FQN="registry.opentofu.org/${LOCAL_ADDR}"
CANON_FQN="registry.terraform.io/hashicorp/${PROVIDER_NAME}"
TFPLUGINDOCS_VERSION="${TFPLUGINDOCS_VERSION:-latest}"

TF_BIN="$(command -v tofu || command -v terraform || true)"
[ -n "$TF_BIN" ] || { echo "error: need 'tofu' or 'terraform' on PATH" >&2; exit 1; }
command -v jq >/dev/null || { echo "error: need 'jq' on PATH" >&2; exit 1; }

echo "==> building provider binary"
go build -o "${REPO}/terraform-provider-${PROVIDER_NAME}" .

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

cat > "$WORK/dev.tfrc" <<EOF
provider_installation {
  dev_overrides { "${LOCAL_ADDR}" = "${REPO}" }
  direct {}
}
EOF

cat > "$WORK/main.tf" <<EOF
terraform {
  required_providers {
    ${PROVIDER_NAME} = {
      source = "${LOCAL_ADDR}"
    }
  }
}
provider "${PROVIDER_NAME}" {}
EOF

echo "==> exporting provider schema via ${TF_BIN##*/} (dev_overrides)"
DISCORD_TOKEN=dummy TF_CLI_CONFIG_FILE="$WORK/dev.tfrc" \
  "$TF_BIN" -chdir="$WORK" providers schema -json > "$WORK/schema.json"

echo "==> normalizing provider address for tfplugindocs"
jq --arg from "$LOCAL_FQN" --arg to "$CANON_FQN" \
  '{format_version: .format_version, provider_schemas: {($to): .provider_schemas[$from]}}' \
  "$WORK/schema.json" > "$WORK/schema-canon.json"

echo "==> rendering docs/ with tfplugindocs"
go run "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@${TFPLUGINDOCS_VERSION}" generate \
  --provider-name "$PROVIDER_NAME" \
  --providers-schema "$WORK/schema-canon.json"

echo "==> done: $(find docs -name '*.md' | wc -l) pages under docs/"
