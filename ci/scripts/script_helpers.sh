#!/bin/bash

function bosh_login() {
  ENV=${1}
  ENV_DIR="${ENV_DIR:-}"
  DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  DEPLOYMENT_DIR="${DIR}/../../../deployments-routing/${ENV_DIR}/${ENV}"

  if [ "$ENV" = "lite" ]; then
    bosh_login_lite
    return
  fi
  if [ -f "${DEPLOYMENT_DIR}/bosh-vars.yml" ]; then
    DIRECTOR_NAME=$(< "${DEPLOYMENT_DIR}/terraform.tfvars" jq -r .env_name)
    if [ "${ENV}" != "${DIRECTOR_NAME}" ]; then
      echo "Director Name does not match given env_name"
      return 1
    fi
    bosh_login_bosh_vars "${ENV}"
  else
    if [ ! -f "${DEPLOYMENT_DIR}/bbl-state.json" ]; then
      if [ ! -f "${DEPLOYMENT_DIR}/bbl-state/bbl-state.json" ]; then
        echo "Neither bosh-vars.yml nor bbl-state.json is found in ${DEPLOYMENT_DIR}"
        return 1
      fi
    fi
    bosh_login_bbl "${ENV}"
  fi
}

function bosh_login_bosh_vars() {
  ENV=${1}
  BOSH_CLIENT_SECRET="$(bosh int "${DEPLOYMENT_DIR}/bosh-vars.yml" --path /admin_password)"
  BOSH_CA_CERT="$(bosh int "${DEPLOYMENT_DIR}/bosh-vars.yml" --path /director_ssl/ca)"
  DIRECTOR_IP="$(bosh int "${DEPLOYMENT_DIR}/bosh-vars.yml" --path /external_ip)"
  BOSH_ENVIRONMENT="$(< "${DEPLOYMENT_DIR}/terraform.tfstate" jq -r .modules[0].outputs.external_ip.value)"
  JUMPBOX_PRIVATE_KEY="/tmp/${ENV}"
  bosh int "${DEPLOYMENT_DIR}/bosh-vars.yml" --path /jumpbox_ssh/private_key > "${JUMPBOX_PRIVATE_KEY}"
  chmod 600 "${JUMPBOX_PRIVATE_KEY}"
  export BOSH_CLIENT="admin"
  export BOSH_DEPLOYMENT="cf"
  export BOSH_GW_USER="jumpbox"
  export BOSH_GW_HOST="${DIRECTOR_IP}"
  export BOSH_CLIENT_SECRET
  export BOSH_CA_CERT
  export BOSH_ENVIRONMENT
  export JUMPBOX_PRIVATE_KEY

  bosh -e "${BOSH_ENVIRONMENT}" --ca-cert <(echo "${BOSH_CA_CERT}") alias-env "${ENV}"
  bosh login
}

function bosh_login_bbl() {
  if [ -z "${DEPLOYMENT_DIR}"  ]; then
    echo "missing DEPLOYMENT_DIR. you probably meant to use bosh_login."
    return
  fi

  ENV=${1}
  local bbl_state_dir
  if [ -f "${DEPLOYMENT_DIR}/bbl-state.json" ]; then
    bbl_state_dir="${DEPLOYMENT_DIR}"
  else
    bbl_state_dir="${DEPLOYMENT_DIR}/bbl-state"
  fi

  JUMPBOX_PRIVATE_KEY="/tmp/${ENV}"
  touch "$JUMPBOX_PRIVATE_KEY"
  chmod 600 "${JUMPBOX_PRIVATE_KEY}"
  export JUMPBOX_PRIVATE_KEY
  local director_ip
  pushd "${bbl_state_dir}"
    eval "$(bbl print-env)"
    director_ip="$(bbl director-address | grep -o '[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}')"
    bbl ssh-key > "$JUMPBOX_PRIVATE_KEY"
  popd

  export BOSH_DEPLOYMENT="cf"
  export BOSH_GW_USER="jumpbox"
  export BOSH_GW_HOST="${director_ip}"


  bosh -e "${BOSH_ENVIRONMENT}" --ca-cert <(echo "${BOSH_CA_CERT}") alias-env "${ENV}"
  bosh login
}

