#!/usr/bin/env bash
# Send the sample payloads in testdata/ to a running alert-copilot.
#   BASE_URL=http://localhost:8080 WEBHOOK_TOKEN=... ./scripts/send-sample.sh
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/testdata"
AUTH=()
if [[ -n "${WEBHOOK_TOKEN:-}" ]]; then
  AUTH=(-H "Authorization: Bearer ${WEBHOOK_TOKEN}")
fi

post() {
  local path="$1" ctype="$2" file="$3"
  printf '%-24s -> ' "$path"
  curl -sS -o /dev/null -w '%{http_code}\n' "${AUTH[@]+"${AUTH[@]}"}" \
    -H "Content-Type: ${ctype}" --data-binary "@${DIR}/${file}" "${BASE_URL}${path}"
}

post /webhook/alertmanager application/json                  alertmanager.json
post /webhook/alicloud     application/x-www-form-urlencoded alicloud.form
post /webhook/alicloud     application/x-www-form-urlencoded alicloud-ok.form
post /webhook/generic      application/json                  generic.json
