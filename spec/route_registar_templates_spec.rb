# frozen_string_literal: true

# rubocop: disable Layout/LineLength
# rubocop: disable Metrics/BlockLength
require 'rspec'
require 'bosh/template/test'
require 'yaml'
require 'json'

describe 'route_registrar' do
  let(:release_path) { File.join(File.dirname(__FILE__), '..') }
  let(:release) { Bosh::Template::Test::ReleaseDir.new(release_path) }
  let(:job) { release.job('route_registrar') }

  let(:merged_manifest_properties) do
    {
      'route_registrar' => {
        'routes' => [
          {
            'health_check' => {
              'name' => 'uaa-healthcheck',
              'script_path' => '/var/vcap/jobs/uaa/bin/health_check'
            },
            'name' => 'uaa',
            'registration_interval' => '10s',
            'tags' => {
              'component' => 'uaa'
            },
            'tls_port' => 8443, # enables tls
            'server_cert_domain_san' => 'valid_cert',
            'uris' => [
              'uaa.uaa-acceptance.cf-app.com',
              '*.login.uaa-acceptance.cf-app.com'
            ]
          }
        ],
        'routing_api' => {},
        'nats' => {
          'fail_if_using_nats_without_tls' => false
        }
      }
    }
  end

  describe 'config/routing_api/certs/client.crt' do
    let(:template) { job.template('config/routing_api/certs/client.crt') }
    let(:links) do
      [
        Bosh::Template::Test::Link.new(
          name: 'routing_api',
          properties: {
            'routing_api' => {
              'mtls_client_cert' => 'the client cert from link'
            }
          }
        )
      ]
    end
    context 'when properties and link is provided' do
      before do
        merged_manifest_properties['route_registrar']['routing_api']['client_cert'] = 'the client cert from properties'
      end

      it 'should prefer the value in the properties' do
        rendered_template = template.render(merged_manifest_properties, consumes: links)
        expect(rendered_template).to eq('the client cert from properties')
      end
    end

    context 'when no properties and link is provided' do
      it 'should render the value from the link' do
        rendered_template = template.render({}, consumes: links)
        expect(rendered_template).to eq('the client cert from link')
      end
    end

    context 'when properties and no link is provided' do
      before do
        merged_manifest_properties['route_registrar']['routing_api']['client_cert'] = 'the client cert from properties'
      end

      it 'should prefer the value in the properties' do
        rendered_template = template.render(merged_manifest_properties)
        expect(rendered_template).to eq('the client cert from properties')
      end
    end

    context 'when no properties and no link is provided' do
      it 'should not error' do
        expect do
          template.render(merged_manifest_properties)
        end.not_to raise_error
      end
    end
  end

  describe 'config/routing_api/keys/client_private.key' do
    let(:template) { job.template('config/routing_api/keys/client_private.key') }
    let(:links) do
      [
        Bosh::Template::Test::Link.new(
          name: 'routing_api',
          properties: {
            'routing_api' => {
              'mtls_client_key' => 'the client key from link'
            }
          }
        )
      ]
    end

    context 'when properties and link is provided' do
      before do
        merged_manifest_properties['route_registrar']['routing_api']['client_private_key'] = 'the client key from properties'
      end

      it 'should prefer the value in the properties' do
        rendered_template = template.render(merged_manifest_properties, consumes: links)
        expect(rendered_template).to eq('the client key from properties')
      end
    end

    context 'when no properties and link is provided' do
      it 'should render the value from the link' do
        rendered_template = template.render({}, consumes: links)
        expect(rendered_template).to eq('the client key from link')
      end
    end

    context 'when properties and no link is provided' do
      before do
        merged_manifest_properties['route_registrar']['routing_api']['client_private_key'] = 'the client key from properties'
      end

      it 'should prefer the value in the properties' do
        rendered_template = template.render(merged_manifest_properties)
        expect(rendered_template).to eq('the client key from properties')
      end
    end

    context 'when no properties and no link is provided' do
      it 'should not error' do
        expect do
          template.render(merged_manifest_properties)
        end.not_to raise_error
      end
    end
  end

  describe 'config/routing_api/certs/server_ca.crt' do
    let(:template) { job.template('config/routing_api/certs/server_ca.crt') }
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

    context 'when properties and link is provided' do
      before do
        merged_manifest_properties['route_registrar']['routing_api']['server_ca_cert'] = 'the server ca cert from properties'
      end

      it 'should prefer the value in the properties' do
        rendered_template = template.render(merged_manifest_properties, consumes: links)
        expect(rendered_template).to eq('the server ca cert from properties')
      end
    end

    context 'when no properties and link is provided' do
      it 'should render the value from the link' do
        rendered_template = template.render({}, consumes: links)
        expect(rendered_template).to eq('the mtls ca from link')
      end
    end

    context 'when properties and no link is provided' do
      before do
        merged_manifest_properties['route_registrar']['routing_api']['server_ca_cert'] = 'the server ca cert from properties'
      end

      it 'should prefer the value in the properties' do
        rendered_template = template.render(merged_manifest_properties)
        expect(rendered_template).to eq('the server ca cert from properties')
      end
    end

    context 'when no properties and no link is provided' do
      it 'should not error' do
        expect do
          template.render(merged_manifest_properties)
        end.not_to raise_error
      end
    end
  end

  describe 'config/registrar_settings.json' do
    let(:template) { job.template('config/registrar_settings.json') }
    let(:links) do
      [
        Bosh::Template::Test::Link.new(
          name: 'nats',
          properties: {
            'nats' => {
              'hostname' => 'nats-host', 'user' => 'nats-user', 'password' => 'nats-password', 'port' => 8080
            }
          },
          instances: [Bosh::Template::Test::LinkInstance.new(address: 'my-nats-address')]
        )
      ]
    end

    describe 'nats properties' do
      it 'renders with the default' do
        merged_manifest_properties['nats'] = {'fail_if_using_nats_without_tls' => false }
        rendered_hash = JSON.parse(template.render(merged_manifest_properties, consumes: links))
        expect(rendered_hash['message_bus_servers'][0]['host']).to eq('nats-host:8080')
        expect(rendered_hash['message_bus_servers'][0]['user']).to eq('nats-user')
        expect(rendered_hash['message_bus_servers'][0]['password']).to eq('nats-password')
      end
    end

    context 'when nats-tls link is present' do
      let(:links) do
        [
          Bosh::Template::Test::Link.new(
            name: 'nats',
            properties: {
              'nats' => {
                'hostname' => 'nats-host', 'user' => 'nats-user', 'password' => 'nats-password', 'port' => 8080
              }
            },
            instances: [Bosh::Template::Test::LinkInstance.new(address: 'my-nats-ip')]
          ),
          Bosh::Template::Test::Link.new(
            name: 'nats-tls',
            properties: {
              'nats' => {
                'hostname' => 'nats-tls-host', 'user' => 'nats-tls-user', 'password' => 'nats-tls-password', 'port' => 9090
              }
            },
            instances: [Bosh::Template::Test::LinkInstance.new(address: 'my-nats-tls-ip')]
          )
        ]
      end

      context 'when mTLS is enabled for NATS' do
        it 'renders with the nats-tls properties' do
          merged_manifest_properties['nats'] = { 'tls' => { 'enabled' => true } }

          rendered_hash = JSON.parse(template.render(merged_manifest_properties, consumes: links))
          expect(rendered_hash['nats_mtls_config']['enabled']).to be true
          expect(rendered_hash['message_bus_servers'].length).to eq(1)
          expect(rendered_hash['message_bus_servers'][0]['host']).to eq('nats-tls-host:9090')
          expect(rendered_hash['message_bus_servers'][0]['user']).to eq('nats-tls-user')
          expect(rendered_hash['message_bus_servers'][0]['password']).to eq('nats-tls-password')
        end
      end

      context 'when mTLS is not enabled for NATS' do
        context 'when nats.fail_if_using_nats_without_tls is false' do
          it 'renders with the default nat properties' do
            merged_manifest_properties['nats'] = {'fail_if_using_nats_without_tls' => false }
            rendered_hash = JSON.parse(template.render(merged_manifest_properties, consumes: links))
            expect(rendered_hash['nats_mtls_config']['enabled']).to be false
            expect(rendered_hash['message_bus_servers'].length).to eq(1)
            expect(rendered_hash['message_bus_servers'][0]['host']).to eq('nats-host:8080')
            expect(rendered_hash['message_bus_servers'][0]['user']).to eq('nats-user')
            expect(rendered_hash['message_bus_servers'][0]['password']).to eq('nats-password')
          end
        end
        context 'when nats.fail_if_using_nats_without_tls is true' do
          it 'fails' do
            nats_err_msg = <<-TEXT
