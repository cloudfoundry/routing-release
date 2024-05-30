# frozen_string_literal: true

require 'rspec'
require 'bosh/template/test'
require 'yaml'
require 'json'

describe 'tcp_router' do
  let(:release_path) { File.join(File.dirname(__FILE__), '..') }
  let(:release) { Bosh::Template::Test::ReleaseDir.new(release_path) }
  let(:job) { release.job('tcp_router') }

  let(:merged_manifest_properties) do
    {
      'tcp_router' => {
        'oauth_secret' => '',
      },
      'uaa' => {
        'tls_port' => 1000
      },
      'routing_api' => {}
    }
  end

  describe 'config/certs/health.pem' do
    let(:template) { job.template('config/certs/health.pem') }
    let(:links) do
      [
        Bosh::Template::Test::Link.new(
          name: 'routing_api',
          properties: {
            'routing_api' => {
              'mtls_client_cert' => 'the mtls client cert from link'
            }
          }
        )
      ]
    end

    before do
      merged_manifest_properties['tcp_router']['tls_health_check_key'] = 'tls health check key'
      merged_manifest_properties['tcp_router']['tls_health_check_cert'] = 'tls health check cert'
    end

    it 'should render the key + cert in a pem file' do
      rendered_template = template.render(merged_manifest_properties, consumes: links)
      expect(rendered_template).to eq("tls health check key\ntls health check cert\n")
    end

    describe 'when tls_health_check_key is not provided' do
      before do
        merged_manifest_properties['tcp_router'].delete('tls_health_check_key')
      end
      it 'should error' do
        expect do
          template.render(merged_manifest_properties)
        end.to raise_error(
          RuntimeError,
          "Please set tcp_router.tls_health_check_key in the tcp_router's job properties."
        )
      end
    end
    describe 'when tls_health_check_key is not provided' do
      before do
        merged_manifest_properties['tcp_router'].delete('tls_health_check_cert')
      end
      it 'should error' do
        expect do
          template.render(merged_manifest_properties)
        end.to raise_error(
          RuntimeError,
          "Please set tcp_router.tls_health_check_cert in the tcp_router's job properties."
        )
      end
    end
  end

  describe 'config/certs/tcp-router/ca_cert.crt' do
    let(:template) { job.template('config/certs/tcp-router/ca_cert.crt') }
    
    let(:links) do
      [
        Bosh::Template::Test::Link.new(
          name: 'routing_api',
          properties: {
            'routing_api' => {
              'mtls_client_cert' => 'the mtls client cert from link'
            },
            'tcp_router' => {}
          }
        )
      ]
    end

    before do
      merged_manifest_properties['tcp_router']['ca_cert'] = 'ca_cert_string'
    end

    it 'renders the client ca cert' do
      client_ca = template.render(merged_manifest_properties, consumes: links)
      expect(client_ca).to eq('ca_cert_string')
    end

    describe 'when the client ca is not provided' do # TODO: the routing_api this is based on doesn't include the ca_cert property and relies on deletion of mtls to raise a template error
      before do
        merged_manifest_properties['tcp_router'].delete('ca_cert')
      end

      it 'should err' do
        expect { template.render(merged_manifest_properties) }.to raise_error(RuntimeError, 'TCP Router server ca certificate not found in properties nor in tcp_router Link. This value can be specified using the tcp_router.ca_cert property.')
      end
    end

    describe 'when the gorouter link is present and includes the backends ca' do
      before do
        merged_manifest_properties['tcp_router'].delete('ca_cert')
      end
      let(:links) do
        [
          Bosh::Template::Test::Link.new(
            name: 'gorouter',
            properties: {
              'router' => {
                'backends' => {
                  'ca' => 'gorouter backends ca cert'
                }
              }
            }
          )
        ]
      end
      it 'renders the gorouter backends ca cert' do
        client_ca = template.render(merged_manifest_properties, consumes: links)
        expect(client_ca.strip).to eq("a ca cert\n\ngorouter backends ca cert")
      end
    end

    describe 'when the link is present and does not include the backends ca cert' do
      let(:links) do
        [
          Bosh::Template::Test::Link.new(
            name: 'gorouter',
            properties: {
              'router' => {}
            }
          )
        ]
      end
      it 'does not render the gorouter backends ca cert' do
        client_ca = template.render(merged_manifest_properties, consumes: links)
        expect(client_ca.strip).to eq('a ca cert')
      end
    end
  end

  describe 'config/certs/tcp-router/server.crt' do
    let(:template) { job.template('config/certs/tcp-router/server.crt') }

    it 'renders the server cert' do
      client_ca = template.render(merged_manifest_properties)
      expect(client_ca).to eq('the server cert')
    end

    describe 'when the server cert is not provided' do
      before do
        merged_manifest_properties['tcp_router'].delete('mtls_server_cert')
      end

      it 'should err' do
        expect { template.render(merged_manifest_properties) }.to raise_error Bosh::Template::UnknownProperty
      end
    end
  end

  describe 'config/keys/tcp-router/server.key' do
    let(:template) { job.template('config/keys/tcp-router/server.key') }

    it 'renders the server key' do
      expect(template.render(merged_manifest_properties)).to eq('the server key')
    end

    describe 'when the server key is not provided' do
      before do
        merged_manifest_properties['tcp_router'].delete('mtls_server_key')
      end

      it 'should err' do
        expect { template.render(merged_manifest_properties) }.to raise_error Bosh::Template::UnknownProperty
      end
    end
  end

  describe 'tcp_router.yml' do
    let(:template) { job.template('config/tcp_router.yml') }
    let(:links) { [] }

    subject(:rendered_config) do
      YAML.safe_load(template.render(merged_manifest_properties, consumes: links))
    end

    describe "when the client cert isn't supplied" do
      before do
        merged_manifest_properties['tcp_router'].delete('mtls_client_cert')
      end

      it 'should error so that link consumers are ensured to have the property' do
        expect { template.render(merged_manifest_properties) }.to raise_error Bosh::Template::UnknownProperty
      end
    end

    describe "when the client key isn't supplied" do
      before do
        merged_manifest_properties['tcp_router'].delete('mtls_client_key')
      end

      it 'should error so that link consumers are ensured to have the property' do
        expect { template.render(merged_manifest_properties) }.to raise_error Bosh::Template::UnknownProperty
      end
    end

    it 'renders a file with default properties' do
      expect(rendered_config).to eq('admin_port' => 15_897,
                                    'lock_ttl' => '10s',
                                    'retry_interval' => '5s',
                                    'debug_address' => '127.0.0.1:17002',
                                    'fail_on_router_port_conflicts' => false,
                                    'locket' => {
                                      'locket_address' => 'locket_server',
                                      'locket_ca_cert_file' => '/var/vcap/jobs/tcp-router/config/certs/locket/ca.crt',
                                      'locket_client_cert_file' => '/var/vcap/jobs/tcp-router/config/certs/locket/client.crt',
                                      'locket_client_key_file' => '/var/vcap/jobs/tcp-router/config/certs/locket/client.key'
                                    },
                                    'log_guid' => 'tcp_router',
                                    'max_ttl' => '120s',
                                    'metrics_reporting_interval' => '30s',
                                    'metron_config' => { 'address' => 'localhost', 'port' => 3457 },
                                    'oauth' => {
                                      'token_endpoint' => 'uaa.service.cf.internal',
                                      'port' => 8080,
                                      'skip_ssl_validation' => false
                                    },
                                    'api' => {
                                      'listen_port' => 3000,
                                      'http_enabled' => false,
                                      'mtls_listen_port' => 3001,
                                      'mtls_client_ca_file' => '/var/vcap/jobs/tcp-router/config/certs/routing-api/client_ca.crt',
                                      'mtls_server_cert_file' => '/var/vcap/jobs/tcp-router/config/certs/routing-api/server.crt',
                                      'mtls_server_key_file' => '/var/vcap/jobs/tcp-router/config/certs/routing-api/server.key'
                                    },
                                    'router_groups' => [],
                                    'reserved_system_component_ports' => [2_822, 2_825, 3_457, 3_458, 3_459, 3_460, 3_461, 8_853, 9_100, 14_726, 14_727, 14_821, 14_822, 14_823, 14_824, 14_829,
                                                                          14_830, 14_920, 14_922, 15_821, 17_002, 53_035, 53_080],
                                    'sqldb' => {
                                      'host' => 'host',
                                      'port' => 1234,
                                      'type' => 'mysql',
                                      'schema' => 'schema',
                                      'username' => 'username',
                                      'password' => 'password',
                                      'skip_hostname_validation' => false,
                                      'max_open_connections' => 201,
                                      'max_idle_connections' => 11,
                                      'connections_max_lifetime_seconds' => 3601
                                    },
                                    'statsd_client_flush_interval' => '300ms',
                                    'statsd_endpoint' => 'localhost:8125',
                                    'system_domain' => 'the.system.domain',
                                    'uuid' => 'xxxxxx-xxxxxxxx-xxxxx')
    end

    describe 'when overrideing the mTLS api listen port' do
      before do
        merged_manifest_properties['tcp_router']['mtls_port'] = 6000
      end

      it 'renders the overridden port' do
        expect(rendered_config['api']['mtls_listen_port']).to eq(6000)
      end
    end

    describe 'when overriding the api listen port' do
      before do
        merged_manifest_properties['tcp_router']['port'] = 6000
      end

      it 'renders the overridden port' do
        expect(rendered_config['api']['listen_port']).to eq(6000)
      end
    end

    describe 'when mtls is enabled' do
      before do
        merged_manifest_properties['tcp_router']['enabled_api_endpoints'] = 'mtls'
      end

      it 'enables just the mTLS API endpoint' do
        expect(rendered_config['api']['http_enabled']).to eq(false)
      end
    end

    describe 'when both are enabled' do
      before do
        merged_manifest_properties['tcp_router']['enabled_api_endpoints'] = 'both'
      end

      it 'enables the HTTP API endpoint' do
        expect(rendered_config['api']['http_enabled']).to eq(true)
      end
    end

    describe 'when an invalid api endpoints is specified' do
      before do
        merged_manifest_properties['tcp_router']['enabled_api_endpoints'] = 'junk'
      end

      it 'raises a validation error' do
        expect { template.render(merged_manifest_properties) }.to raise_error(RuntimeError, "expected tcp_router.enabled_api_endpoints to be one of 'mtls' or 'both' but got 'junk'")
      end
    end
  end

  describe 'config/certs/routing-api/client.crt' do
    let(:template) { job.template('config/certs/routing-api/client.crt') }
    let(:links) do
      [
        Bosh::Template::Test::Link.new(
          name: 'routing_api',
          properties: {
            'routing_api' => {
              'mtls_client_cert' => 'the mtls client cert from link'
            }
          }
        )
      ]
    end

    describe 'when properties and link is provided' do
      before do
        merged_manifest_properties['routing_api']['client_cert'] = 'the client cert from properties'
      end

      it 'should prefer the value in the properties' do
        rendered_template = template.render(merged_manifest_properties, consumes: links)
        expect(rendered_template).to eq('the client cert from properties')
      end
    end

    describe 'when no properties and link is provided' do
      it 'should render the value from the link' do
        rendered_template = template.render({}, consumes: links)
        expect(rendered_template).to eq('the mtls client cert from link')
      end
    end

    describe 'when properties and no link is provided' do
      before do
        merged_manifest_properties['routing_api']['client_cert'] = 'the client cert from properties'
      end

      it 'should prefer the value in the properties' do
        rendered_template = template.render(merged_manifest_properties)
        expect(rendered_template).to eq('the client cert from properties')
      end
    end

    describe 'when no properties and no link is provided' do
      it 'should error' do
        expect do
          template.render(merged_manifest_properties)
        end.to raise_error(
          RuntimeError,
          'Routing API client certificate not found in properties nor in routing_api Link. This value can be specified using the routing_api.client_cert property.'
        )
      end
    end
  end

  describe 'config/keys/routing-api/client.key' do
    let(:template) { job.template('config/keys/routing-api/client.key') }
    let(:links) do
      [
        Bosh::Template::Test::Link.new(
          name: 'routing_api',
          properties: {
            'routing_api' => {
              'mtls_client_key' => 'the mtls client key from link'
            }
          }
        )
      ]
    end

    describe 'when properties and link is provided' do
      before do
        merged_manifest_properties['routing_api']['client_private_key'] = 'the client key from properties'
      end

      it 'should prefer the value in the properties' do
        rendered_template = template.render(merged_manifest_properties, consumes: links)
        expect(rendered_template).to eq('the client key from properties')
      end
    end

    describe 'when no properties and link is provided' do
      it 'should render the value from the link' do
        rendered_template = template.render({}, consumes: links)
        expect(rendered_template).to eq('the mtls client key from link')
      end
    end

    describe 'when properties and no link is provided' do
      before do
        merged_manifest_properties['routing_api']['client_private_key'] = 'the client key from properties'
      end

      it 'should prefer the value in the properties' do
        rendered_template = template.render(merged_manifest_properties)
        expect(rendered_template).to eq('the client key from properties')
      end
    end

    describe 'when no properties and no link is provided' do
      it 'should error' do
        expect do
          template.render(merged_manifest_properties)
        end.to raise_error(
          RuntimeError,
          'Routing API client private key not found in properties nor in routing_api Link. This value can be specified using the routing_api.client_private_key property.'
        )
      end
    end
  end

  describe 'config/certs/routing-api/ca_cert.crt' do
    let(:template) { job.template('config/certs/routing-api/ca_cert.crt') }
    let(:links) do
      [
        Bosh::Template::Test::Link.new(
          name: 'routing_api',
          properties: {
            'routing_api' => {
              'mtls_ca' => 'the mtls ca from link'
            }
          }
        )
      ]
    end

    describe 'when properties and link is provided' do
      before do
        merged_manifest_properties['routing_api']['ca_cert'] = 'the ca from properties'
      end

      it 'should prefer the value in the properties' do
        rendered_template = template.render(merged_manifest_properties, consumes: links)
        expect(rendered_template).to eq('the ca from properties')
      end
    end

    describe 'when no properties and link is provided' do
      it 'should render the value from the link' do
        rendered_template = template.render({}, consumes: links)
        expect(rendered_template).to eq('the mtls ca from link')
      end
    end

    describe 'when properties and no link is provided' do
      before do
        merged_manifest_properties['routing_api']['ca_cert'] = 'the ca from properties'
      end

      it 'should prefer the value in the properties' do
        rendered_template = template.render(merged_manifest_properties)
        expect(rendered_template).to eq('the ca from properties')
      end
    end

    describe 'when no properties and no link is provided' do
      it 'should error' do
        expect do
          template.render(merged_manifest_properties)
        end.to raise_error(
          RuntimeError,
          'Routing API server ca certificate not found in properties nor in routing_api Link. This value can be specified using the routing_api.ca_cert property.'
        )
      end
    end
  end

  describe 'tcp_router.yml' do
    let(:template) { job.template('config/tcp_router.yml') }
    let(:links) do
      [
        Bosh::Template::Test::Link.new(
          name: 'routing_api',
          properties: {
            'routing_api' => {
              'mtls_port' => 1337,
              'reserved_system_component_ports' => [8080, 8081]
            }
          }
        )
      ]
    end

    subject(:rendered_config) do
      YAML.safe_load(template.render(merged_manifest_properties, consumes: links))
    end

    context 'when ips have leading 0s' do
      it 'debug_address fails with a nice message' do
        merged_manifest_properties['tcp_router']['debug_address'] = '127.0.0.01:17002'
        expect do
          rendered_config
        end.to raise_error(/Invalid tcp_router.debug_address/)
      end
    end

    it 'renders a file with default properties' do
      expect(rendered_config).to eq('isolation_segments' => [],
                                    'haproxy_pid_file' => '/var/vcap/data/tcp_router/config/haproxy.pid',
                                    'oauth' => {
                                      'token_endpoint' => 'uaa.service.cf.internal',
                                      'client_name' => 'tcp_router',
                                      'client_secret' => nil,
                                      'port' => 1000,
                                      'skip_ssl_validation' => false
                                    },
                                    'reserved_system_component_ports' => [8080, 8081],
                                    'routing_api' => {
                                      'uri' => 'https://routing-api.service.cf.internal',
                                      'port' => 1337,
                                      'auth_disabled' => false,
                                      'client_cert_path' => '/var/vcap/jobs/tcp_router/config/certs/routing-api/client.crt',
                                      'ca_cert_path' => '/var/vcap/jobs/tcp_router/config/certs/routing-api/ca_cert.crt',
                                      'client_private_key_path' => '/var/vcap/jobs/tcp_router/config/keys/routing-api/client.key'
                                    })
    end

    describe 'routing_api.reserved_system_component_ports' do
      describe 'when the property and link is defined' do
        before do
          merged_manifest_properties['reserved_system_component_ports'] = [1111]
        end

        it 'prefers the property' do
          expect(rendered_config['reserved_system_component_ports']).to eq([1111])
        end
      end

      describe 'when no property is defined and link is defined' do
        it 'prefers the link' do
          expect(rendered_config['reserved_system_component_ports']).to eq([8080, 8081])
        end
      end

      describe 'when property is defined and no link is defined' do
        before do
          merged_manifest_properties['reserved_system_component_ports'] = [1111]
        end

        it 'prefers the property' do
          expect(rendered_config['reserved_system_component_ports']).to eq([1111])
        end
      end

      describe 'when no property and no link is defined' do
        let(:links) do
          [
            Bosh::Template::Test::Link.new(
              name: 'routing_api',
              properties: {
                'routing_api' => {
                  'mtls_port' => 1337
                }
              }
            )
          ]
        end
        it 'defaults to empty' do
          expect(rendered_config['reserved_system_component_ports']).to eq([])
        end
      end
    end

    describe 'routing_api.port' do
      describe 'when the property and link is defined' do
        before do
          merged_manifest_properties['routing_api']['port'] = 1234
        end

        it 'prefers the property' do
          expect(rendered_config['routing_api']['port']).to eq(1234)
        end
      end

      describe 'when no property and link is defined' do
        it 'prefers the link' do
          expect(rendered_config['routing_api']['port']).to eq(1337)
        end
      end

      describe 'when property and no link is defined' do
        before do
          merged_manifest_properties['routing_api']['port'] = 1234
        end

        it 'prefers the property' do
          expect(rendered_config['routing_api']['port']).to eq(1234)
        end
      end

      describe 'when no property and no link is defined' do
        it 'should error' do
          expect { template.render(merged_manifest_properties) }.to raise_error(
            RuntimeError,
            'Routing API port not found in properties nor in routing_api Link. This value can be specified using the routing_api.port property.'
          )
        end
      end
    end
  end
end
