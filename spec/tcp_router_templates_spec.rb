# rubocop: disable LineLength
# rubocop: disable BlockLength
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
      'routing_api' => {
        'client_cert' => 'the client cert',
        'client_private_key' => 'the client key',
        'ca_cert' => 'the ca cert'
      }
    }
  end

  describe 'config/certs/routing-api/client.crt' do
    let(:template) { job.template('config/certs/routing-api/client.crt') }

    it 'renders the client cert' do
      client_ca = template.render(merged_manifest_properties)
      expect(client_ca).to eq('the client cert')
    end

    describe 'when the client cert is not provided' do
      it 'should not error' do
        merged_manifest_properties['routing_api']['client_cert'] = nil
        expect { template.render(merged_manifest_properties) }.to raise_error Bosh::Template::UnknownProperty
      end
    end
  end

  describe 'config/keys/routing-api/client.key' do
    let(:template) { job.template('config/keys/routing-api/client.key') }

    it 'renders the private client key' do
      client_ca = template.render(merged_manifest_properties)
      expect(client_ca).to eq('the client key')
    end

    describe 'when the private client key is not provided' do
      it 'should err' do
        merged_manifest_properties['routing_api']['client_private_key'] = nil
        expect { template.render(merged_manifest_properties) }.to raise_error Bosh::Template::UnknownProperty
      end
    end
  end

  describe 'config/certs/routing-api/ca_cert.crt' do
    let(:template) { job.template('config/certs/routing-api/ca_cert.crt') }

    it 'renders the client cert' do
      client_ca = template.render(merged_manifest_properties)
      expect(client_ca).to eq('the ca cert')
    end

    describe 'when the ca cert is not provided' do
      it 'should err' do
        merged_manifest_properties['routing_api']['ca_cert'] = nil
        expect { template.render(merged_manifest_properties) }.to raise_error Bosh::Template::UnknownProperty
      end
    end
  end

  describe 'tcp_router.yml' do
    let(:template) { job.template('config/tcp_router.yml') }

    subject(:rendered_config) do
      YAML.safe_load(template.render(merged_manifest_properties))
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
                                    'routing_api' => {
                                      'uri' => 'https://routing-api.service.cf.internal',
                                      'port' => 3001,
                                      'auth_disabled' => false,
                                      'client_cert_path' => '/var/vcap/jobs/tcp_router/config/certs/routing-api/client.crt',
                                      'ca_cert_path' => '/var/vcap/jobs/tcp_router/config/certs/routing-api/ca_cert.crt',
                                      'client_private_key_path' => '/var/vcap/jobs/tcp_router/config/keys/routing-api/client.key'
                                    })
    end
  end
end
