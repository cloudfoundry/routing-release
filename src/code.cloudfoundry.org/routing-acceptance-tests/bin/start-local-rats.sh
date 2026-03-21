#!/usr/bin/env bash

set -e

ACCEPTANCE_DIR=${ACCEPTANCE_DIR:?"ACCEPTANCE_DIR must be set"}
KIND_DEPLOYMENT_DIR=${KIND_DEPLOYMENT_DIR:?"KIND_DEPLOYMENT_DIR must be set"}
CONFIG=${CONFIG:?"CONFIG must be set"}
RTR_BIN=${RTR_BIN:?"RTR_BIN must be set"}
EXIT_STATUS=0
VERBOSE_MODE="${VERBOSE:+-v}"

routing_api_ip=$(docker inspect cfk8s-worker --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
tcp_apps_domain=$(cat "$CONFIG" | jq -r '.tcp_apps_domain')
jq --arg ip "$routing_api_ip" '.addresses = [$ip]' "$CONFIG" > "${CONFIG}.tmp" && mv "${CONFIG}.tmp" "$CONFIG"

export PATH=${RTR_BIN}:${PATH}
export CONFIG

make -C $KIND_DEPLOYMENT_DIR login
cf target -o system && cf buildpacks | grep -q "go_buildpack" || make -C $KIND_DEPLOYMENT_DIR bootstrap
cf target -o system && cf domains | grep -q "$tcp_apps_domain" || cf create-shared-domain $tcp_apps_domain --router-group default-tcp

# Retry flaky tests up to 10 times to handle transient network timeouts; sleep 60 seconds between suites to allow connections/DNS caches to clear and prevent port conflicts
echo "HTTP Routing Tests"
ginkgo -randomize-all $VERBOSE_MODE -nodes=1 -keep-going --flake-attempts 10 --after-run-hook "sleep 60" "${ACCEPTANCE_DIR}/http_routes" || EXIT_STATUS=$?

echo "Sleeping for 60 seconds to allow any lingering connections to close before starting TCP routing tests..."
sleep 60

echo "TCP Routing Tests"
ginkgo -randomize-all $VERBOSE_MODE -nodes=1 -keep-going --flake-attempts 10 --after-run-hook "sleep 60" "${ACCEPTANCE_DIR}/tcp_routing" || EXIT_STATUS=$?

echo "Cleaning up test artifacts..."
cf target -o system && sleep 5 && cf delete-shared-domain $tcp_apps_domain -f

[[ "${CLEANUP_CATS_ORGS:-false}" == "true" ]] && echo "Cleanup the orgs" && cf orgs | grep "^CATS-" | xargs -I {} cf delete-org {} -f && sleep 5

echo "Acceptance Tests Complete; exit status: $EXIT_STATUS"
exit $EXIT_STATUS
