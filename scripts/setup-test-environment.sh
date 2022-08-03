#!/bin/bash
set -ex -o pipefail

bin_dir="$(dirname "$0")/../bin"
tmpdir=$(mktemp -d /tmp/cf-routing-downloads.XXXX)

echo "Some Gorouter tests require the machine to be Linux and not anything else like Windows or Mac"

# setup mysql and postgres
chown -R mysql:mysql /var/lib/mysql /var/log/mysql /var/run/mysqld
chown -R postgres:postgres /var/lib/postgresql /var/log/postgresql /var/run/postgresql /etc/postgresql
chown -R root:ssl-cert /etc/ssl/private

rsyslog_pid=$(pidof rsyslogd || true)
if [[ -z $rsyslog_pid ]]; then
  rsyslogd
else
  kill -HUP $(pidof rsyslogd)
fi
service mariadb restart
service postgresql restart

echo "Running template tests"
  # gem install bundler
  bundle install
  rubocop spec # fix these errors by running 'rubocop -a spec' or add an ignore directive
  bundle exec rspec
echo "Finished running template tests"

export BIN_DIR="$PWD/bin"
export PATH=$PATH:$BIN_DIR

pushd src/code.cloudfoundry.org
go build -o "$BIN_DIR/nats-server" github.com/nats-io/nats-server/v2
go build -o "$BIN_DIR/ginkgo" github.com/onsi/ginkgo/ginkgo
popd

echo "Done setting up for tests"
