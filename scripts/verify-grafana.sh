#!/usr/bin/env bash
set -euo pipefail

GRAFANA_URL="${GRAFANA_URL:-http://localhost:3001}"
USER="${GRAFANA_USER:-admin}"
PASS="${GRAFANA_PASS:-admin}"

echo "==> Waiting for Grafana API..."
for i in $(seq 1 30); do
  if curl -fsS "${GRAFANA_URL}/api/health" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "==> Checking data source..."
DS=$(curl -fsS -u "${USER}:${PASS}" "${GRAFANA_URL}/api/datasources/uid/prometheus" | jq -r '.uid')
if [ "$DS" != "prometheus" ]; then
  echo "FAIL: expected data source prometheus, got: $DS"
  exit 1
fi
echo "  data source OK"

echo "==> Fetching dashboard..."
DASH=$(curl -fsS -u "${USER}:${PASS}" "${GRAFANA_URL}/api/dashboards/uid/gojudge-overview" | jq -r '.dashboard.uid')
if [ "$DASH" != "gojudge-overview" ]; then
  echo "FAIL: expected dashboard gojudge-overview, got: $DASH"
  exit 1
fi

echo "==> Checking row titles..."
ROWS=$(curl -fsS -u "${USER}:${PASS}" "${GRAFANA_URL}/api/dashboards/uid/gojudge-overview" | jq -r '[.dashboard.panels[] | select(.type=="row") | .title] | join(" ")')
for row in "API" "Queue / Outbox" "Worker" "Judge"; do
  if echo "$ROWS" | grep -qF "$row"; then
    echo "  row '$row' OK"
  else
    echo "FAIL: missing row '$row'"
    exit 1
  fi
done

echo "Grafana verification passed."