function bosh_login_lite ()
{
  bosh_logout
  local env_dir=${HOME}/workspace/deployments-routing/lite

  pushd $env_dir >/dev/null
    BOSH_CLIENT="admin"
    BOSH_CLIENT_SECRET="$(bosh int ./bosh-vars.yml --path /admin_password)"
    BOSH_ENVIRONMENT="vbox"
    BOSH_CA_CERT="$(bosh int ./bosh-vars.yml --path /director_ssl/ca)"

    export BOSH_CLIENT
    export BOSH_CLIENT_SECRET
    export BOSH_ENVIRONMENT
    export BOSH_CA_CERT
  popd 1>/dev/null

  export BOSH_DEPLOYMENT=cf;
}


function bosh_logout ()
{
  unset BOSH_BBL_ENVIRONMENT
  unset BOSH_USER
  unset BOSH_PASSWORD
  unset BOSH_ENVIRONMENT
  unset BOSH_GW_HOST
  unset BOSH_GW_USER
  unset BOSH_GW_PRIVATE_KEY
  unset BOSH_CA_CERT
  unset BOSH_DEPLOYMENT
  unset BOSH_CLIENT
  unset BOSH_CLIENT_SECRET
  unset JUMPBOX_PRIVATE_KEY
  unset BOSH_ALL_PROXY
  unset CREDHUB_SERVER
  unset CREDHUB_CA_CERT
  unset CREDHUB_PROXY
  unset CREDHUB_CLIENT
  unset CREDHUB_SECRET
}

function extract_var()
{
  local env=${1}
  local var=${2}

  bosh_login "${env}" > /dev/null
  credhub find -j -n "${var}" | jq -r .credentials[].name | xargs -n 1 -I {} credhub get -j -n {} | jq -r .value
}

function get_system_domain()
{
  local env
  env=${1}
  if [ "${env}" = "lite" ]; then
    echo "bosh-lite.com"
    return
  fi

  echo "${env}.routing.cf-app.com"
}

function cf_login()
{
  env=${1}
  local cf_admin_passsword
  if [ "${env}" = "lite" ]; then
    local env_dir=${HOME}/workspace/deployments-routing/lite
    cf_admin_password="$(bosh int ${env_dir}/deployment-vars.yml --path /cf_admin_password)"
  else
    cf_admin_password="$(extract_var "${env}" cf_admin_password)"
  fi
  cf api "api.$(get_system_domain "${env}")" --skip-ssl-validation
  cf auth admin "${cf_admin_password}"
}

function gke_login()
{
  local env
  env=${1}
  gcloud container clusters get-credentials ${env} --zone=us-west1-a --project=cf-routing
}

function concourse_credhub_login()
{
  local concourse_env
  local web_ip
  local credhub_address
  concourse_env="concourse-gcp"
  DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
  DEPLOYMENT_DIR="${DIR}/../../../deployments-routing/${concourse_env}"
  web_ip=$(bosh int "${DEPLOYMENT_DIR}/options.yml" --path /web_ip_two)
  credhub_address="https://${web_ip}:8844"
  bosh_login "${concourse_env}" > /dev/null

  unset CREDHUB_CA_CERT CREDHUB_CLIENT CREDHUB_SECRET CREDHUB_SERVER

  credhub api -s "${credhub_address}" --ca-cert=<(bosh int "${DEPLOYMENT_DIR}/credentials.yml" --path /universal_ca/ca)
  credhub login -u routing -p $(bosh int "${DEPLOYMENT_DIR}/credentials.yml" --path /routing_team_credhub_cli_password)
}
