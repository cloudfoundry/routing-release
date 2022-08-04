# frozen_string_literal: true

# rubocop: disable Layout/LineLength
# rubocop: disable Metrics/BlockLength
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
        'oauth_secret' => ''
      },
      'uaa' => {
        'tls_port' => 1000
      },
      'routing_api' => {}
    }
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
# rubocop: enable Layout/LineLength
# rubocop: enable Metrics/BlockLength
