# frozen_string_literal: true

require 'rspec'
require 'bosh/template/test'

describe 'tcp_router haproxy configs' do
  let(:release_path) { File.join(File.dirname(__FILE__), '..') }
  let(:release) { Bosh::Template::Test::ReleaseDir.new(release_path) }
  let(:tcp_router_job) { release.job('tcp_router') }
  let(:properties) do
    {
      'tcp_router' => {
        'oauth_secret' => ''
      },
      'uaa' => {
        'tls_port' => 1000
      },
      'routing_api' => {
        'mtls_port' => 1337,
        'reserved_system_component_ports' => [8080, 8081]
      }
    }
  end

  ['haproxy.conf', 'haproxy.conf.template'].each do |file|
    context "config/#{file}" do
      let(:template) { tcp_router_job.template("config/#{file}") }

      let(:haproxy_config) do
        parse_haproxy_config(template.render(properties))
      end

      context 'the non-tls health check listener' do
        let(:http_health_check) { haproxy_config['listen health_check_http_url'] }

        it 'listens on port 80' do
          expect(http_health_check).to include('mode http')
          expect(http_health_check).to include('bind :80')
          expect(http_health_check).to include('monitor-uri /health')
        end

        context 'when disabled' do
          before do
            properties['tcp_router']['enable_nontls_health_checks'] = false
          end

          it 'does not have a listener defined' do
            expect(haproxy_config).to_not have_key('listen health_check_http_url')
          end
        end

        context 'when overriding the default port' do
          before do
            properties['tcp_router']['health_check_port'] = 8080
          end

          it 'updates the bind port' do
            expect(http_health_check).to include('bind :8080')
          end
        end
      end

      context 'the tls health check listener' do
        let(:https_health_check) { haproxy_config['listen health_check_https_url'] }

        it 'is always on' do
          expect(https_health_check).to include('mode http')
          expect(https_health_check).to include('bind :443 ssl crt /var/vcap/jobs/tcp_router/config/certs/health.pem')
          expect(https_health_check).to include('monitor-uri /health')
        end

        it 'requires tls 1.2 or above' do
          expect(haproxy_config['global']).to include('ssl-default-bind-options ssl-min-ver TLSv1.2')
        end

        context 'when overriding the default port' do
          before do
            properties['tcp_router']['tls_health_check_port'] = 8443
          end

          it 'updates the bind port' do
            expect(https_health_check).to include('bind :8443 ssl crt /var/vcap/jobs/tcp_router/config/certs/health.pem')
          end
        end
      end
    end
  end
end

### The following code has been pulled from https://github.com/cloudfoundry/haproxy-boshrelease/blob/master/spec/spec_helper.rb
# converts haproxy config into hash of arrays grouped
# by top-level values eg
# {
#    "global" => [
#       "nbproc 4",
#       "daemon",
#       "stats timeout 2m"
#    ]
# }
def parse_haproxy_config(config) # rubocop:disable Metrics/AbcSize
  # remove comments and empty lines
  config = config.split("\n").reject { |l| l.empty? || l =~ /^\s*#.*$/ }.join("\n")

  # split into top-level groups
  config.split(/^([^\s].*)/).drop(1).each_slice(2).to_h do |group|
    key = group[0]
    properties = group[1]

    # remove empty lines
    properties = properties.split("\n").reject(&:empty?).join("\n")

    # split and strip leading/trailing whitespace
    properties = properties.split("\n").map(&:strip)

    [key, properties]
  end
end
