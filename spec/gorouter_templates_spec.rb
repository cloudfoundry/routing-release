# frozen_string_literal: true

# rubocop:disable Layout/LineLength
# rubocop:disable Metrics/BlockLength

require 'rspec'
require 'yaml'
require 'json'
require 'bosh/template/test'
require 'bosh/template/evaluation_context'
require 'spec_helper'
require 'openssl'

TEST_CERT = '-----BEGIN CERTIFICATE-----
MIIESjCCAjKgAwIBAgIRAMLNrkeAdcANSxOHGdVhsfowDQYJKoZIhvcNAQELBQAw
EjEQMA4GA1UEAxMHdGVzdC1jYTAeFw0yMTEwMjExNzA0MDFaFw0yMzA0MjExNjI5
MDVaMBwxGjAYBgNVBAMTEXRlc3Qtd2l0aC1zYW4uY29tMIIBIjANBgkqhkiG9w0B
AQEFAAOCAQ8AMIIBCgKCAQEA3q+N8Se+LMXjanIBlkHhzrcKT71C0T6iB64jvyCJ
oQ0Z63M7pRs7h1YZV37KJCE3/QuIt6Atw/EA88/yIvSxWw9ytVQntzqtcKambC3b
8qGWxpF9piktyzZjpXJvTIWrYYyCOlZM1QkJ976O76+yoZM2Ttp36n1OqIX2DpEt
XJ9/VoMDBhQ/TvEAUdEUP0GFrBrUP7WoSLOjRnEn8gPvuGMQ7QDjx+EWScAaDz3c
R3X7UGa5w7+RdcZ6zhKlftg7D1+XMgCelsZjxZjEECNF7p/YhaSLhgKN/XZ5CtEt
5sa1EVSQmiIb715B8ee8BjwUEzD9VteYdCaH6YivoeDyzQIDAQABo4GQMIGNMA4G
A1UdDwEB/wQEAwIDuDAdBgNVHSUEFjAUBggrBgEFBQcDAQYIKwYBBQUHAwIwHQYD
VR0OBBYEFCtWb9SZGcuTEmthC8enxyYwHbXSMB8GA1UdIwQYMBaAFBIf8JzVENnJ
GH272x8d5Ld5ZjNMMBwGA1UdEQQVMBOCEXRlc3Qtd2l0aC1zYW4uY29tMA0GCSqG
SIb3DQEBCwUAA4ICAQDIwIxeB5F1DC48OtDiHj2pbX0O7IsWwax6SAlY+j0taQuy
EMDuBWYXw1sDdnTHY+AytymRd8KFNdCzzsZhflLwp+iZ9zb81xS7IfdOo3KV6dc/
zEtaU0B2aP1Q7yfdl9TwZ0FNoSf0AZYLizr85KcW1LStWypiegY/7CcuwrUnXiZB
Lg8/YM5BTd2rZIgnid4d2fvp2KgcU1ztiCCJVGkty/LKtwwJxrjvuwGxjJVWRcjq
l1VObuX8HYHufn62EW3L1WL5TMYd5t34eXo1KAjv+FGqD280SjwFFaaOZ5qfYkx1
wcItuinnx6m2TtSB8Rj/QFdItLVhEOTxoPbmMi0iVw/fYEcqUBn4OIDPBZbKzlcU
jizmjv8waQlFgZbLKZBDYht3+x45k9+IWViLl5IPM4I4cVj9kYRUr0GOlPxBYRkW
0evndFjeCka24cjdW1/b7NHq9uCRDj/Px+i0oUfvEAVQU94N/Pir3nuUIKpkx/TQ
A1xXeONZVuGuarQmcRN9gCC3FUbnkh1lUO4qgFE8iIKnOtFeUnMdiBcWPmRaOJRI
BdgLIJDrTJStUc4OcZSE6gBkHAt0SAtST7BcLyholehyvheFw4nWUOEvEs1p/bkY
NexOrpDV8Ump01u0IPyZZv/LNNaWX1wpxbjusVYZCxCfTO2d7s/VQSdRsyH5Hg==
-----END CERTIFICATE-----'

TEST_CERT2 = '-----BEGIN CERTIFICATE-----
MIIESzCCAjOgAwIBAgIQDnaPUSkJl2T+TaMLHUlWqzANBgkqhkiG9w0BAQsFADAS
MRAwDgYDVQQDEwd0ZXN0LWNhMB4XDTIxMTAyMTIwMDIzMloXDTIzMDQyMTE2Mjkw
NVowHTEbMBkGA1UEAxMSdGVzdDItd2l0aC1zYW4uY29tMIIBIjANBgkqhkiG9w0B
AQEFAAOCAQ8AMIIBCgKCAQEAwc8fvFNfGF1SqVs7UOTwYbQCv18wF+EfJYT4tq3P
1MLBuW7eURKnJ4ZAslsogX4WXmksYHnjRbIsQw6mtgAkMtkC+C5tuRO5uaEBSFxP
vA9z7b9uM9MGA2YSJVP1+U00y/HCrwI5LEc/SGij3bvKOcs+CUAEmHhr3sG95BTF
atVE5vbG+XHLw2DwaWzDFrtPG3o9zBtDb7/yqTNJCb+i7iyp9Yh0N2ZHgKjNI8Ru
6rEkQz8nYk44NjCwV5l0fKV3eKLXTRyfEb+Gr1RHfTtG7wRvfDcDanS5XTZQWAAr
a61V38xfR+bYniYsSLmH/VZ1CAhqY8t45N9Sc47cH6gOHQIDAQABo4GRMIGOMA4G
A1UdDwEB/wQEAwIDuDAdBgNVHSUEFjAUBggrBgEFBQcDAQYIKwYBBQUHAwIwHQYD
VR0OBBYEFMl9dHRf9wJoWyuWNg93QldFc5IKMB8GA1UdIwQYMBaAFBIf8JzVENnJ
GH272x8d5Ld5ZjNMMB0GA1UdEQQWMBSCEnRlc3QyLXdpdGgtc2FuLmNvbTANBgkq
hkiG9w0BAQsFAAOCAgEADQFp7nPDLMRPbwWo1fRj7jF6XC85qJF5F9hMRyuioxu9
ZHnlejEpi8o41/NRCV0lPLIAXe9/0owppZF3WhqF7eYUFa1YxFr+BkQg3r4nAq9g
UKkT2qB/AmJA72HYGGYbb1OdxccRxbgh1+nWzEkxFzB7HbiIrY7OqmDFLr7JRAeF
kaH27wLnZ2TJ2MCDQZNM8n12+3szwytZDl8uz6Dl5W7L4HcK6KIROhvjA6s3lAvA
3E2VJ07bkAvMcXGX0jobcTVDB/+WVvVoZym0TfmMUVQ0JD6vFe8sNdsJysWCsUe9
DbE9ZRA+3GaURVlpZ89n4sURVIopP+N51Vs6aZAIZdOuOvFwu+7N82LjI4i8800c
P90vC+M77jSGmu7Cuehu62Q0aUIx+X98TXQXpb4KLQ5Ot5RIPV0E3ksmKuqIrwuO
m5tSO2hX/BIkj160bikWbi3oqU8+91+jeW9fQnRLApkPwLWXSViF1Q7K8c6+/H/p
oyX7VxkqnU43+nzL+Egc9ibYRF22XMkCBICZFrPu3rZbz8zHTw43ldqHezvx+O/J
R5DG1U9dcbt9urELWUBEWlrDudlyC1p6ZvMYQIHP2e27pUaU6wFy7xnIrTxYbDM6
HTngE/Gz+qIUe7OkPXPPkFeoSfR1poQ3yNz4bim9Vx+w50l6m+h6SZOYJTxEUds=
-----END CERTIFICATE-----'

