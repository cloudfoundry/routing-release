package config

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var validFrontendName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type RoutingAPIConfig struct {
	URI          string `yaml:"uri"`
	Port         uint16 `yaml:"port"`
	AuthDisabled bool   `yaml:"auth_disabled"`

	ClientCertificatePath string `yaml:"client_cert_path"`
	ClientPrivateKeyPath  string `yaml:"client_private_key_path"`
	CACertificatePath     string `yaml:"ca_cert_path"`
}

type OAuthConfig struct {
	TokenEndpoint     string `yaml:"token_endpoint"`
	Port              uint16 `yaml:"port"`
	SkipSSLValidation bool   `yaml:"skip_ssl_validation"`
	ClientName        string `yaml:"client_name"`
	ClientSecret      string `yaml:"client_secret"`
	CACerts           string `yaml:"ca_certs"`
}

type FrontendTLSConfig struct {
	Enabled bool `yaml:"enabled"`
	// https://www.haproxy.com/documentation/haproxy-configuration-manual/latest/#5.1-crt
	// https://www.haproxy.com/documentation/haproxy-configuration-manual/latest/#3.12-load
	CertificateDir string `yaml:"cert_path"`
}

type BackendTLSConfig struct {
	Enabled              bool   `yaml:"enabled"`
	CACertificatePath    string `yaml:"ca_cert_path"`
	ClientCertAndKeyPath string `yaml:"client_cert_and_key_path"`
}

type FrontendTLSJob struct {
	Name       string `yaml:"name"`
	CertChain  string `yaml:"cert_chain"`
	PrivateKey string `yaml:"private_key"`
}

type Config struct {
	OAuth                        OAuthConfig         `yaml:"oauth"`
	RoutingAPI                   RoutingAPIConfig    `yaml:"routing_api"`
	HaProxyPidFile               string              `yaml:"haproxy_pid_file"`
	IsolationSegments            []string            `yaml:"isolation_segments"`
	ReservedSystemComponentPorts []uint16            `yaml:"reserved_system_component_ports"`
	DrainWaitDuration            time.Duration       `yaml:"drain_wait"`
	BackendTLS                   BackendTLSConfig    `yaml:"backend_tls"`
	FrontendTLS                  []FrontendTLSConfig `yaml:"frontend_tls_pem"`
	FrontendTLSJob               []FrontendTLSJob    `yaml:"frontend_tls"`
}

const DrainWaitDefault = 20 * time.Second

func New(path string, enableCertCreation bool) (*Config, error) {
	c := &Config{}
	err := c.initConfigFromFile(path, enableCertCreation)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) FrontendTLSJobBasePath() string {
	if bp := os.Getenv("FRONTEND_TLS_BASE_PATH"); bp != "" {
		return bp
	}
	return "/var/vcap/jobs/tcp_router/config/certs/tcp-router/frontend"
}

func resolveGroupID(primary, fallback string) (int, error) {
	group, err := user.LookupGroup(primary)
	if err != nil {
		group, err = user.LookupGroup(fallback)
		if err != nil {
			return -1, err
		}
	}

	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return -1, err
	}

	return gid, nil
}

