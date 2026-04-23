#!/usr/bin/env bash
# wait-ready.sh — poll each service until it responds. Useful in CI and after
# `docker compose up -d` to avoid racing the orderer/peer startup.

set -euo pipefail

TIMEOUT="${TIMEOUT:-120}"
BIND_HOST="${BIND_HOST:-127.0.0.1}"

wait_for_port() {
    local name="$1" host="$2" port="$3"
    local deadline=$(( $(date +%s) + TIMEOUT ))
    while ! (echo > "/dev/tcp/${host}/${port}") 2>/dev/null; do
        if (( $(date +%s) > deadline )); then
            echo "[wait-ready] TIMEOUT waiting for ${name} (${host}:${port})" >&2
            return 1
        fi
        sleep 1
    done
    echo "[wait-ready] ✓ ${name}"
}

# Always-present services (tiny profile and above)
wait_for_port "orderer0"            "$BIND_HOST" 7050
wait_for_port "peer0-hospital"      "$BIND_HOST" 7051
wait_for_port "ipfs0 swarm"         "$BIND_HOST" 4001
wait_for_port "ipfs0 api"           "$BIND_HOST" 5001
wait_for_port "prometheus"          "$BIND_HOST" 9090
wait_for_port "grafana"             "$BIND_HOST" 3000

# Optional services — skip silently if not bound
optional_port() {
    local name="$1" host="$2" port="$3"
    if (echo > "/dev/tcp/${host}/${port}") 2>/dev/null; then
        echo "[wait-ready] ✓ ${name} (optional)"
    fi
}

optional_port "peer1-hospital"  "$BIND_HOST" 8051
optional_port "peer0-regulator" "$BIND_HOST" 9051
optional_port "peer1-regulator" "$BIND_HOST" 10051
optional_port "orderer1"        "$BIND_HOST" 7150
optional_port "orderer2"        "$BIND_HOST" 7250

echo "[wait-ready] all services up."
