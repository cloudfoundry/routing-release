#!/bin/bash

set -eu
set -o pipefail

function lint() {
  . <(/ci/shared/helpers/extract-default-params-for-task.bash /ci/shared/tasks/lint-repo/linux.yml)

  /ci/shared/tasks/lint-repo/task.bash
}

lint