TEST_KEY = 'some


multi line key'

CERT_WITHOUT_CN = '-----BEGIN CERTIFICATE-----
MIIEITCCAgmgAwIBAgIRAMGCNmHhXZnK1fSdCinKK9owDQYJKoZIhvcNAQELBQAw
EjEQMA4GA1UEAxMHdGVzdC1jYTAeFw0yMTEwMjExNjU2NTJaFw0yMzA0MjExNjI5
MDVaMBMxETAPBgNVBAMTCHRlc3QuY29tMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEAwY9FO90qNGnztTlPSUODTLvdKex08dA+/hQ2URMBStqI5g6dJZP9
RcLVyRpp9719KKs2PL2ol/QEfUMXKSB1pld6kRGFEXbPkz8rxLhYt79UzjAC8lWj
z/NbyIvNVzqgYlB7Tk+sgIBF3LSV3Zh4ZsrNoXMu/VDG+ODm/1dcLZJE3QXaMM6Z
nbvdy/eUOhJ12BzgM+1PKjNi93azOB6uBiXZ1QgzWbmWJHnGmvX/HUdT8s4e1snt
5mAsS7hmsrxpu2QD9b3gGUIgy6z6ZuFp1kq0S5HxoFDNjvi88p2E4Jk+unfFMaO9
4+OyOZWW5TqyyhTYCrhBEcZ4m5hm82v76wIDAQABo3EwbzAOBgNVHQ8BAf8EBAMC
A7gwHQYDVR0lBBYwFAYIKwYBBQUHAwEGCCsGAQUFBwMCMB0GA1UdDgQWBBRZ7D+U
LkHi0vbszx8bMG2LZSqUejAfBgNVHSMEGDAWgBQSH/Cc1RDZyRh9u9sfHeS3eWYz
TDANBgkqhkiG9w0BAQsFAAOCAgEA1YluE0iSE4HEc2N2fdYhmwF2LP3pjUfmzF/g
NcxjhydQUoxyOxf6+1RsNe7taXQRLhmpN2JaiE8yCf+wDciIhRWnqyHgJEKoJgK6
4liu7JUpOFgAloe8koKhWxEerkU4VcPy8kN5gZ8I6b8Mso4hTq2O5NhntqKDFRS0
v0ZpMkz1PhWwI79No8WXU0tUwx5pT3mcwjCr57mnyYWmeHqAXgnUI4U0QnSyr3sa
jmjpLk2TncpC3CSTr1AbOhm/yglsrbLllvufHUbYv5QNlzkOauvgCzvXQ4ScFttn
epDzPE8PrsY8N/26BwOCc6ftQqabhpIKzT6w6DN5xYRZi5fyzRNho5+5RuBDRKmL
AGfrpiixm4zzgUL7jVlOVlZXQ/vkQ+h4+aqS2ssRwPoqGxilFxfUMgO+hr3jZkxz
o9Z7Yeljt7rzeYESEDtkwou+75LHzfKduVT8Kxwn8LwiB0trgbcx3qj2ab8fucM4
UUXAXr6ve5DcdkKevLoNypq2kCh7hySjrjDp/gnCMhuc0ch8oV2RV2ZlA+QOD+J4
VAgYLhy03ZZaUFvmGhCx+FEkkzq/d2GGWuNd1T2MMkTBplf+pK+3l+jHxYuSc8DR
gPYhs8i50bWlTVu/yJgJGBzAmWcybfi7NmUkQyYHmpLP3GRbtdI+eESF9vAJpKSs
ONppgXo=
-----END CERTIFICATE----- '

ROUTE_SERVICES_CLIENT_TEST_CERT = 'route services

multiline

cert'

ROUTE_SERVICES_CLIENT_TEST_KEY = 'route services

multi line key'