Using nats (instead of nats-tls) is deprecated. The nats process will
be removed soon. Please migrate to using nats-tls as soon as possible.
If you must continue using nats for a short time you can set the
nats.fail_if_using_nats_without_tls property on route_registrar to
false.
TEXT
            merged_manifest_properties['nats'] = {'fail_if_using_nats_without_tls' => true }
            expect { template.render(merged_manifest_properties, consumes: links) }.to raise_error(
              RuntimeError, nats_err_msg
            )
          end
        end
      end
    end

    context 'when nats-tls link is present with mTLS authentication only' do
      let(:links) do
        [
          Bosh::Template::Test::Link.new(
            name: 'nats-tls',
            properties: {
              'nats' => {
                'hostname' => 'nats-tls-host', 'port' => 9090
              }
            },
            instances: [Bosh::Template::Test::LinkInstance.new(address: 'my-nats-tls-ip')]
          )
        ]
      end

      context 'when mTLS is enabled for NATS' do
        it 'renders with the nats-tls properties without password authentication' do
          merged_manifest_properties['nats'] = { 'tls' => { 'enabled' => true } }

          rendered_hash = JSON.parse(template.render(merged_manifest_properties, consumes: links))
          expect(rendered_hash['nats_mtls_config']['enabled']).to be true
          expect(rendered_hash['message_bus_servers'].length).to eq(1)
          expect(rendered_hash['message_bus_servers'][0]['host']).to eq('nats-tls-host:9090')
          expect(rendered_hash['message_bus_servers'][0]['user']).to be_nil
          expect(rendered_hash['message_bus_servers'][0]['password']).to be_nil
        end
      end
    end

    describe 'routing_api' do
      context 'when routing_api is mtls only' do
        let(:links) do
          [
            Bosh::Template::Test::Link.new(
              name: 'routing_api',
              properties: {
                'routing_api' => {
                  'enabled_api_endpoints' => 'mtls'
                }
              }
            ),
            Bosh::Template::Test::Link.new(
              name: 'nats-tls',
              properties: {
                'nats' => {
                  'hostname' => 'nats-tls-host', 'user' => 'nats-tls-user', 'password' => 'nats-tls-password', 'port' => 9090
                }
              },
              instances: [Bosh::Template::Test::LinkInstance.new(address: 'my-nats-tls-ip')]
            )
          ]
        end
        before do
          merged_manifest_properties['nats'] = { 'tls' => { 'enabled' => true } }
        end
        context 'when routing_api_url is not provided' do
          it 'renders with the default' do
            rendered_hash = JSON.parse(template.render(merged_manifest_properties, consumes: links))
            expect(rendered_hash['routing_api']['api_url']).to eq('https://routing-api.service.cf.internal:3001')
          end
        end
        context 'when routing_api_url is provided' do
          it 'rejects plaintext urls' do
            merged_manifest_properties['route_registrar']['routing_api']['api_url'] = 'http://routing-api.service.cf.internal:3001'
            expect { template.render(merged_manifest_properties, consumes: links) }.to raise_error(
              RuntimeError, 'expected route_registrar.routing_api.api_url to be https when routing_api.enabled_api_endpoints is mtls only'
            )
          end
          it 'uses configured url' do
            merged_manifest_properties['route_registrar']['routing_api']['api_url'] = 'https://other-routing-api.service.cf.internal:3001'
            rendered_hash = JSON.parse(template.render(merged_manifest_properties, consumes: links))
            expect(rendered_hash['routing_api']['api_url']).to eq('https://other-routing-api.service.cf.internal:3001')
          end
        end

        context 'when max_ttl is not provided in the link' do
          it 'renders with the default' do
            rendered_hash = JSON.parse(template.render(merged_manifest_properties, consumes: links))
            expect(rendered_hash['routing_api']['max_ttl']).to eq('120s')
          end
        end
        context 'when max_ttl is provided in the link' do
          it 'uses the link value' do
            links[0].properties['routing_api']['max_ttl'] = '100s'
            rendered_hash = JSON.parse(template.render(merged_manifest_properties, consumes: links))
            expect(rendered_hash['routing_api']['max_ttl']).to eq('100s')
          end
        end
      end
    end

    describe 'when given a valid set of properties' do
      it 'renders the template' do
        merged_manifest_properties['nats'] = {'fail_if_using_nats_without_tls' => false }
        rendered_hash = JSON.parse(template.render(merged_manifest_properties, consumes: links))
        expect(rendered_hash).to eq(
          'host' => '192.168.0.0',
          'message_bus_servers' => [{ 'host' => 'nats-host:8080', 'password' => 'nats-password', 'user' => 'nats-user' }],
          'routes' => [
            {
              'health_check' => { 'name' => 'uaa-healthcheck', 'script_path' => '/var/vcap/jobs/uaa/bin/health_check' },
              'name' => 'uaa',
              'registration_interval' => '10s',
              'tags' => { 'component' => 'uaa' },
              'tls_port' => 8443,
              'server_cert_domain_san' => 'valid_cert',
              'uris' => ['uaa.uaa-acceptance.cf-app.com', '*.login.uaa-acceptance.cf-app.com']
            }
          ],
          'routing_api' => {
            'ca_certs' => '/var/vcap/jobs/route_registrar/config/certs/ca.crt',
            'api_url' => 'https://routing-api.service.cf.internal:3001',
            'oauth_url' => 'https://uaa.service.cf.internal:8443',
            'client_id' => 'routing_api_client',
            'skip_ssl_validation' => false,
            'client_cert_path' => '/var/vcap/jobs/route_registrar/config/routing_api/certs/client.crt',
            'client_private_key_path' => '/var/vcap/jobs/route_registrar/config/routing_api/keys/client_private.key',
            'server_ca_cert_path' => '/var/vcap/jobs/route_registrar/config/routing_api/certs/server_ca.crt',
            'max_ttl' => '120s'
          },
          'nats_mtls_config' => {
            'enabled' => false,
            'cert_path' => '/var/vcap/jobs/route_registrar/config/nats/certs/client.crt',
            'key_path' => '/var/vcap/jobs/route_registrar/config/nats/certs/client_private.key',
            'ca_path' => '/var/vcap/jobs/route_registrar/config/nats/certs/server_ca.crt'
          }
        )
      end
    end

    describe 'when skip_ssl_validation is enabled' do
      before do
        merged_manifest_properties['route_registrar']['routing_api'] = { 'skip_ssl_validation' => true }
        merged_manifest_properties['nats'] = {'fail_if_using_nats_without_tls' => false }
      end

      it 'renders skip_ssl_validation as true' do
        rendered_hash = JSON.parse(template.render(merged_manifest_properties, consumes: links))
        expect(rendered_hash['routing_api']['skip_ssl_validation']).to be true
      end
    end

    describe 'when tls is enabled and the san is not provided' do
      before do
        merged_manifest_properties['route_registrar']['routes'][0].delete('server_cert_domain_san')
        merged_manifest_properties['nats'] = {'fail_if_using_nats_without_tls' => false }
      end
      it 'should required san if tls_port is provided' do
        expect { template.render(merged_manifest_properties, consumes: links) }.to raise_error(
          RuntimeError, 'expected route_registrar.routes[0].route.server_cert_domain_san when tls_port is provided'
        )
      end
    end

    describe 'when tls is enabled and the san is not provided' do
      before do
        merged_manifest_properties['route_registrar']['routes'][0]['server_cert_domain_san'] = ''
        merged_manifest_properties['nats'] = {'fail_if_using_nats_without_tls' => false }
      end
      it 'should required san if tls_port is provided' do
        expect { template.render(merged_manifest_properties, consumes: links) }.to raise_error(
          RuntimeError, 'expected route_registrar.routes[0].route.server_cert_domain_san when tls_port is provided'
        )
      end
    end

    describe 'when tls is not enabled and the san is not provided' do
      before do
        merged_manifest_properties['route_registrar']['routes'][0].delete('tls_port')
        merged_manifest_properties['route_registrar']['routes'][0].delete('server_cert_domain_san')
        merged_manifest_properties['nats'] = {'fail_if_using_nats_without_tls' => false }
      end

      it 'renders the template' do
        expect { template.render(merged_manifest_properties, consumes: links) }.not_to raise_error
      end
    end

    describe 'when protocol is provided' do
      before do
        merged_manifest_properties['nats'] = {'fail_if_using_nats_without_tls' => false }
      end

      it 'uses configured protocol http1' do
        merged_manifest_properties['route_registrar']['routes'][0]['protocol'] = 'http1'
        rendered_hash = JSON.parse(template.render(merged_manifest_properties, consumes: links))
        expect(rendered_hash['routes'][0]['protocol']).to eq('http1')
      end
      it 'uses configured protocol http2' do
        merged_manifest_properties['route_registrar']['routes'][0]['protocol'] = 'http2'
        rendered_hash = JSON.parse(template.render(merged_manifest_properties, consumes: links))
        expect(rendered_hash['routes'][0]['protocol']).to eq('http2')
      end
      it 'raises error for invalid protocol' do
        merged_manifest_properties['route_registrar']['routes'][0]['protocol'] = 'abc'
        expect { template.render(merged_manifest_properties, consumes: links) }.to raise_error(
          RuntimeError, 'expected route_registrar.routes[0].route.protocol to be http1 or http2 when protocol is provided'
        )
      end
    end
  end
end

# rubocop: enable Layout/LineLength
# rubocop: enable Metrics/BlockLength
