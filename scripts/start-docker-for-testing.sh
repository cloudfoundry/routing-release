#!/bin/bash -x

BASEDIR="$(cd $(dirname "$0") && pwd)"
ROUTING_RELEASE_DIR="${BASEDIR}/.."

IMAGE="cloudfoundry/cf-routing-pipeline"
MOUNT_DIR="/routing-release"
PREPARE_CONTAINER="cd ${MOUNT_DIR}; source .envrc; scripts/setup-test-environment.sh; bash"

docker pull "${IMAGE}"
docker run -it \
  -v "${ROUTING_RELEASE_DIR}:${MOUNT_DIR}" \
  "${IMAGE}" \
  /bin/bash -c "${PREPARE_CONTAINER}"

