require 'rspec'
require 'bosh/template/test'
require 'yaml'
require 'json'

module Bosh::Template::Test
  describe 'routing-api job template rendering' do
    let(:release_path) {File.join(File.dirname(__FILE__), '..')}
    let(:release) {ReleaseDir.new(release_path)}
    let(:job) {release.job('routing-api')}

    let(:merged_manifest_properties) do
      {
        'routing_api' => {
          'system_domain' => 'the.system.domain',
          'sqldb' => {
            'host' => 'host',
            'port' => 1234,
            'type' => 'mysql',
            'schema' => 'schema',
            'username' => 'username',
            'password' => 'password',
          }
        },
        'uaa' => {
          'tls_port' => 8080
        }
      }
    end

    subject(:rendered_config) do
      YAML.safe_load(template.render(merged_manifest_properties))
    end

    describe 'routing-api.yml' do
      let(:template) { job.template('config/routing-api.yml') }

      it 'renders a file with default properties' do
        expect(rendered_config).to eq({
          "admin_port" => 15897,
          "consul_cluster" => {"servers"=>"http://127.0.0.1:8500", "lock_ttl"=>"10s", "retry_interval"=>"5s"},
          "debug_address" => "127.0.0.1:17002",
          "locket" => {"locket_address"=>nil, "locket_ca_cert_file"=>"/var/vcap/jobs/routing-api/config/certs/locket/ca.crt", "locket_client_cert_file"=>"/var/vcap/jobs/routing-api/config/certs/locket/client.crt", "locket_client_key_file"=>"/var/vcap/jobs/routing-api/config/certs/locket/client.key"},
          "log_guid" => "routing_api",
          "max_ttl" => "120s",
          "metrics_reporting_interval" => "30s",
          "metron_config" => {"address"=>"localhost", "port"=>3457},
          "oauth" => {
            "token_endpoint"=>"uaa.service.cf.internal",
            "port"=>8080,
            "skip_ssl_validation"=>false
          },
          "router_groups" => [],
          "skip_consul_lock" => false,
          "sqldb" => {
            "host"=>"host",
            "port"=>1234,
            "type"=>"mysql",
            "schema"=>"schema",
            "username"=>"username",
            "password"=>"password",
            "skip_hostname_validation"=>false,
          },
          "statsd_client_flush_interval" => "300ms",
          "statsd_endpoint" => "localhost:8125",
          "system_domain" => "the.system.domain",
          "uuid" => "xxxxxx-xxxxxxxx-xxxxx"
        })
      end
      describe 'when the db connection should skip hostname validation' do
        before do
          merged_manifest_properties['routing_api']['sqldb']['skip_hostname_validation'] = true
        end
  
        it 'should render the yml accordingly' do
          expect(rendered_config['sqldb']['skip_hostname_validation']).to be true
        end
      end
    end
  end
end

