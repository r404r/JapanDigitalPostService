#!/usr/bin/env bash
# Generate the tracked regression summary used at task handoff.
set -euo pipefail

cd "$(dirname "$0")/.."
mkdir -p output

coverage_file="output/coverage.out"
report_file="output/regression-report.txt"

go test ./... -coverprofile="$coverage_file"
coverage_summary=$(go tool cover -func="$coverage_file")

{
	printf 'JapanDigitalPostService regression report\n'
	printf '=========================================\n\n'
	printf 'Command: go test ./... -coverprofile=%s\n\n' "$coverage_file"
	printf 'Coverage summary:\n'
	printf '%s\n' "$coverage_summary"
} > "$report_file"

printf 'wrote %s\n' "$report_file"
