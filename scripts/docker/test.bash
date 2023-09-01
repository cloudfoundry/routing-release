#!/bin/bash

set -eu
set -o pipefail

function test() {
  local package="${1:?Provide a package}"
  local sub_package="${2:-}"

  export DIR=${package}
  . <(/ci/shared/helpers/extract-default-params-for-task.bash /ci/shared/tasks/run-bin-test/linux.yml)

  export DEFAULT_PARAMS="ci/routing-release/default-params/run-bin-test/linux.yml"
  export ENVS="DB=${DB}"
  export GOFLAGS="-buildvcs=false"
  /ci/shared/tasks/run-bin-test/task.bash "${sub_package}"
}

if [[ -n "${1:-}" ]]; then
  test "src/code.cloudfoundry.org/${1}" "${2:-}"
else
  for component in gorouter cf-tcp-router multierror route-registrar routing-api routing-api-cli routing-info; do
    test "src/code.cloudfoundry.org/${component}"
  done
fi
