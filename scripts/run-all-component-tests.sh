#!/bin/bash

set -e

PACKAGE="$1"
SUB_PACKAGE="$2"

if [[ -n "${PACKAGE}" ]]; then
  pushd "./src/code.cloudfoundry.org/${PACKAGE}"
    echo "Testing component: ${PACKAGE}"
    if [[ -n "${SUB_PACKAGE}" ]]; then
      ./bin/test -flakeAttempts 3 ${SUB_PACKAGE}
    else
      ./bin/test -flakeAttempts 3
    fi
  popd
else
  for component in gorouter cf-tcp-router multierror route-registrar routing-api routing-api-cli ; do
    pushd src/code.cloudfoundry.org/${component}
      echo "Testing component: ${component}..."
      ./bin/test --flakeAttempts=3
    popd
  done
fi
