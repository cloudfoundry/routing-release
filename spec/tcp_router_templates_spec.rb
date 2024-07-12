# frozen_string_literal: true

require 'rspec'
require 'bosh/template/test'
require 'yaml'
require 'json'

describe 'tcp_router' do
  let(:release_path) { File.join(File.dirname(__FILE__), '..') }
  let(:release) { Bosh::Template::Test::ReleaseDir.new(release_path) }
  let(:job) { release.job('tcp_router') }
  let(:backend_tls) { {} }

  let(:merged_manifest_properties) do
    {
      'tcp_router' => {
        'oauth_secret' => '',
        'backend_tls' => backend_tls,
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

  describe 'config/keys/tcp-router/client_cert_and_key.pem' do
    let(:template) { job.template('config/keys/tcp-router/backend/client_cert_and_key.pem') }

    it 'renders the client cert + key in a single PEM file' do
      merged_manifest_properties['tcp_router']['backend_tls']['client_cert'] = "the backend client cert"
      merged_manifest_properties['tcp_router']['backend_tls']['client_key'] = "the backend client key"
      client_pem = template.render(merged_manifest_properties)
      expect(client_pem).to eq("the backend client cert\nthe backend client key")
    end
  end

  describe 'config/certs/tcp-router/ca.crt' do
    let(:template) { job.template('config/certs/tcp-router/backend/ca.crt') }

    it 'renders the ca.crt' do
      merged_manifest_properties['tcp_router']['backend_tls']['ca_cert'] = "the backend ca cert"
      expect(template.render(merged_manifest_properties)).to eq('the backend ca cert')
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
                                    'backend_tls' => { 'enabled' => false },
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

    describe 'tcp_router.backend_tls' do
      describe 'when disabled' do
        let :backend_tls do
          {
            'enabled' => false,
            'ca_cert' => 'meowca',
            'client_cert' => 'meowcert',
            'client_key' => 'meowkey',
          }
        end

        it 'does not set the CA path or client cert/key path' do
            expect(rendered_config['backend_tls']).to eq({
              'enabled' => false,
            })
        end
      end
      describe 'when enabled' do
        describe 'when CA is a whitespace-only string' do
          let :backend_tls do
            {
              'enabled' => true,
              'ca_cert' => ' ',
            }
          end
          it 'throws an error' do
              expect { rendered_config }.to raise_error(
                RuntimeError,
                'tcp_router.backend_tls.enabled was set to true, but tcp_router.backend_tls.ca_cert was not provided',
              )
          end
        end
        describe 'when CA is not provided' do
          let :backend_tls do
            {
              'enabled' => true,
            }
          end
          it 'throws an error' do
              expect { rendered_config }.to raise_error(
                RuntimeError,
                'tcp_router.backend_tls.enabled was set to true, but tcp_router.backend_tls.ca_cert was not provided',
              )
          end
        end


        describe 'when a CA is provided' do
          let :backend_tls do
            {
              'enabled' => true,
              'ca_cert' => 'ca cert',
            }
          end

          it 'renders the backend_tls properties' do
            expect(rendered_config['backend_tls']).to eq({
              'enabled' => true,
              'ca_cert_path' => '/var/vcap/jobs/tcp_router/config/certs/tcp-router/backend/ca.crt',
            })
          end

          describe 'when client cert/keys are provided' do
            let :backend_tls do
              {
                'enabled' => true,
                'ca_cert' => 'ca cert',
                'client_cert' => 'client cert',
                'client_key' =>'client key',
              }
            end

            it 'renders the backend_tls properties' do
              expect(rendered_config['backend_tls']).to eq({
                'enabled' => true,
                'ca_cert_path' => '/var/vcap/jobs/tcp_router/config/certs/tcp-router/backend/ca.crt',
                'client_cert_and_key_path' => '/var/vcap/jobs/tcp_router/config/keys/tcp-router/backend/client_cert_and_key.pem',
              })
            end
          end

          describe 'when a client cert is provided but not a key' do
            let :backend_tls do
              {
                'enabled' => true,
                'ca_cert' => 'ca cert',
                'client_cert' => 'client cert',
              }
            end

            it 'throws an error' do
              expect { rendered_config }.to raise_error(
                RuntimeError,
                'tcp_router.backend_tls.enabled was set to true, tcp_router.backend_tls.client_cert was set, but tcp_router.backend_tls.client_key was not provided',
              )
            end
          end

          describe 'when client cert is provided but key is a whitespace-only string' do
            let :backend_tls do
              {
                'enabled' => true,
                'ca_cert' => 'ca cert',
                'client_cert' => 'client cert',
                'client_key' => ' ',
              }
            end

            it 'throws an error' do
              expect { rendered_config }.to raise_error(
                RuntimeError,
                'tcp_router.backend_tls.enabled was set to true, tcp_router.backend_tls.client_cert was set, but tcp_router.backend_tls.client_key was not provided',
              )
            end
          end

          describe 'when a client key is provided but not a cert' do
            let :backend_tls do
              {
                'enabled' => true,
                'ca_cert' => 'ca cert',
                'client_key' =>'client key',
              }
            end

            it 'throws an error' do
              expect { rendered_config }.to raise_error(
                RuntimeError,
                'tcp_router.backend_tls.enabled was set to true, tcp_router.backend_tls.client_key was set, but tcp_router.backend_tls.client_cert was not provided',
              )
            end
          end

          describe 'when client key is provided but cert is a whitespace-only string' do
            let :backend_tls do
              {
                'enabled' => true,
                'ca_cert' => 'ca cert',
                'client_cert' => ' ',
                'client_key' =>'client key',
              }
            end

            it 'throws an error' do
              expect { rendered_config }.to raise_error(
                RuntimeError,
                'tcp_router.backend_tls.enabled was set to true, tcp_router.backend_tls.client_key was set, but tcp_router.backend_tls.client_cert was not provided',
              )
            end
          end
        end

        describe 'when a client cert is provided but not the CA' do
            let :backend_tls do
              {
                'enabled' => true,
                'client_cert' => 'client cert',
              }
            end

            it 'throws an error' do
              expect { rendered_config }.to raise_error(
                RuntimeError,
                'tcp_router.backend_tls.enabled was set to true, but tcp_router.backend_tls.ca_cert was not provided',
              )
            end
        end
        describe 'when a client key is provided but not the CA' do
          let :backend_tls do
            {
              'enabled' => true,
              'client_key' =>'client key',
            }
          end

          it 'throws an error' do
            expect { rendered_config }.to raise_error(
              RuntimeError,
              'tcp_router.backend_tls.enabled was set to true, but tcp_router.backend_tls.ca_cert was not provided',
            )
          end
        end
        describe 'when a client key is provided but the CA is whitespace-only ' do
          let :backend_tls do
            {
              'enabled' => true,
              'client_key' =>'client key',
              'ca_cert' => ' ',
            }
          end

          it 'throws an error' do
            expect { rendered_config }.to raise_error(
              RuntimeError,
              'tcp_router.backend_tls.enabled was set to true, but tcp_router.backend_tls.ca_cert was not provided',
            )
          end
        end
      end
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