describe 'gorouter' do
  let(:release_path) { File.join(File.dirname(__FILE__), '..') }
  let(:release) { Bosh::Template::Test::ReleaseDir.new(release_path) }
  let(:job) { release.job('gorouter') }

  describe 'gorouter.yml.erb' do
    let(:deployment_manifest_fragment) do
      {
        'router' => {
          'status' => {
            'port' => 80,
            'user' => 'test',
            'password' => 'pass'
          },
          'enable_ssl' => true,
          'tls_port' => 443,
          'client_cert_validation' => 'none',
          'logging_level' => 'info',
          'tracing' => {
            'enable_zipkin' => false,
            'enable_w3c' => false,
            'w3c_tenant_id' => nil
          },
          'ssl_skip_validation' => false,
          'port' => 80,
          'offset' => 0,
          'number_of_cpus' => 0,
          'trace_key' => 'key',
          'debug_address' => '127.0.0.1',
          'secure_cookies' => false,
          'write_access_logs_locally' => true,
          'access_log' => {
            'enable_streaming' => false
          },
          'drain_wait' => 10,
          'drain_timeout' => 300,
          'healthcheck_user_agent' => 'test-agent',
          'requested_route_registration_interval_in_seconds' => 10,
          'load_balancer_healthy_threshold' => 10,
          'balancing_algorithm' => 'round-robin',
          'disable_log_forwarded_for' => true,
          'disable_log_source_ip' => true,
          'tls_pem' => [
            {
              'cert_chain' => TEST_CERT,
              'private_key' => 'test-key'
            },
            {
              'cert_chain' => TEST_CERT2,
              'private_key' => 'test-key2'
            }
          ],
          'min_tls_version' => 'TLSv1.2',
          'max_tls_version' => 'TLSv1.2',
          'disable_http' => false,
          'ca_certs' => [TEST_CERT],
          'cipher_suites' => 'test-suites',
          'forwarded_client_cert' => ['test-cert'],
          'isolation_segments' => '[is1]',
          'routing_table_sharding_mode' => 'sharding',
          'route_services_timeout' => 10,
          'route_services_secret' => 'secret',
          'route_services_secret_decrypt_only' => 'secret',
          'route_services_recommend_https' => false,
          'extra_headers_to_log' => 'test-header',
          'max_header_kb' => 1_024,
          'enable_proxy' => false,
          'force_forwarded_proto_https' => false,
          'sanitize_forwarded_proto' => false,
          'suspend_pruning_if_nats_unavailable' => false,
          'max_idle_connections' => 100,
          'keep_alive_probe_interval' => '1s',
          'backends' => {
            'max_attempts' => 3,
            'max_conns' => 100,
            'cert_chain' => TEST_CERT,
            'private_key' => TEST_KEY
          },
          'route_services' => {
            'max_attempts' => 3,
            'cert_chain' => ROUTE_SERVICES_CLIENT_TEST_CERT,
            'private_key' => ROUTE_SERVICES_CLIENT_TEST_KEY
          },
          'frontend_idle_timeout' => 5,
          'ip_local_port_range' => '1024 65535',
          'per_request_metrics_reporting' => true,
          'send_http_start_stop_server_event' => true,
          'send_http_start_stop_client_event' => true,
          'per_app_prometheus_http_metrics_reporting' => false
        },
        'golang' => {},
        'request_timeout_in_seconds' => 100,
        'endpoint_dial_timeout_in_seconds' => 6,
        # the websocket_dial_timeout_in_seconds will default to the value of endpoint_dial_timeout_in_seconds if not set
        'tls_handshake_timeout_in_seconds' => 9,
        'routing_api' => {
          'enabled' => false,
          'port' => '23423',
          'ca_certs' => "CA CERTS\n",
          'private_key' => 'PRIVATE KEY',
          'cert_chain' => 'CERT CHAIN'
        },
        'uaa' => {
          'ca_cert' => 'blah-cert',
          'ssl' => {
            'port' => 900
          },
          'clients' => {
            'gorouter' => {
              'secret' => 'secret'
            }
          },
          'token_endpoint' => 'uaa.token_endpoint'
        },
        'nats' => {
          'machines' => ['127.0.0.1'],
          'port' => 8080,
          'user' => 'test',
          'password' => 'test_pass',
          'tls_enabled' => true,
          'ca_certs' => 'test_ca_cert',
          'cert_chain' => 'test_cert_chain',
          'private_key' => 'test_private_key'
        },
        'metron' => {
          'port' => 3745
        },
        'for_backwards_compatibility_only' => {
          'empty_pool_response_code_503' => true,
          'empty_pool_timeout' => '10s'
        }
      }
    end

    let(:template) { job.template('config/gorouter.yml') }
    let(:rendered_template) { template.render(deployment_manifest_fragment) }
    subject(:parsed_yaml) { YAML.safe_load(rendered_template) }

    context 'given a generally valid manifest' do
      context 'when ips have leading 0s' do
        it 'debug_address fails with a nice message' do
          deployment_manifest_fragment['router']['debug_address'] = '127.0.0.01:17002'
          expect do
            rendered_template
          end.to raise_error(/Invalid router.debug_address/)
        end
      end

      describe 'max_header_kb' do
        it 'should set max_header_kb' do
          expect(parsed_yaml['max_header_bytes']).to eq(1_048_576)
        end
      end

      describe 'keep alives' do
        context 'max_idle_connections is set' do
          context 'using default values' do
            it 'should not disable keep alives' do
              expect(parsed_yaml['disable_keep_alives']).to eq(false)
            end
            it 'should set endpoint_keep_alive_probe_interval' do
              expect(parsed_yaml['endpoint_keep_alive_probe_interval']).to eq('1s')
            end
            it 'should set max_idle_conns' do
              expect(parsed_yaml['max_idle_conns']).to eq(100)
              expect(parsed_yaml['max_idle_conns_per_host']).to eq(100)
            end
          end
          context 'using custom values' do
            before do
              deployment_manifest_fragment['router']['max_idle_connections'] = 2500
              deployment_manifest_fragment['router']['keep_alive_probe_interval'] = '500ms'
            end
            it 'should not disable keep alives' do
              expect(parsed_yaml['disable_keep_alives']).to eq(false)
            end
            it 'should set endpoint_keep_alive_probe_interval' do
              expect(parsed_yaml['endpoint_keep_alive_probe_interval']).to eq('500ms')
            end
            it 'should set max_idle_conns' do
              expect(parsed_yaml['max_idle_conns']).to eq(2500)
              expect(parsed_yaml['max_idle_conns_per_host']).to eq(100)
            end
            it 'should not enable zipkin' do
              expect(parsed_yaml.dig('tracing', 'enable_zipkin')).to eq(false)
            end
            it 'should not enable w3c' do
              expect(parsed_yaml.dig('tracing', 'enable_w3c')).to eq(false)
            end
          end
        end

        context 'min_tls_version' do
          context 'when it is set to an invalid version' do
            before do
              deployment_manifest_fragment['router']['min_tls_version'] = 'TLSv2.7'
            end

            it 'fails' do
              expect { raise parsed_yaml }.to raise_error(RuntimeError, 'router.min_tls_version must be "TLSv1.0", "TLSv1.1", "TLSv1.2" or "TLSv1.3"')
            end
          end
        end

        context 'max_tls_version' do
          context 'when it is set to an invalid version' do
            before do
              deployment_manifest_fragment['router']['max_tls_version'] = 'TLSv2.7'
            end

            it 'fails' do
              expect { raise parsed_yaml }.to raise_error(RuntimeError, 'router.max_tls_version must be "TLSv1.2" or "TLSv1.3"')
            end
          end
        end

        context 'max_idle_connections is not set' do
          before do
            deployment_manifest_fragment['router']['max_idle_connections'] = 0
          end
          it 'should disable keep alives' do
            expect(parsed_yaml['disable_keep_alives']).to eq(true)
          end
          it 'should not set endpoint_keep_alive_probe_interval' do
            expect(parsed_yaml['endpoint_keep_alive_probe_interval']).to eq(nil)
          end
          it 'should not set max_idle_conns' do
            expect(parsed_yaml['max_idle_conns']).to eq(nil)
            expect(parsed_yaml['max_idle_conns_per_host']).to eq(nil)
          end
        end
      end
      describe 'sticky_session_cookies' do
        context 'when no value is provided' do
          it 'should use JSESSIONID' do
            expect(parsed_yaml['sticky_session_cookie_names']).to match_array(['JSESSIONID'])
          end
        end
        context 'when multiple cookies are provided' do
          before do
            deployment_manifest_fragment['router']['sticky_session_cookie_names'] = %w[meow bark]
          end
          it 'should use all of the cookies in the config' do
            expect(parsed_yaml['sticky_session_cookie_names']).to match_array(%w[meow bark])
          end
        end
      end
      describe 'client_cert_validation' do
        context 'when no override is provided' do
          it 'should default to none' do
            expect(parsed_yaml['client_cert_validation']).to eq('none')
          end
        end

        context 'when the value is not valid' do
          before do
            deployment_manifest_fragment['router']['client_cert_validation'] = 'meow'
          end
          it 'should error' do
            expect { raise parsed_yaml }.to raise_error(RuntimeError, 'router.client_cert_validation must be "none", "request", or "require"')
          end
        end
      end

      context 'route_services_internal_lookup' do
        it 'defaults to false' do
          expect(parsed_yaml['route_services_hairpinning']).to eq(false)
        end

        context 'when enabled' do
          before do
            deployment_manifest_fragment['router']['route_services_internal_lookup'] = true
          end

          it 'parses to true' do
            expect(parsed_yaml['route_services_hairpinning']).to eq(true)
          end
        end
      end

      context 'route_services_internal_lookup_allowlist' do
        it 'defaults to empty array' do
          expect(parsed_yaml['route_services_hairpinning_allowlist']).to eq([])
        end

        context 'when set to a list' do
          before do
            deployment_manifest_fragment['router']['route_services_internal_lookup_allowlist'] = ['route-service.com', '*.example.com']
          end

          it 'parses to the same list' do
            expect(parsed_yaml['route_services_hairpinning_allowlist']).to eq(['route-service.com', '*.example.com'])
          end
        end
      end

      context 'html_error_template' do
        it 'is not set by default' do
          expect(parsed_yaml['html_error_template_file']).to be_nil
        end

        context 'when enabled' do
          before do
            deployment_manifest_fragment['router']['html_error_template'] = '<html>...goes here...</html>'
          end

          it 'sets the template path to the templated file' do
            expect(parsed_yaml['html_error_template_file']).to eq('/var/vcap/jobs/gorouter/config/error.html')
          end
        end
      end

      context 'tls_pem' do
        context 'when correct tls_pem is provided' do
          it 'should configure the property' do
            expect(parsed_yaml['tls_pem'].length).to eq(2)
            expect(parsed_yaml['tls_pem'][0]).to eq('cert_chain' => TEST_CERT,
                                                    'private_key' => 'test-key')
            expect(parsed_yaml['tls_pem'][1]).to eq('cert_chain' => TEST_CERT2,
                                                    'private_key' => 'test-key2')
          end
        end

        context 'when an incorrect tls_pem value is provided with missing cert' do
          before do
            deployment_manifest_fragment['router']['tls_pem'] = [{ 'private_key' => 'test-key' }]
          end
          it 'should error' do
            expect { raise parsed_yaml }.to raise_error(RuntimeError, 'must provide cert_chain and private_key with tls_pem')
          end
        end

        context 'when an incorrect tls_pem value is provided with missing key' do
          before do
            deployment_manifest_fragment['router']['tls_pem'] = [{ 'cert_chain' => 'test-chain' }]
          end
          it 'should error' do
            expect { raise parsed_yaml }.to raise_error(RuntimeError, 'must provide cert_chain and private_key with tls_pem')
          end
        end

        context 'when an incorrect tls_pem value is provided as wrong format' do
          before do
            deployment_manifest_fragment['router']['tls_pem'] = ['cert']
          end
          it 'should error' do
            expect { raise parsed_yaml }.to raise_error(RuntimeError, 'must provide cert_chain and private_key with tls_pem')
          end
        end
        context 'when a tls_pem does not have a SAN' do
          before do
            deployment_manifest_fragment['router']['tls_pem'][1]['cert_chain'] = CERT_WITHOUT_CN
          end
          it 'should error' do
            expect { raise parsed_yaml }.to raise_error(RuntimeError, 'tls_pem[1].cert_chain must include a subjectAltName extension')
          end
        end
      end

      describe 'drain' do
        it 'should configure properly' do
          expect(parsed_yaml['drain_wait']).to eq('10s')
          expect(parsed_yaml['drain_timeout']).to eq('300s')
        end
      end

      describe 'connection and request timeouts' do
        it 'should configure properly' do
          expect(parsed_yaml['endpoint_dial_timeout']).to eq('6s')
          expect(parsed_yaml['websocket_dial_timeout']).to eq('6s')
          expect(parsed_yaml['tls_handshake_timeout']).to eq('9s')
          expect(parsed_yaml['endpoint_timeout']).to eq('100s')
        end
      end

      describe 'explicitly set websocket_dial_timeout' do
        before do
          deployment_manifest_fragment['websocket_dial_timeout_in_seconds'] = 8
        end
        it 'should configure properly' do
          expect(parsed_yaml['endpoint_dial_timeout']).to eq('6s')
          expect(parsed_yaml['websocket_dial_timeout']).to eq('8s')
        end
      end

      describe 'prometheus metrics' do
        context 'by default' do
          it 'should not be configured' do
            expect(parsed_yaml['per_app_prometheus_http_metrics_reporting']).to be false
            expect(parsed_yaml['prometheus']).to be_nil
          end
        end
        context 'when prometheus is configured' do
          before do
            deployment_manifest_fragment['router']['per_app_prometheus_http_metrics_reporting'] = true
            deployment_manifest_fragment['router']['prometheus'] = { 'port' => 9090 }
          end
          it 'should set prometheus configuration' do
            expect(parsed_yaml['per_app_prometheus_http_metrics_reporting']).to be true
            expect(parsed_yaml['prometheus']['port']).to eq(9090)
            expect(parsed_yaml['prometheus']['cert_path']).to eq('/var/vcap/jobs/gorouter/config/certs/prometheus/prometheus.crt')
            expect(parsed_yaml['prometheus']['key_path']).to eq('/var/vcap/jobs/gorouter/config/certs/prometheus/prometheus.key')
            expect(parsed_yaml['prometheus']['ca_path']).to eq('/var/vcap/jobs/gorouter/config/certs/prometheus/prometheus_ca.crt')
          end
        end
        context 'when per app metrics is configured but prometheus port is not' do
          before do
            deployment_manifest_fragment['router']['per_app_prometheus_http_metrics_reporting'] = true
          end
          it 'should error' do
            expect { raise parsed_yaml }.to raise_error(RuntimeError, 'per_app_prometheus_http_metrics_reporting should not be set without configuring prometheus')
          end
        end
      end

      describe 'route_services' do
        context 'when max_attempts is set correctly' do
          it 'should configure the property' do
            expect(parsed_yaml['route_services']['max_attempts']).to eq(3)
          end
        end
        context 'when max_attempts is set to 0' do
          before do
            deployment_manifest_fragment['router']['route_services']['max_attempts'] = 0
          end
          it 'should error' do
            expect { raise parsed_yaml }.to raise_error(RuntimeError, 'router.route_services.max_attempts must maintain a minimum value of 1')
          end
        end
        context 'when max_attempts is negative' do
          before do
            deployment_manifest_fragment['router']['route_services']['max_attempts'] = -1
          end
          it 'should error' do
            expect { raise parsed_yaml }.to raise_error(RuntimeError, 'router.route_services.max_attempts must maintain a minimum value of 1')
          end
        end
        context 'when both cert_chain and private_key are provided' do
          it 'should configure the property' do
            expect(parsed_yaml['route_services']['cert_chain']).to eq(ROUTE_SERVICES_CLIENT_TEST_CERT)
            expect(parsed_yaml['route_services']['private_key']).to eq(ROUTE_SERVICES_CLIENT_TEST_KEY)
          end
        end
        context 'when cert_chain is provided but not private_key' do
          before do
            deployment_manifest_fragment['router']['route_services']['private_key'] = nil
          end
          it 'should error' do
            expect { raise parsed_yaml }.to raise_error(RuntimeError, 'route_services.cert_chain and route_services.private_key must be both provided or not at all')
          end
        end
        context 'when private_key is provided but not cert_chain' do
          before do
            deployment_manifest_fragment['router']['route_services']['cert_chain'] = nil
          end
          it 'should error' do
            expect { raise parsed_yaml }.to raise_error(RuntimeError, 'route_services.cert_chain and route_services.private_key must be both provided or not at all')
          end
        end
        context 'when neither cert_chain nor private_key are provided' do
          before do
            deployment_manifest_fragment['router']['route_services']['cert_chain'] = nil
            deployment_manifest_fragment['router']['route_services']['private_key'] = nil
          end
          it 'should not error and should not configure the properties' do
            expect(parsed_yaml['route_services']['cert_chain']).to eq('')
            expect(parsed_yaml['route_services']['private_key']).to eq('')
          end
        end
      end

      describe 'backends' do
        context 'when max_attempts is set correctly' do
          it 'should configure the property' do
            expect(parsed_yaml['backends']['max_attempts']).to eq(3)
          end
        end
        context 'when max_attempts is set to 0' do
          before do
            deployment_manifest_fragment['router']['backends']['max_attempts'] = 0
          end
          it 'should configure the property with indefinite retries' do
            expect(parsed_yaml['backends']['max_attempts']).to eq(0)
          end
        end
        context 'when max_attempts is negative' do
          before do
            deployment_manifest_fragment['router']['backends']['max_attempts'] = -1
          end
          it 'should error' do
            expect { raise parsed_yaml }.to raise_error(RuntimeError, 'router.backends.max_attempts cannot be negative')
          end
        end
        context 'when both cert_chain and private_key are provided' do
          it 'should configure the property' do
            expect(parsed_yaml['backends']['cert_chain']).to eq(TEST_CERT)
            expect(parsed_yaml['backends']['private_key']).to eq(TEST_KEY)
          end
        end
        context 'when cert_chain is provided but not private_key' do
          before do
            deployment_manifest_fragment['router']['backends']['private_key'] = nil
          end
          it 'should error' do
            expect { raise parsed_yaml }.to raise_error(RuntimeError, 'backends.cert_chain and backends.private_key must be both provided or not at all')
          end
        end
        context 'when private_key is provided but not cert_chain' do
          before do
            deployment_manifest_fragment['router']['backends']['cert_chain'] = nil
          end
          it 'should error' do
            expect { raise parsed_yaml }.to raise_error(RuntimeError, 'backends.cert_chain and backends.private_key must be both provided or not at all')
          end
        end
        context 'when neither cert_chain nor private_key are provided' do
          before do
            deployment_manifest_fragment['router']['backends']['cert_chain'] = nil
            deployment_manifest_fragment['router']['backends']['private_key'] = nil
          end
          it 'should not error and should not configure the properties' do
            expect(parsed_yaml['backends']['cert_chain']).to eq('')
            expect(parsed_yaml['backends']['private_key']).to eq('')
          end
        end
      end

      context 'certificate authorities' do
        context 'client_ca_certs' do
          context 'are not provided' do
            before do
              deployment_manifest_fragment['router']['only_trust_client_ca_certs'] = true
            end
            it 'renders the manifest with a default of nothing' do
              expect(parsed_yaml['client_ca_certs']).to eq('')
            end
          end

          context 'are provided' do
            context 'when only_trust_client_ca_certs is true' do
              before do
                deployment_manifest_fragment['router']['client_ca_certs'] = 'cool potato'
                deployment_manifest_fragment['router']['ca_certs'] = ['lame rhutabega']
                deployment_manifest_fragment['router']['only_trust_client_ca_certs'] = true
              end

              it 'client_ca_certs do not contain ca_certs' do
                expect(parsed_yaml['client_ca_certs']).to eq('cool potato')
              end

              it 'sets only_trust_client_ca_certs to true' do
                expect(parsed_yaml['only_trust_client_ca_certs']).to equal(true)
              end
            end

            context 'when only_trust_client_ca_certs is false' do
              before do
                deployment_manifest_fragment['router']['client_ca_certs'] = TEST_CERT
                deployment_manifest_fragment['router']['ca_certs'] = [TEST_CERT2, 'cert-too-short']
                deployment_manifest_fragment['router']['only_trust_client_ca_certs'] = false
              end

              it 'client_ca_certs contain only valid ca_certs' do
                expect(parsed_yaml['client_ca_certs']).to_not include('cert-too-short')
                expect(parsed_yaml['client_ca_certs']).to eq("#{TEST_CERT}\n#{TEST_CERT2}")
              end

              it 'sets only_trust_client_ca_certs to false' do
                expect(parsed_yaml['only_trust_client_ca_certs']).to equal(false)
              end
            end
          end
        end

        context 'ca_certs' do
          context 'when correct ca_certs is provided' do
            it 'should configure the property' do
              expect(parsed_yaml['ca_certs']).to eq([TEST_CERT])
            end
          end

          context 'when ca_certs is blank' do
            before do
              deployment_manifest_fragment['router']['ca_certs'] = nil
            end
            it 'returns a helpful error message' do
              expect { parsed_yaml }.to raise_error(/Can't find property '\["router.ca_certs"\]'/)
            end
          end

          context 'when a string is provided' do
            before do
              deployment_manifest_fragment['router']['ca_certs'] = 'some-tls-cert'
            end
            it 'raises error' do
              expect { parsed_yaml }.to raise_error(RuntimeError, 'ca_certs must be provided as an array of strings containing one or more certificates in PEM encoding')
            end
          end

          context 'when an empty string is provided' do
            before do
              deployment_manifest_fragment['router']['ca_certs'] = ''
            end
            it 'raises error' do
              expect { parsed_yaml }.to raise_error(RuntimeError, 'ca_certs must be provided as an array of strings containing one or more certificates in PEM encoding')
            end
          end

          context 'when one of the certs is empty' do
            before do
              deployment_manifest_fragment['router']['ca_certs'] = [' ', TEST_CERT]
            end
            it 'only keeps non-empty certs' do
              expect(parsed_yaml['ca_certs']).to eq([TEST_CERT])
            end
          end

          context 'when one of the certs is nil' do
            before do
              deployment_manifest_fragment['router']['ca_certs'] = [nil, TEST_CERT]
            end
            it 'only keeps non-empty certs' do
              expect(parsed_yaml['ca_certs']).to eq([TEST_CERT])
            end
          end

          context 'when one of the certs is less than 50 char' do
            before do
              deployment_manifest_fragment['router']['ca_certs'] = ['meow-meow-meow-meow', TEST_CERT]
            end
            it 'only keeps longer value certs' do
              expect(parsed_yaml['ca_certs']).to eq([TEST_CERT])
            end
          end

          context 'when set to a multi-line string' do
            let(:test_certs) do
              '
    some
    multi
    line

    string
    with lots

    of

    whitespace

              '
            end

            before do
              deployment_manifest_fragment['router']['ca_certs'] = [test_certs]
            end
            it 'successfully configures the property' do
              expect(parsed_yaml['ca_certs']).to eq([test_certs])
            end
          end
        end
      end

      # ca_certs, private_key, cert_chain
      context 'routing-api' do
        context 'when the routing API is disabled' do
          before do
            deployment_manifest_fragment['routing_api']['enabled'] = false
          end

          context 'when ca_certs is not set' do
            before do
              deployment_manifest_fragment['routing_api']['ca_certs'] = 'nice'
            end

            it 'is happy' do
              expect { parsed_yaml }.not_to raise_error
            end
          end
        end

        context 'when the routing API is enabled' do
          let(:property_value) { ('a'..'z').to_a.shuffle.join }
          let(:link_value) { ('a'..'z').to_a.shuffle.join }

          before do
            deployment_manifest_fragment['routing_api']['enabled'] = true
          end

          describe 'routing API port' do
            it_behaves_like 'overridable_link', LinkConfiguration.new(
              description: 'Routing API port',
              property: 'routing_api.port',
              link_path: 'routing_api.mtls_port',
              parsed_yaml_property: 'routing_api.port'
            )
          end

          describe 'ca_certs' do
            let(:ca_certs) { parsed_yaml['routing_api']['ca_certs'] }

            context 'when a simple array is provided' do
              before do
                deployment_manifest_fragment['routing_api']['ca_certs'] = ['some-tls-cert']
              end

              it 'raises error' do
                expect { parsed_yaml }.to raise_error(RuntimeError, 'routing_api.ca_certs must be provided as a single string block')
              end
            end

            context 'when set to a multi-line string' do
              let(:str) { "some   \nmulti\nline\n  string" }

              before do
                deployment_manifest_fragment['routing_api']['ca_certs'] = str
              end

              it 'successfully configures the property' do
                expect(ca_certs).to eq(str)
              end
            end

            context 'when containing dashes' do
              let(:str) { '---some---string------with--dashes' }

              before do
                deployment_manifest_fragment['routing_api']['ca_certs'] = str
              end

              it 'successfully configures the property' do
                expect(ca_certs).to eq(str)
              end
            end

            it_behaves_like 'overridable_link', LinkConfiguration.new(
              description: 'Routing API server CA certificate',
              property: 'routing_api.ca_certs',
              link_path: 'routing_api.mtls_ca',
              parsed_yaml_property: 'routing_api.ca_certs'
            )
          end

          describe 'private_key' do
            context 'when set to a multi-line string' do
              let(:str) { "some   \nmulti\nline\n  string" }

              before do
                deployment_manifest_fragment['routing_api']['private_key'] = str
              end

              it 'successfully configures the property' do
                expect(parsed_yaml['routing_api']['private_key']).to eq(str)
              end
            end

            it_behaves_like 'overridable_link', LinkConfiguration.new(
              description: 'Routing API client private key',
              property: 'routing_api.private_key',
              link_path: 'routing_api.mtls_client_key',
              parsed_yaml_property: 'routing_api.private_key'
            )
          end

          describe 'cert_chain' do
            context 'when a simple array is provided' do
              before do
                deployment_manifest_fragment['routing_api']['cert_chain'] = ['some-tls-cert']
              end

              it 'raises error' do
                expect { parsed_yaml }.to raise_error(RuntimeError, 'routing_api.cert_chain must be provided as a single string block')
              end
            end

            context 'when set to a multi-line string' do
              let(:str) { "some   \nmulti\nline\n  string" }

              before do
                deployment_manifest_fragment['routing_api']['cert_chain'] = str
              end

              it 'successfully configures the property' do
                expect(parsed_yaml['routing_api']['cert_chain']).to eq(str)
              end
            end

            it_behaves_like 'overridable_link', LinkConfiguration.new(
              description: 'Routing API client certificate',
              property: 'routing_api.cert_chain',
              link_path: 'routing_api.mtls_client_cert',
              parsed_yaml_property: 'routing_api.cert_chain'
            )
          end
        end
      end

      context 'nats' do
        let(:property_value) { ('a'..'z').to_a.shuffle.join }
        let(:link_value) { ('a'..'z').to_a.shuffle.join }

        describe 'NATS port' do
          it_behaves_like 'overridable_link', LinkConfiguration.new(
            description: 'NATS server port number',
            property: 'nats.port',
            link_path: 'nats.port',
            link_namespace: 'nats-tls',
            parsed_yaml_property: 'nats.hosts.0.port'
          )
        end

        describe 'optional authentication' do
          let(:nats) { parsed_yaml['nats'] }

          context 'when username and password are provided' do
            before do
              deployment_manifest_fragment['nats']['user'] = 'nats'
              deployment_manifest_fragment['nats']['password'] = 'stan'
            end

            it 'contains auth information' do
              expect(nats['user']).to eq('nats')
              expect(nats['pass']).to eq('stan')
            end
          end
          context 'when username and password are not provided' do
            before do
              deployment_manifest_fragment['nats']['user'] = nil
              deployment_manifest_fragment['nats']['password'] = nil
            end

            it 'omits auth information' do
              expect(nats['user']).to be_nil
              expect(nats['pass']).to be_nil
            end
          end
        end

        describe 'ca_certs' do
          let(:ca_certs) { parsed_yaml['nats']['ca_certs'] }

          context 'when a simple array is provided' do
            before do
              deployment_manifest_fragment['nats']['ca_certs'] = ['some-tls-cert']
            end

            it 'raises error' do
              expect { parsed_yaml }.to raise_error(RuntimeError, 'nats.ca_certs must be provided as a single string block')
            end
          end

          context 'when set to a multi-line string' do
            let(:str) { "some   \nmulti\nline\n  string" }

            before do
              deployment_manifest_fragment['nats']['ca_certs'] = str
            end

            it 'successfully configures the property' do
              expect(ca_certs).to eq(str)
            end
          end

          context 'when containing dashes' do
            let(:str) { '---some---string------with--dashes' }

            before do
              deployment_manifest_fragment['nats']['ca_certs'] = str
            end

            it 'successfully configures the property' do
              expect(ca_certs).to eq(str)
            end
          end

          it_behaves_like 'overridable_link', LinkConfiguration.new(
            description: 'NATS server CA certificate',
            property: 'nats.ca_certs',
            link_path: 'nats.external.tls.ca',
            link_namespace: 'nats-tls',
            parsed_yaml_property: 'nats.ca_certs'
          )
        end

        describe 'private_key' do
          context 'when set to a multi-line string' do
            let(:str) { "some   \nmulti\nline\n  string" }

            before do
              deployment_manifest_fragment['nats']['private_key'] = str
            end

            it 'successfully configures the property' do
              expect(parsed_yaml['nats']['private_key']).to eq(str)
            end
          end
        end

        describe 'cert_chain' do
          context 'when a simple array is provided' do
            before do
              deployment_manifest_fragment['nats']['cert_chain'] = ['some-tls-cert']
            end

            it 'raises error' do
              expect { parsed_yaml }.to raise_error(RuntimeError, 'nats.cert_chain must be provided as a single string block')
            end
          end

          context 'when set to a multi-line string' do
            let(:str) { "some   \nmulti\nline\n  string" }

            before do
              deployment_manifest_fragment['nats']['cert_chain'] = str
            end

            it 'successfully configures the property' do
              expect(parsed_yaml['nats']['cert_chain']).to eq(str)
            end
          end
        end
      end

      context 'logging' do
        context 'when timestamp format is not provided' do
          it 'it defaults to rfc3339' do
            expect(parsed_yaml['logging']['format']['timestamp']).to eq('rfc3339')
          end
        end

        context 'when timestamp format is provided' do
          before do
            deployment_manifest_fragment['router']['logging'] = { 'format' => { 'timestamp' => 'unix-epoch' } }
          end

          it 'it sets the value correctly' do
            expect(parsed_yaml['logging']['format']['timestamp']).to eq('unix-epoch')
          end
        end

        fcontext 'when the timestamp format is set to deprecated' do
          before do
            deployment_manifest_fragment['router']['logging'] = { 'format' => { 'timestamp' => 'deprecated' } }
          end

          it 'sets the value to be unix-epoch' do
            expect(parsed_yaml['logging']['format']['timestamp']).to eq('unix-epoch')
          end
        end

        context 'when an invalid timestamp format is provided' do
          before do
            deployment_manifest_fragment['router']['logging'] = { 'format' => { 'timestamp' => 'meow' } }
          end

          it 'raises error' do
            expect { parsed_yaml }.to raise_error(RuntimeError, "'meow' is not a valid timestamp format for the property 'router.logging.format.timestamp'. Valid options are: 'rfc3339', 'deprecated', and 'unix-epoch'.")
          end
        end
      end

      context 'tracing' do
        context 'when zipkin is enabled' do
          before do
            deployment_manifest_fragment['router']['tracing']['enable_zipkin'] = true
          end

          it 'is happy' do
            expect { parsed_yaml }.not_to raise_error
          end

          it 'should enable zipkin' do
            expect(parsed_yaml['tracing']['enable_zipkin']).to eq(true)
          end
        end

        context 'when w3c is enabled' do
          before do
            deployment_manifest_fragment['router']['tracing']['enable_w3c'] = true
          end

          it 'is happy' do
            expect { parsed_yaml }.not_to raise_error
          end

          it 'should enable w3c tracing' do
            expect(parsed_yaml['tracing']['enable_w3c']).to eq(true)
          end

          it 'should not set the w3c tenant ID' do
            expect(parsed_yaml['tracing']['w3c_tenant_id']).to eq(nil)
          end

          context 'when w3c is enabled' do
            before do
              deployment_manifest_fragment['router']['tracing']['w3c_tenant_id'] = 'tid'
            end

            it 'is happy' do
              expect { parsed_yaml }.not_to raise_error
            end

            it 'should set wc3_tenant_id' do
              expect(parsed_yaml['tracing']['w3c_tenant_id']).to eq('tid')
            end
          end
        end
      end

      context 'backwards compatible properties' do
        context 'empty_pool_response_code_503' do
          context 'when it is not set' do
            it 'is happy' do
              expect { parsed_yaml }.not_to raise_error
              expect(parsed_yaml['empty_pool_response_code_503']).to eq(true)
            end
          end

          context 'when it is true' do
            before do
              deployment_manifest_fragment['for_backwards_compatibility_only']['empty_pool_response_code_503'] = true
            end
            it 'is set to true' do
              expect(parsed_yaml['empty_pool_response_code_503']).to eq(true)
              expect(parsed_yaml['empty_pool_timeout']).to eq('10s')
            end
          end

          context 'when it is false' do
            before do
              deployment_manifest_fragment['for_backwards_compatibility_only']['empty_pool_response_code_503'] = false
            end
            it 'is set to false' do
              expect(parsed_yaml['empty_pool_response_code_503']).to eq(false)
            end
          end
        end
      end

      context 'max_header_kb' do
        context 'less than 1' do
          before do
            deployment_manifest_fragment['router']['max_header_kb'] = 0
          end
          it 'throws an error' do
            expect { parsed_yaml }.to raise_error(/Invalid router.max_header_kb/)
          end
        end

        context 'greater than 1mb' do
          before do
            deployment_manifest_fragment['router']['max_header_kb'] = 1024 + 1
          end
          it 'throws an error' do
            expect { parsed_yaml }.to raise_error(/Invalid router.max_header_kb/)
          end
        end
      end
    end
  end

  describe 'healthchecker.yml' do
    let(:template) { job.template('config/healthchecker.yml') }
    let(:rendered_template) do
      template.render(
        {
          'router' =>
            {
              'logging_level' => 'debug',
              'status' => { 'port' => 8090, 'user' => 'some-user', 'password' => 'some-password' }
            }
        }
      )
    end

    subject(:parsed_yaml) { YAML.safe_load(rendered_template) }

    it 'populates component name' do
      expect(parsed_yaml['component_name']).to eq('gorouter-healthchecker')
    end

    it 'sets the log level' do
      expect(parsed_yaml['log_level']).to eq('debug')
    end

    it 'sets the healthcheck endpoint' do
      expect(parsed_yaml['healthcheck_endpoint']).to eq(
        {
          'host' => '0.0.0.0',
          'port' => 8090,
          'user' => 'some-user',
          'password' => 'some-password',
          'path' => '/healthz'
        }
      )
    end
  end

  describe 'indicators.yml' do
    let(:template) { job.template('config/indicators.yml') }
    let(:rendered_template) { template.render({}) }
    subject(:parsed_yaml) { YAML.safe_load(rendered_template) }

    it 'populates metadata deployment name' do
      expect(parsed_yaml['metadata']['labels']['deployment']).to eq('my-deployment')
    end

    it 'contains indicators' do
      expect(parsed_yaml['spec']['indicators']).to_not be_empty
    end
  end

  describe 'prom_scraper_config.yml' do
    let(:deployment_manifest_fragment) { {} }
    let(:template) { job.template('config/prom_scraper_config.yml') }
    let(:rendered_template) { template.render(deployment_manifest_fragment) }
    subject(:parsed_yaml) { YAML.safe_load(rendered_template) }

    it 'renders an empty file' do
      expect(parsed_yaml).to be_nil
    end

    context 'when gorouter prometheus support is enabled' do
      before do
        deployment_manifest_fragment['router'] = {
          'prometheus' => {
            'port' => 9090,
            'server_name' => 'example.org'
          }
        }
      end
      it 'configures the prom scraper to scrape the gorouter prometheus endpoint' do
        expect(parsed_yaml['port']).to eq(9090)
        expect(parsed_yaml['scheme']).to eq('https')
        expect(parsed_yaml['server_name']).to eq('example.org')
      end
      it 'configures the prom scraper to emit events with source_id and instance_id tags' do
        expect(parsed_yaml['source_id']).to eq('gorouter')
        expect(parsed_yaml['instance_id']).to_not be_empty
      end
    end
  end

  describe 'error.html' do
    let(:template) { job.template('config/error.html') }
    let(:rendered_template) do
      updated_properties = { 'router' => { 'html_error_template' => html } }
      template.render(updated_properties)
    end

    context 'by default' do
      let(:html) { '' }

      it 'is empty' do
        expect(rendered_template).to eq("\n")
      end
    end

    context 'when an error template is defined' do
      let(:html) { '<html>error</html>' }

      it 'consists of the rendered template' do
        expect(rendered_template).to eq("<html>error</html>\n")
      end
    end
  end

  describe 'pre-start' do
    let(:template) { job.template('bin/pre-start') }
    let(:properties) do
      { 'router' => {
        'port' => 81,
        'status' => { 'port' => 8081 },
        'prometheus' => { 'port' => 7070 },
        'tls_port' => 442,
        'debug_address' => '127.0.0.1:17003'
      } }
    end

    context 'ip_local_reserved_ports' do
      it 'contains reserved ports in order' do
        rendered_template = template.render(properties)
        ports = '81,442,2822,2825,3458,3459,3460,3461,7070,8081,8853,17003,53080'
        expect(rendered_template).to include("\"#{ports}\" > /proc/sys/net/ipv4/ip_local_reserved_ports")
      end

      context 'when prometheus port is not set' do
        it 'skips that port' do
          properties['router'].delete('prometheus')
          rendered_template = template.render(properties)
          ports = '81,442,2822,2825,3458,3459,3460,3461,8081,8853,17003,53080'
          expect(rendered_template).to include("\"#{ports}\" > /proc/sys/net/ipv4/ip_local_reserved_ports")
        end
      end

      context 'when debug_address does not contain a port' do
        it 'skips that port' do
          properties['router']['debug_address'] = 'meow'
          rendered_template = template.render(properties)
          ports = '81,442,2822,2825,3458,3459,3460,3461,7070,8081,8853,53080'
          expect(rendered_template).to include("\"#{ports}\" > /proc/sys/net/ipv4/ip_local_reserved_ports")
        end
      end
    end
  end
end

# rubocop:enable Layout/LineLength
# rubocop:enable Metrics/BlockLength
