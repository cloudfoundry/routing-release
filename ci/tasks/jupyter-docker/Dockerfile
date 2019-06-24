FROM ubuntu:trusty
MAINTAINER https://github.com/cloudfoundry-incubator/cf-routing-release

RUN \
      apt-get update && \
      apt-get -y install --fix-missing \
            build-essential \
            curl \
            git \
            htop \
            graphviz \
            lsof \
            psmisc \
            strace \
            libreadline6 \
            libreadline6-dev\
            wget \
            unzip \
            libicu52 \
            libfontconfig1-dev libfreetype6 libssl-dev libpng-dev libjpeg-dev \
            jq \
            libssl-dev \
            libssl-dev/trusty \
            libssl1.0.0/trusty \
            libxml2-dev \
            libxslt-dev \
            libyaml-dev \
            openssl/trusty \
            zip \
            zlib1g-dev \
      && \
      apt-get clean

# Install ruby-install
RUN curl https://codeload.github.com/postmodern/ruby-install/tar.gz/v0.5.0 | tar xvz -C /tmp/ && \
          cd /tmp/ruby-install-0.5.0 && \
          make install

ENV DOCKER_RUBY_VERSION 2.2.3
ENV CONDA_DIR /opt/conda
ENV PATH $CONDA_DIR/bin:$PATH

# Install conda
RUN cd /tmp && \
    mkdir -p $CONDA_DIR && \
    wget --quiet https://repo.continuum.io/miniconda/Miniconda3-4.2.12-Linux-x86_64.sh && \
    echo "c59b3dd3cad550ac7596e0d599b91e75d88826db132e4146030ef471bb434e9a *Miniconda3-4.2.12-Linux-x86_64.sh" | sha256sum -c - && \
    /bin/bash Miniconda3-4.2.12-Linux-x86_64.sh -f -b -p $CONDA_DIR && \
    rm Miniconda3-4.2.12-Linux-x86_64.sh && \
    $CONDA_DIR/bin/conda config --system --add channels conda-forge && \
    $CONDA_DIR/bin/conda config --system --set auto_update_conda false && \
    conda clean -tipsy

# Install Jupyter notebook
RUN conda install --quiet --yes \
    'notebook=4.3*' \
    jupyterhub=0.7 \
    && conda clean -tipsy

# Install Python 3 packages
# Remove pyqt and qt pulled in for matplotlib since we're only ever going to
# use notebook-friendly backends in these images
RUN conda install --quiet --yes \
    'nomkl' \
    'ipywidgets=5.2*' \
    'pandas=0.19*' \
    'numexpr=2.6*' \
    'matplotlib=1.5*' \
    'scipy=0.17*' \
    'seaborn=0.7*' \
    'scikit-learn=0.18*' \
    'scikit-image=0.11*' \
    'sympy=1.0*' \
    'cython=0.23*' \
    'patsy=0.4*' \
    'statsmodels=0.6*' \
    'cloudpickle=0.1*' \
    'dill=0.2*' \
    'numba=0.23*' \
    'bokeh=0.11*' \
    'sqlalchemy=1.0*' \
    'hdf5=1.8.17' \
    'h5py=2.6*' \
    'vincent=0.4.*' \
    'beautifulsoup4=4.5.*' \
    'xlrd'  && \
    conda remove --quiet --yes --force qt pyqt && \
    conda clean -tipsy

# Install Ruby
RUN ruby-install ruby ${DOCKER_RUBY_VERSION} -- --disable-install-rdoc

# Add ruby to PATH
ENV PATH $PATH:/home/root/.gem/ruby/${DOCKER_RUBY_VERSION}/bin:/opt/rubies/ruby-2.2.3/lib/ruby/gems/${DOCKER_RUBY_VERSION}/bin:/opt/rubies/ruby-${DOCKER_RUBY_VERSION}/bin

# Install gems
RUN \
  /opt/rubies/ruby-${DOCKER_RUBY_VERSION}/bin/gem install --no-rdoc --no-ri \
    bundler \
    bosh_cli \
    debugger-ruby_core_source

RUN wget https://bootstrap.pypa.io/get-pip.py && \
    python3 get-pip.py && \
    python3 -m pip install --upgrade pip


RUN jupyter nbextension enable --py --sys-prefix widgetsnbextension

# # Import matplotlib the first time to build the font cache
RUN MPLBACKEND=Agg $CONDA_DIR/bin/python -c "import matplotlib.pyplot"

COPY jupyter_notebook_config.py /root/.jupyter/
