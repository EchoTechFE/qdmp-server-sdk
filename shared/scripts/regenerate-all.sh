#!/usr/bin/env bash
# Regenerates every generated artifact in the repo from shared/openapi.yaml,
# in dependency order. Used by contributors after editing the spec, and by
# .github/workflows/generate-check.yml to detect drift (re-run this, then
# `git diff --exit-code` the generated paths).
#
# Requires on PATH: node, go, oapi-codegen (go install
# github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest), and java
# (for the openapi-generator-cli jar, downloaded on demand below if
# OPENAPI_GENERATOR_CLI_JAR isn't already set to an existing jar path).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

echo "==> shared: parsing openapi.yaml extensions into shared/generated/*.json"
node shared/scripts/gen-route-meta.mjs

echo "==> node: openapi-typescript + route-meta/error-codes mirror"
(cd node && npx openapi-typescript ../shared/openapi.yaml -o src/generated/schema.d.ts && node scripts/gen-route-meta.mjs)

echo "==> java: openapi-generator-cli (models only) + route-meta/error-codes mirror"
OPENAPI_GENERATOR_CLI_JAR="${OPENAPI_GENERATOR_CLI_JAR:-/tmp/tools/openapi-generator-cli.jar}"
if [ ! -f "$OPENAPI_GENERATOR_CLI_JAR" ]; then
  echo "    downloading openapi-generator-cli 7.16.0 to $OPENAPI_GENERATOR_CLI_JAR"
  mkdir -p "$(dirname "$OPENAPI_GENERATOR_CLI_JAR")"
  curl -sSL -o "$OPENAPI_GENERATOR_CLI_JAR" \
    https://repo1.maven.org/maven2/org/openapitools/openapi-generator-cli/7.16.0/openapi-generator-cli-7.16.0.jar
fi
rm -rf java/generated/src
java -jar "$OPENAPI_GENERATOR_CLI_JAR" generate \
  -i shared/openapi.yaml -g java -o java/generated \
  --global-property models,modelTests=false,modelDocs=false \
  --model-package=io.github.echotechfe.qdmp.generated \
  --library=resttemplate \
  --additional-properties=hideGenerationTimestamp=true,dateLibrary=java8,openApiNullable=false
node java/scripts/gen-route-meta.mjs

echo "==> go: oapi-codegen (models only)"
oapi-codegen -config go/generated/oapi-codegen-config.yaml shared/openapi.yaml

echo "==> done"
