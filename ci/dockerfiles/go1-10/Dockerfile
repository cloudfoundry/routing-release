FROM ubuntu:xenial
MAINTAINER https://github.com/cloudfoundry/routing-release

ENV DOCKER_RUBY_VERSION 2.4.4
ENV BOSH_CLI_VERSION 5.4.0
ENV GO_VERSION 1.10
ENV TERRAFORM_VERSION 0.11.4
ENV BBL_VERSION 6.9.16
ENV RUBY_INSTALLER_VERSION 0.5.0
ENV ROUTING_API_CLI_VERSION 2.9.0
ENV CF_CLI_VERSION 6.45.0
ENV POSTGRES_VERSION 9.3
ENV CREDHUB_CLI_VERSION 2.0.0

RUN set -ex \
    && echo 'mysql-server mysql-server/root_password password password' | debconf-set-selections \
    && echo 'mysql-server mysql-server/root_password_again password password' | debconf-set-selections

RUN \
      apt update && \
      apt -y install --fix-missing \
            mysql-server \
            rsyslog \
            build-essential \
            curl \
            git \
            netcat-openbsd \
            htop \
            graphviz \
            libpython-dev \
            lsof \
            psmisc \
            python \
            strace \
            libreadline6 \
            libreadline6-dev\
            wget \
            unzip \
            libmysqlclient-dev \
            libfontconfig1-dev libfreetype6 libssl-dev libpng-dev libjpeg-dev \
            jq \
            libssl-dev \
            libssl-dev/xenial \
            libssl1.0.0/xenial \
            libxml2-dev \
            libxslt-dev \
            libyaml-dev \
            openssl/xenial \
            zip \
            python-pip \
            zlib1g-dev \
      && \
      apt clean

# Install ruby-install
RUN curl https://codeload.github.com/postmodern/ruby-install/tar.gz/v${RUBY_INSTALLER_VERSION} | tar xvz -C /tmp/ && \
          cd /tmp/ruby-install-${RUBY_INSTALLER_VERSION} && \
          make install

# Install Ruby
RUN ruby-install ruby ${DOCKER_RUBY_VERSION} -- --disable-install-rdoc

# Add ruby to PATH
ENV PATH $PATH:/home/root/.gem/ruby/${DOCKER_RUBY_VERSION}/bin:/opt/rubies/ruby-${DOCKER_RUBY_VERSION}/lib/ruby/gems/${DOCKER_RUBY_VERSION}/bin:/opt/rubies/ruby-${DOCKER_RUBY_VERSION}/bin

# Install gems
RUN \
  /opt/rubies/ruby-${DOCKER_RUBY_VERSION}/bin/gem install --no-rdoc --no-ri \
    bundler \
    debugger-ruby_core_source

# Install go
RUN curl https://storage.googleapis.com/golang/go${GO_VERSION}.linux-amd64.tar.gz | tar xvz -C /usr/local/

# Add golang environment variables
ENV HOME /root
ENV GOPATH /root/go
ENV PATH $PATH:/usr/local/go/bin:/root/bin:$GOPATH/bin

# install Mercurial (for hg go dependencies)
RUN apt-get update && apt-get -y install --no-install-recommends mercurial && apt-get clean

# install common CI dependencies
RUN \
  go get \
    golang.org/x/tools/cmd/cover

# Install aws cli
RUN wget https://bootstrap.pypa.io/get-pip.py -O /tmp/get-pip.py && \
          python /tmp/get-pip.py && \
          rm /tmp/get-pip.py && \
          pip install awscli

RUN \
  curl -sL "https://packages.cloudfoundry.org/stable?release=linux64-binary&version=${CF_CLI_VERSION}&source=github-rel" | tar zxf - -C /usr/bin

# Add the PostgreSQL PGP key to verify their Debian packages.
# # It should be the same key as https://www.postgresql.org/media/keys/ACCC4CF8.asc
RUN apt-key adv --keyserver hkp://p80.pool.sks-keyservers.net:80 --recv-keys B97B0AFCAA1A47F044F244A07FCC7D46ACCC4CF8
# Add the PostgreSQL repo
RUN echo "deb http://apt.postgresql.org/pub/repos/apt/ precise-pgdg main" > /etc/apt/sources.list.d/pgdg.list

# Install postgres
RUN \
  apt-get update && \
  apt-get -y install --fix-missing \
    python-software-properties \
    software-properties-common \
    postgresql-${POSTGRES_VERSION} \
    postgresql-client-${POSTGRES_VERSION} \
    postgresql-contrib-${POSTGRES_VERSION} \
  && \
  apt-get clean

