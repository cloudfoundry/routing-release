#!/bin/bash

set -eu
set -o pipefail

THIS_FILE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
REPO="${THIS_FILE_DIR}/../.."
CI="${THIS_FILE_DIR}/../../../wg-app-platform-runtime-ci"
unset THIS_FILE_DIR

DB="${1?Provide DB flavor e.g mysql-8.0(or mysql),mysql-5.7,postgres}"

if [[ "${DB}" == "mysql" ]] || [[ "${DB}" == "mysql-8.0" ]]; then
  IMAGE="cloudfoundry/tas-runtime-mysql-8.0"
  DB="mysql"
elif [[ "${DB}" == "mysql-5.7" ]]; then
  IMAGE="cloudfoundry/tas-runtime-mysql-5.7"
  DB="mysql"
elif [[ "${DB}" == "postgres" ]]; then
  IMAGE="cloudfoundry/tas-runtime-postgres"
else
  echo "Unsupported DB flavor"
  exit 1
fi

shift 1

if [[ -z "${*}" ]]; then
  ARGS="-it"
else
  ARGS="${*}"
fi

echo $ARGS

docker pull "${IMAGE}"
docker rm -f routing-release-docker-container
docker run -it \
  --env "DB=${DB}" \
  --name "routing-release-docker-container" \
  -v "${REPO}:/repo" \
  -v "${CI}:/ci" \
  ${ARGS} \
  "${IMAGE}" \
  /bin/bash
  
