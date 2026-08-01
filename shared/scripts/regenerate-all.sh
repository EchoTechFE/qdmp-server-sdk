#!/usr/bin/env bash
# Regenerates every generated artifact in the repo from shared/openapi.yaml,
# in dependency order. Used by contributors after editing the spec, and by
# .github/workflows/generate-check.yml to detect drift (re-run this, then
# `git diff --exit-code` the generated paths).
#
# Requires on PATH: node, go, and java (for the openapi-generator-cli jar,
# downloaded and SHA-256-verified on demand below if OPENAPI_GENERATOR_CLI_JAR
# isn't already set to an existing jar path). oapi-codegen does NOT need to be
# preinstalled: it's invoked via `go run <module>@<pinned version>` below, so
# the version is pinned in this file and Go verifies the module's checksum
# against sum.golang.org on first fetch.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

echo "==> shared: parsing openapi.yaml extensions into shared/generated/*.json"
node shared/scripts/gen-route-meta.mjs

echo "==> node: openapi-typescript + route-meta/error-codes mirror"
(cd node && npx openapi-typescript ../shared/openapi.yaml -o src/generated/schema.d.ts && node scripts/gen-route-meta.mjs)

echo "==> java: openapi-generator-cli (models only) + route-meta/error-codes mirror"
OPENAPI_GENERATOR_CLI_VERSION="7.16.0"
# Verified against the artifact's published Maven Central SHA-1 as of writing;
# pinning our own SHA-256 here since Maven Central doesn't publish one.
OPENAPI_GENERATOR_CLI_SHA256="6999b18cece5b58f5d5b246fef5a43bdf61239491c4f0ed6513214e0f6e8464b"

verify_openapi_generator_cli_jar() {
  local jar="$1"
  [ "$(shasum -a 256 "$jar" | awk '{print $1}')" = "$OPENAPI_GENERATOR_CLI_SHA256" ]
}

if [ -n "${OPENAPI_GENERATOR_CLI_JAR:-}" ]; then
  # Caller explicitly pointed us at a jar (a local mirror, or a different
  # generator version) -- trust it as given, the same as this SDK's other
  # explicit-override conventions. We have no known-good hash for an
  # arbitrary caller-chosen jar. Fail now, before the `rm -rf
  # java/generated/src` below, if a stale/typo'd override points at nothing
  # -- otherwise this turns a recoverable "tool not found" into "generated
  # sources deleted, then the build fails".
  if [ ! -f "$OPENAPI_GENERATOR_CLI_JAR" ]; then
    echo "OPENAPI_GENERATOR_CLI_JAR=$OPENAPI_GENERATOR_CLI_JAR does not exist" >&2
    exit 1
  fi
  echo "    using caller-supplied OPENAPI_GENERATOR_CLI_JAR=$OPENAPI_GENERATOR_CLI_JAR (not checksum-verified)"
else
  OPENAPI_GENERATOR_CLI_JAR="/tmp/tools/openapi-generator-cli-${OPENAPI_GENERATOR_CLI_VERSION}.jar"
  if [ -f "$OPENAPI_GENERATOR_CLI_JAR" ] && ! verify_openapi_generator_cli_jar "$OPENAPI_GENERATOR_CLI_JAR"; then
    echo "    cached $OPENAPI_GENERATOR_CLI_JAR failed SHA-256 verification, discarding and re-downloading"
    rm -f "$OPENAPI_GENERATOR_CLI_JAR"
  fi
  if [ ! -f "$OPENAPI_GENERATOR_CLI_JAR" ]; then
    echo "    downloading openapi-generator-cli $OPENAPI_GENERATOR_CLI_VERSION to $OPENAPI_GENERATOR_CLI_JAR"
    mkdir -p "$(dirname "$OPENAPI_GENERATOR_CLI_JAR")"
    curl --fail -sSL -o "$OPENAPI_GENERATOR_CLI_JAR" \
      "https://repo1.maven.org/maven2/org/openapitools/openapi-generator-cli/${OPENAPI_GENERATOR_CLI_VERSION}/openapi-generator-cli-${OPENAPI_GENERATOR_CLI_VERSION}.jar"
  fi
  if ! verify_openapi_generator_cli_jar "$OPENAPI_GENERATOR_CLI_JAR"; then
    echo "openapi-generator-cli-${OPENAPI_GENERATOR_CLI_VERSION}.jar failed SHA-256 verification (expected $OPENAPI_GENERATOR_CLI_SHA256), refusing to run it" >&2
    exit 1
  fi
fi
rm -rf java/generated/src
# -t overrides just the file-header partial (java/openapi-generator-templates/licenseInfo.mustache)
# so every one of the ~70 generated model files doesn't repeat the full spec
# title/description; everything else still falls back to openapi-generator-cli's
# built-in Java/resttemplate templates.
java -jar "$OPENAPI_GENERATOR_CLI_JAR" generate \
  -i shared/openapi.yaml -g java -o java/generated \
  --global-property models,modelTests=false,modelDocs=false \
  --model-package=io.github.echotechfe.qdmp.generated \
  --library=resttemplate \
  --additional-properties=hideGenerationTimestamp=true,dateLibrary=java8,openApiNullable=false \
  -t java/openapi-generator-templates
node java/scripts/gen-route-meta.mjs

echo "==> go: oapi-codegen (models only, pinned to v2.8.0)"
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 \
  -config go/generated/oapi-codegen-config.yaml shared/openapi.yaml

echo "==> done"