RUN sed -i 's/peer/trust/' "$(find /etc/postgresql -name pg_hba.conf)" \
    && sed -i 's/md5/trust/' "$(find /etc/postgresql -name pg_hba.conf)"

# Correct the Error: could not open temporary statistics file "/var/run/postgresql/9.3-main.pg_stat_tmp/global.tmp": No such file or directory
RUN mkdir -p /var/run/postgresql/${POSTGRES_VERSION}-main.pg_stat_tmp
RUN chown postgres.postgres /var/run/postgresql/${POSTGRES_VERSION}-main.pg_stat_tmp -R

RUN \
  wget https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip -P /tmp && \
  unzip /tmp/terraform_${TERRAFORM_VERSION}_linux_amd64.zip -d /usr/local/bin/

RUN \
  wget https://github.com/cloudfoundry/bosh-bootloader/releases/download/v${BBL_VERSION}/bbl-v${BBL_VERSION}_linux_x86-64 -P /tmp && \
  mv /tmp/bbl-* /usr/local/bin/bbl && \
  chmod +x /usr/local/bin/bbl

RUN \
  wget https://github.com/cloudfoundry-incubator/credhub-cli/releases/download/${CREDHUB_CLI_VERSION}/credhub-linux-${CREDHUB_CLI_VERSION}.tgz -P /tmp && \
  tar -xvf /tmp/credhub-linux-${CREDHUB_CLI_VERSION}.tgz -C /tmp && \
  mv /tmp/credhub /usr/local/bin/credhub && \
  chmod +x /usr/local/bin/credhub

RUN \
  wget https://github.com/cloudfoundry-incubator/routing-api-cli/releases/download/${ROUTING_API_CLI_VERSION}/rtr-linux-amd64.tgz && \
  tar -xvf rtr-linux-amd64.tgz && \
  mv rtr-linux-amd64 /usr/local/bin/rtr

RUN \
  wget https://s3.amazonaws.com/bosh-cli-artifacts/bosh-cli-${BOSH_CLI_VERSION}-linux-amd64 && \
  chmod +x bosh-cli-${BOSH_CLI_VERSION}-linux-amd64 && \
  mv bosh-cli-${BOSH_CLI_VERSION}-linux-amd64 /usr/local/bin/bosh2 && \
  cp /usr/local/bin/bosh2 /usr/local/bin/bosh

# setup mysql database for testing, then stop and clean up
RUN set -ex \
    && mkdir -p /var/lib/mysql/tmp /var/log/mysql /var/run/mysqld \
    && chown -R mysql:mysql /var/lib/mysql /var/log/mysql /var/run/mysqld \
    && chmod -R 755 /var/lib/mysql \
    && chmod -R 777 /tmp \
    && sed -i 's%/tmp%/var/lib/mysql/tmp%' /etc/mysql/my.cnf \
    && service mysql start \
    && echo 'CREATE DATABASE routing_api_test;' | mysql -u root -ppassword \
    && rm -rf /tmp/* /var/tmp/* /var/run/mysqld/* /var/lib/mysql/tmp/*

# install sshuttle and iptables
RUN \
  apt-get update && \
  apt-get -y install iptables && \
  apt-get clean && \
  pip install sshuttle

# install go dep
RUN \
    go get \
        -u github.com/golang/dep/cmd/dep

# install certstrap
RUN \
    go get \
        -u github.com/square/certstrap

# install gcloudsdk for cli & use gcloud sdk to install kubectl
RUN export CLOUD_SDK_REPO="cloud-sdk-$(lsb_release -c -s)" && \
    echo "deb http://packages.cloud.google.com/apt $CLOUD_SDK_REPO main" | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list && \
    curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add - && \
    apt-get update -y && apt-get install google-cloud-sdk -y

# install kubectl using gcloud
RUN apt-get install kubectl

# install Helm
ENV HELM_VERSION v2.11.0
ENV HELM_FILENAME helm-${HELM_VERSION}-linux-amd64.tar.gz
ENV HELM_URL https://storage.googleapis.com/kubernetes-helm/${HELM_FILENAME}

RUN echo $HELM_URL

RUN curl -o /tmp/$HELM_FILENAME ${HELM_URL} \
  && tar -zxvf /tmp/${HELM_FILENAME} -C /tmp \
  && mv /tmp/linux-amd64/helm /bin/helm \
  && rm -rf /tmp/${HELM_FILENAME}
