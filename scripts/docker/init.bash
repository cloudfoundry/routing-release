#!/bin/bash

for dir in /built-binaries/*; do
  envVar="$(echo "${dir}" | cut -d "/" -f 3 | tr '[:lower:]' '[:upper:]' | tr '-' '_')_BINARY"
  eval "export ${envVar}=$dir/run";
done

pushd /repo
. "/ci/routing-release/helpers/configure-binaries.bash"
. "/ci/shared/helpers/helpers.bash"
expand_functions

configure_rsyslog
configure_db "${DB}"
