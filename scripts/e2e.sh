#!/usr/bin/env sh
# builds every variant with tools/build into bin/e2e and runs the end-to-end tests in e2e/ against them
# extra arguments go to go test, for example: sh scripts/e2e.sh -run 'TestBinaries/abot$' -v

if [ ! -f "go.mod" ]; then
    echo "No 'go.mod' found. This script must be run from the project root!" >&2
    exit 1
fi

. tools/messages
. tools/variants

out=bin/e2e
rm -rf "$out"
for variant in $VARIANTS; do
    OUT_DIR=$out sh tools/build "$variant" || exit 1
done
THE_AGENT_CLI_E2E_BIN_DIR=$out go test -count=1 "$@" ./e2e
