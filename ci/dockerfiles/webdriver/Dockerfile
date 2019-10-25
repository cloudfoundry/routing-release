FROM cloudfoundry/cf-routing-pipeline
MAINTAINER https://github.com/cloudfoundry/routing-release

ENV CHROME_DRIVER_VERSION 2.37

# install Chrome
RUN \
  curl -sS -o - https://dl-ssl.google.com/linux/linux_signing_key.pub | apt-key add - && \
  echo "deb http://dl.google.com/linux/chrome/deb/ stable main" >> /etc/apt/sources.list.d/google-chrome.list && \
  apt-get -y update && \
  apt-get -y install google-chrome-stable

# install Chrome Driver
RUN \
  wget -N http://chromedriver.storage.googleapis.com/${CHROME_DRIVER_VERSION}/chromedriver_linux64.zip -P /tmp/chrome && \
  unzip /tmp/chrome/chromedriver_linux64.zip -d /tmp/chrome && \
  mv -f /tmp/chrome/chromedriver /usr/local/bin/chromedriver && \
  rm -rf /tmp/chrome
