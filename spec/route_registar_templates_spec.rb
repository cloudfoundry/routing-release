# rubocop: disable LineLength
# rubocop: disable BlockLength
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
        ]
      }
    }
  end

  describe 'config/registrar_settings.json' do
    let(:template) { job.template('config/registrar_settings.json') }
    let(:links) do
      [
        Bosh::Template::Test::Link.new(
          name: 'nats',
          properties: {
            'nats' => {
              'host' => '', 'user' => '', 'password' => '', 'port' => 8080
            }
          }
        )
      ]
    end

    describe 'when given a valid set of properties' do
      it 'renders the template' do
        rendered_hash = JSON.parse(template.render(merged_manifest_properties, consumes: links))
        expect(rendered_hash).to eq(
          'host' => '192.168.0.0',
          'message_bus_servers' => [],
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
            'api_url' => 'http://routing-api.service.cf.internal:3000',
            'oauth_url' => 'https://uaa.service.cf.internal:8443',
            'client_id' => 'routing_api_client'
          }
        )
      end
    end

    describe 'when tls is enabled and the san is not provided' do
      before do
        merged_manifest_properties['route_registrar']['routes'][0].delete('server_cert_domain_san')
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
      end

      it 'renders the template' do
        expect { template.render(merged_manifest_properties, consumes: links) }.not_to raise_error
      end
    end
  end
end
