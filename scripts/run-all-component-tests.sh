#!/bin/bash

PACKAGE="$1"

if [[ -n "${PACKAGE}" ]]; then
    pushd "./src/code.cloudfoundry.org/${PACKAGE}"
    echo "Testing component: ${PACKAGE}"
    ./bin/test
  popd
else
  for component in gorouter cf-tcp-router multierror route-registrar routing-api routing-api-cli uaa-go-client; do
    pushd src/code.cloudfoundry.org/${component}
      echo "Testing component: ${component}..."
      ./bin/test
    popd
  done
fi
