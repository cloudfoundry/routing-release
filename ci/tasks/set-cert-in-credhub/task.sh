#!/bin/bash -exu

pushd "bbl-state/${BBL_STATE_DIR}"
  eval "$(bbl print-env)"
popd

set +x
credhub set \
  --name="${CREDHUB_KEY}" \
  --type=certificate \
  --certificate="cert-directory/${CERTIFICATE_FILE}"
set -x