// initConfigFromFile initializes the config and write the certs to the filesystem when enableCertCreation is true.
//
// enableCertCreation is set to true during pre-start on all VMs. During pre-start initConfigFromFile is run
// to (1) validate the config provided and (2) create the FrontendTLSJob certs on the filesystem if provided.
//
// enableCertCreation is set to false when bosh starts the tcp-router job.
func (c *Config) initConfigFromFile(path string, enableCertCreation bool) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(b, &c); err != nil {
		return err
	}

	if c.HaProxyPidFile == "" {
		return errors.New("haproxy_pid_file is required")
	}

	if c.DrainWaitDuration < 0 {
		c.DrainWaitDuration = DrainWaitDefault
	}

	if enableCertCreation && len(c.FrontendTLSJob) > 0 {
		// remove existing directory which will clean out old certs if necessary
		if err := os.RemoveAll(c.FrontendTLSJobBasePath()); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("directory could not be removed: %w", err)
		}

		if err := c.createDir(c.FrontendTLSJobBasePath()); err != nil {
			return fmt.Errorf("could not create directory: %w", err)
		}
	}

	for i, cert := range c.FrontendTLSJob {
		// validations
		name := strings.TrimSpace(cert.Name)
		if name == "" {
			return fmt.Errorf("frontend_tls[%d]: empty name", i)
		}
		if !validFrontendName.MatchString(name) {
			return fmt.Errorf("frontend_tls[%d]: name %q contains invalid characters; only alphanumeric characters, hyphens, and underscores are allowed", i, name)
		}

		certChain := strings.TrimSpace(cert.CertChain)
		if certChain == "" {
			return fmt.Errorf("frontend_tls[%d]: empty cert_chain", i)
		}

		privateKey := strings.TrimSpace(cert.PrivateKey)
		if privateKey == "" {
			return fmt.Errorf("frontend_tls[%d]: empty private_key", i)
		}

		// check if san is present and that the cert chain is valid
		if err := certHasSAN(cert.CertChain); err != nil {
			return fmt.Errorf("frontend_tls[%d]: %w", i, err)
		}

		dirPath := filepath.Join(c.FrontendTLSJobBasePath(), name)

		// write certs to the disk
		if enableCertCreation {
			if err := c.createDir(dirPath); err != nil {
				return fmt.Errorf("frontend_tls[%d]: %w", i, err)
			}

			if err := c.writeCertsToDisk(dirPath, name, cert, privateKey); err != nil {
				return fmt.Errorf("frontend_tls[%d]: %w", i, err)
			}
		}

		// update the config
		c.FrontendTLS = append(c.FrontendTLS, FrontendTLSConfig{
			Enabled:        true,
			CertificateDir: dirPath,
		})
	}

	if c.BackendTLS.Enabled {
		if c.BackendTLS.CACertificatePath != "" {
			pemData, err := os.ReadFile(c.BackendTLS.CACertificatePath)
			if err != nil {
				return err
			}

			pemData = []byte(strings.TrimSpace(string(pemData)))
			if len(pemData) > 0 {
				var block *pem.Block
				block, _ = pem.Decode(pemData)
				if block == nil {
					return fmt.Errorf("Invalid PEM block found in file %q", c.BackendTLS.CACertificatePath)
				}
				if len(block.Headers) != 0 {
					return fmt.Errorf("Unexpected headers in PEM block in file %q: %v", c.BackendTLS.CACertificatePath, block.Headers)
				}
				if block.Type != "CERTIFICATE" {
					return fmt.Errorf("Unexpected PEM block type %q in file %q (wanted CERTIFICATE)", block.Type, c.BackendTLS.CACertificatePath)
				}
				_, err = x509.ParseCertificate(block.Bytes)
				if err != nil {
					return fmt.Errorf("failed to parse certificate in %q: %s", c.BackendTLS.CACertificatePath, err)
				}
			}
		} else {
			return fmt.Errorf("Backend TLS was enabled but no CA certificates were specified")
		}

		if c.BackendTLS.ClientCertAndKeyPath != "" {
			pemData, err := os.ReadFile(c.BackendTLS.ClientCertAndKeyPath)
			if err != nil {
				return err
			}

			pemData = []byte(strings.TrimSpace(string(pemData)))
			var certBlock *pem.Block
			certBlock, pemData = pem.Decode(pemData)
			if certBlock == nil {
				return fmt.Errorf("Invalid PEM CERTIFICATE found in file %q", c.BackendTLS.ClientCertAndKeyPath)
			}
			certPEM := bytes.NewBuffer([]byte{})
			err = pem.Encode(certPEM, certBlock)
			if err != nil {
				return fmt.Errorf("Could not encode cert as PEM data: %s", err)
			}

			pemData = []byte(strings.TrimSpace(string(pemData)))
			var keyBlock *pem.Block
			keyBlock, pemData = pem.Decode(pemData)
			if keyBlock == nil {
				return fmt.Errorf("Invalid PEM PRIVATE KEY found in file %q", c.BackendTLS.ClientCertAndKeyPath)
			}
			keyPEM := bytes.NewBuffer([]byte{})
			err = pem.Encode(keyPEM, keyBlock)
			if err != nil {
				return fmt.Errorf("Could not encode key as PEM data: %s", err)
			}

			if len(pemData) > 0 {
				return fmt.Errorf("Unexpected data at the end of %s", c.BackendTLS.ClientCertAndKeyPath)
			}

			_, err = tls.X509KeyPair(certPEM.Bytes(), keyPEM.Bytes())
			if err != nil {
				return fmt.Errorf("Unable to validate backend TLS client cert + key in file %q: %s", c.BackendTLS.ClientCertAndKeyPath, err)
			}
		}
	} else {
		c.BackendTLS = BackendTLSConfig{}
	}

	return nil
}

func (c *Config) createDir(dirPath string) error {
	// ensure directory exists
	if err := os.MkdirAll(dirPath, 0750); err != nil {
		return err
	}

	targetUID, targetGID, err := c.getUIDGID()
	if err != nil {
		return err
	}

	// Change ownership
	err = os.Chown(dirPath, targetUID, targetGID)
	if err != nil {
		return fmt.Errorf("Error changing ownership: %s", err)
	}

	return nil
}

func (c *Config) writeCertsToDisk(dirPath string, name string, cert FrontendTLSJob, privateKey string) error {
	uid, gid, err := c.getUIDGID()
	if err != nil {
		return err
	}

	certFilePath := filepath.Join(dirPath, fmt.Sprintf("%s.pem", name))
	keyFilePath := filepath.Join(dirPath, fmt.Sprintf("%s.pem.key", name))

	if err := writeFile(certFilePath, []byte(cert.CertChain), 0750, uid, gid); err != nil {
		return err
	}

	if err := writeFile(keyFilePath, []byte(privateKey), 0750, uid, gid); err != nil {
		return err
	}
	return nil
}

func (c *Config) getUIDGID() (int, int, error) {
	owner, err := user.Lookup("root")
	if err != nil {
		return 0, 0, err
	}

	uid, err := strconv.Atoi(owner.Uid)
	if err != nil {
		return 0, 0, err
	}

	gid, err := resolveGroupID("vcap", "root")
	if err != nil {
		return 0, 0, err
	}

	return uid, gid, nil
}

func certHasSAN(certChain string) error {
	block, _ := pem.Decode([]byte(certChain))
	if block == nil {
		return errors.New("failed to parse PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("could not parse certificate: %w", err)
	}

	if len(cert.DNSNames) > 0 {
		return nil
	}

	return fmt.Errorf("cert_chain must include either a subjectAltName extension or DNSNames")
}

func writeFile(path string, data []byte, mode os.FileMode, uid, gid int) error {
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}

	return os.Chown(path, uid, gid)
}
