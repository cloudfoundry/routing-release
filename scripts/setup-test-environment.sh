#!/bin/bash
set -ex -o pipefail

function bootDB {
  db="$1"

  if [ "${db}" = "postgres" ]; then
    launchDB="(/postgres-entrypoint.sh postgres &> /var/log/postgres-boot.log) &"
    testConnection="psql -h localhost -U postgres -c '\conninfo'"
  elif [ "${db}" = "mysql" ]  || [ "${db}" = "mysql-5.6" ] || [ "${db}" = "mysql8" ]; then
    launchDB="(MYSQL_ROOT_PASSWORD=password /mysql-entrypoint.sh mysqld &> /var/log/mysql-boot.log) &"
    testConnection="mysql -h localhost -u root -D mysql -e '\s;' --password='password'"
  else
    echo "DB variable not set. The script does not determine which database to use and would fail some tests with errors related to being unable to connect to the db. Bailing early."
    exit 1
  fi

  echo -n "booting ${db}"
  eval "$launchDB"
  for _ in $(seq 1 60); do
    if eval "${testConnection}" &> /dev/null; then
      echo "connection established to ${db}"
      return 0
    fi
    echo -n "."
    sleep 1
  done
  eval "${testConnection}" || true
  echo "unable to connect to ${db}"
  exit 1
}

echo "Some Gorouter tests require the machine to be Linux and not anything else like Windows or Mac"

rsyslog_pid=$(pidof rsyslogd || true)
if [[ -z "${rsyslog_pid}" ]]; then
  rsyslogd
else
  kill -HUP "$(pidof rsyslogd)"
fi

export BIN_DIR="${PWD}/bin"
export PATH="${PATH}:${BIN_DIR}"

pushd src/code.cloudfoundry.org
go build -o "${BIN_DIR}/nats-server" github.com/nats-io/nats-server/v2
go build -o "${BIN_DIR}/ginkgo" github.com/onsi/ginkgo/v2/ginkgo
popd

bootDB "${DB}"
echo "Done setting up for tests"
