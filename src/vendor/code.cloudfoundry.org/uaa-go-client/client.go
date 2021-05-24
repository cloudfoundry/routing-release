package uaa_go_client

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	trace "code.cloudfoundry.org/trace-logger"
	jwt "github.com/dgrijalva/jwt-go"

	"code.cloudfoundry.org/lager"

	"code.cloudfoundry.org/uaa-go-client/config"
	"code.cloudfoundry.org/uaa-go-client/schema"
)

var ErrClientAlreadyExists = errors.New("Client already exists")

type uaaKey struct {
	Alg   string `json:"alg"`
	Value string `json:"value"`
}

//go:generate counterfeiter -o fakes/fake_client.go . Client
type Client interface {
	FetchToken(forceUpdate bool) (*schema.Token, error)
	FetchKey() (string, error)
	DecodeToken(uaaToken string, desiredPermissions ...string) error
	RegisterOauthClient(*schema.OauthClient) (*schema.OauthClient, error)
	FetchIssuer() (string, error)
}

type UaaClient struct {
	clock            clock
	config           *config.Config
	client           *http.Client
	cachedToken      *schema.Token
	refetchTokenTime int64
	lock             *sync.Mutex
	logger           lager.Logger
	uaaPublicKey     string
	rwlock           sync.RWMutex
	issuer           string
}

type OpenIDConfig struct {
	Issuer string `json:"issuer"`
}

type clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

func NewClient(logger lager.Logger, cfg *config.Config, clock clock) (Client, error) {
	logger.Session("uaa-client")

	var (
		client *http.Client
		err    error
		uri    *url.URL
	)

	if cfg == nil {
		return nil, errors.New("Configuration cannot be nil")
	}

	uri, err = cfg.CheckEndpoint()
	if err != nil {
		return nil, err
	}

	if uri.Scheme == "https" {
		client, err = newSecureClient(cfg)
		if err != nil {
			return nil, err
		}
	} else {
		client = &http.Client{}
	}

	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	if cfg.RequestTimeout < 0 {
		client.Timeout = config.DefaultRequestTimeout
	} else {
		client.Timeout = cfg.RequestTimeout
	}
	logger.Debug("HTTP Request timeout in seconds", lager.Data{"value": client.Timeout.Seconds()})

	if cfg.ExpirationBufferInSec < 0 {
		cfg.ExpirationBufferInSec = config.DefaultExpirationBufferInSec
		logger.Info("Expiration buffer in seconds set to default", lager.Data{"value": config.DefaultExpirationBufferInSec})
	}

	return &UaaClient{
		logger: logger,
		config: cfg,
		client: client,
		clock:  clock,
		lock:   new(sync.Mutex),
	}, nil
}

func newSecureClient(cfg *config.Config) (*http.Client, error) {
	tlsConfig := &tls.Config{InsecureSkipVerify: cfg.SkipVerification}
	if cfg.CACerts != "" {
		certBytes, err := ioutil.ReadFile(cfg.CACerts)
		if err != nil {
			return nil, fmt.Errorf("failed read ca cert file: %s", err.Error())
		}

		caCertPool := x509.NewCertPool()
		if ok := caCertPool.AppendCertsFromPEM(certBytes); !ok {
			return nil, errors.New("Unable to load caCert")
		}
		tlsConfig.RootCAs = caCertPool
	}

	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	client := &http.Client{Transport: tr}
	return client, nil
}

func (u *UaaClient) FetchIssuer() (string, error) {
	logger := u.logger.Session("uaa-client")
	fetchOpenIdURL := fmt.Sprintf("%s/.well-known/openid-configuration", u.config.UaaEndpoint)
	logger.Info("started-fetching-openId-metadata", lager.Data{"endpoint": fetchOpenIdURL})

	request, err := http.NewRequest("GET", fetchOpenIdURL, nil)
	if err != nil {
		return "", err
	}
	trace.DumpRequest(request)
	resp, err := u.client.Do(request)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	trace.DumpResponse(resp)
	logger.Info("finished-fetching-openId-metatdata", lager.Data{"status-code": resp.StatusCode})

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", errors.New(fmt.Sprintf("status code: %d, body: %s", resp.StatusCode, body))
	}

	data := &OpenIDConfig{}
	err = json.Unmarshal(body, data)
	if err != nil {
		return "", err
	}

	logger.Info("successfully-received-issuer")
	u.updateIssuer(data.Issuer)
	return data.Issuer, nil
}

func (u *UaaClient) FetchToken(forceUpdate bool) (*schema.Token, error) {
	logger := u.logger.Session("uaa-client")
	tokenURL := fmt.Sprintf("%s/oauth/token", u.config.UaaEndpoint)
	logger.Debug("started-fetching-token", lager.Data{"endpoint": tokenURL, "force-update": forceUpdate})

	if err := u.config.CheckCredentials(); err != nil {
		return nil, err
	}

	u.lock.Lock()
	defer u.lock.Unlock()

	if !forceUpdate && u.canReturnCachedToken() {
		logger.Debug("using-cached-token")
		return u.cachedToken, nil
	}

	retry := true
	var retryCount uint32 = 0
	var token *schema.Token
	var err error
	for retry == true {
		token, retry, err = u.doFetchToken()
		if token != nil {
			break
		}

		if err != nil {
			logger.Error("error-fetching-token", err)
		}

		if retry && retryCount < u.config.MaxNumberOfRetries {
			logger.Debug("retry-fetching-token", lager.Data{"retry-count": retryCount})
			retryCount++
			u.clock.Sleep(u.config.RetryInterval)
			continue
		} else {
			return nil, err
		}
	}

	logger.Debug("successfully-fetched-token")
	u.updateCachedToken(token)
	return token, nil
}

func (u *UaaClient) doFetchToken() (*schema.Token, bool, error) {
	logger := u.logger.Session("uaa-client")
	values := url.Values{}
	values.Add("grant_type", "client_credentials")
	requestBody := values.Encode()
	tokenURL := fmt.Sprintf("%s/oauth/token", u.config.UaaEndpoint)
	request, err := http.NewRequest("POST", tokenURL, bytes.NewBuffer([]byte(requestBody)))
	if err != nil {
		return nil, false, err
	}

	request.SetBasicAuth(u.config.ClientName, u.config.ClientSecret)
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	request.Header.Add("Accept", "application/json; charset=utf-8")
	trace.DumpRequest(request)

	logger.Info("fetch-token-from-uaa-start", lager.Data{"endpoint": request.URL})
	resp, err := u.client.Do(request)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()

	trace.DumpResponse(resp)
	logger.Info("fetch-token-from-uaa-end", lager.Data{"status-code": resp.StatusCode})

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}

	if resp.StatusCode != http.StatusOK {
		retry := false
		if resp.StatusCode >= http.StatusInternalServerError {
			retry = true
		}
		return nil, retry, errors.New(fmt.Sprintf("status code: %d, body: %s", resp.StatusCode, body))
	}

	token := &schema.Token{}
	err = json.Unmarshal(body, token)
	if err != nil {
		return nil, false, err
	}
	return token, false, nil
}

func (u *UaaClient) FetchKey() (string, error) {
	logger := u.logger.Session("uaa-client")
	getKeyUrl := fmt.Sprintf("%s/token_key", u.config.UaaEndpoint)

	logger.Info("fetch-key-starting", lager.Data{"endpoint": getKeyUrl})

	resp, err := u.client.Get(getKeyUrl)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("http error: status code: %d", resp.StatusCode)
		return "", err
	}

	decoder := json.NewDecoder(resp.Body)

	uaaKey := schema.UaaKey{}
	err = decoder.Decode(&uaaKey)
	if err != nil {
		return "", errors.New("unmarshalling error: " + err.Error())
	}

	if err = checkPublicKey(uaaKey.Value); err != nil {
		return "", err
	}

	u.rwlock.Lock()
	defer u.rwlock.Unlock()
	u.uaaPublicKey = uaaKey.Value

	logger.Info("fetch-key-successful")
	return uaaKey.Value, nil
}

func (u *UaaClient) DecodeToken(uaaToken string, desiredPermissions ...string) error {
	logger := u.logger.Session("uaa-client")
	logger.Debug("decode-token-started")
	defer logger.Debug("decode-token-completed")
	var err error
	jwtToken, err := checkTokenFormat(uaaToken)
	if err != nil {
		return err
	}

	var (
		token            *jwt.Token
		uaaKey           string
		forceUaaKeyFetch bool
	)

	for i := 0; i < 2; i++ {
		uaaKey, err = u.getUaaTokenKey(logger, forceUaaKeyFetch)

		if err == nil {
			token, err = jwt.Parse(jwtToken, func(t *jwt.Token) (interface{}, error) {
				if !u.isValidSigningMethod(t) {
					return nil, errors.New("invalid signing method")
				}
				if !u.isValidIssuer(t) {
					return nil, errors.New("invalid issuer")
				}

				pubKey, err := jwt.ParseRSAPublicKeyFromPEM([]byte(uaaKey))
				if err != nil {
					return nil, err
				}

				return pubKey, nil
			})

			if err != nil {
				logger.Error("decode-token-failed", err)
				if matchesError(err, jwt.ValidationErrorSignatureInvalid) {
					forceUaaKeyFetch = true
					continue
				}
				if matchesError(err, jwt.ValidationErrorIssuedAt) {
					logger.Info("decode-token-ignoring-issued-at-validation")
					err = nil
					break
				}
			}
		}

		break
	}

	if err != nil {
		return err
	}

	hasPermission := false
	var permissions interface{}
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		permissions = claims["scope"]
	}
	a := permissions.([]interface{})

	for _, permission := range a {
		for _, desiredPermission := range desiredPermissions {
			if permission.(string) == desiredPermission {
				hasPermission = true
				break
			}
		}
	}

	if !hasPermission {
		err = errors.New("Token does not have '" + strings.Join(desiredPermissions, "', '") + "' scope")
		return err
	}

	return nil
}

func (u *UaaClient) isValidIssuer(token *jwt.Token) bool {
	if u.issuer == "" {
		_, err := u.FetchIssuer()
		if err != nil {
			return false
		}
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		return claims.VerifyIssuer(u.issuer, true)
	}
	return false
}

func (u *UaaClient) isValidSigningMethod(token *jwt.Token) bool {
	if u.config.InsecureAllowAnySigningMethod {
		return true
	}

	switch token.Method {
	case jwt.SigningMethodRS256, jwt.SigningMethodRS384, jwt.SigningMethodRS512:
		return true
	default:
		return false
	}
}

func (u *UaaClient) RegisterOauthClient(oauthClient *schema.OauthClient) (*schema.OauthClient, error) {
	token, err := u.FetchToken(false)
	if err != nil {
		return nil, err
	}

	clientsUrl := fmt.Sprintf("%s/oauth/clients", u.config.UaaEndpoint)
	bodyBytes, err := json.Marshal(oauthClient)
	request, err := http.NewRequest("POST", clientsUrl, bytes.NewBuffer(bodyBytes))
	request.Header.Add("Content-Type", "application/json; charset=UTF-8")
	request.Header.Add("Accept", "application/json; charset=utf-8")
	request.Header.Add("Authorization", "bearer "+token.AccessToken)

	response, err := u.client.Do(request)

	if err != nil {
		return nil, err
	}

	if response.StatusCode == http.StatusConflict {
		return nil, ErrClientAlreadyExists
	}

	if response.StatusCode != http.StatusCreated {
		return nil, err
	}

	body, err := ioutil.ReadAll(response.Body)

	if err != nil {
		return nil, err
	}

	returnedOauthClient := &schema.OauthClient{}

	err = json.Unmarshal(body, returnedOauthClient)

	if err != nil {
		return nil, err
	}

	return returnedOauthClient, nil
}

func (u *UaaClient) canReturnCachedToken() bool {
	return u.cachedToken != nil && u.clock.Now().Unix() < u.refetchTokenTime
}

func (u *UaaClient) updateCachedToken(token *schema.Token) {
	u.logger.Debug("caching-token")
	u.cachedToken = token
	u.refetchTokenTime = u.clock.Now().Unix() + (token.ExpiresIn - u.config.ExpirationBufferInSec)
}

func checkPublicKey(key string) error {
	var block *pem.Block
	if block, _ = pem.Decode([]byte(key)); block == nil {
		return errors.New("Public uaa token must be PEM encoded")
	}
	return nil
}

func (u *UaaClient) updateIssuer(issuer string) {
	u.issuer = issuer
}

func checkTokenFormat(token string) (string, error) {
	tokenParts := strings.Split(token, " ")
	if len(tokenParts) != 2 {
		return "", errors.New("Invalid token format")
	}

	tokenType, userToken := tokenParts[0], tokenParts[1]
	if !strings.EqualFold(tokenType, "bearer") {
		return "", errors.New("Invalid token type: " + tokenType)
	}

	return userToken, nil
}

func matchesError(err error, errorType uint32) bool {
	if validationError, ok := err.(*jwt.ValidationError); ok {
		return validationError.Errors&errorType == errorType
	}
	return false
}

func (u *UaaClient) getUaaTokenKey(logger lager.Logger, forceFetch bool) (string, error) {
	if u.getUaaPublicKey() == "" || forceFetch {
		logger.Debug("fetching-new-uaa-key")
		key, err := u.FetchKey()
		if err != nil {
			return key, err
		}

		if u.getUaaPublicKey() == key {
			logger.Debug("Fetched the same verification key from UAA")
		} else {
			logger.Debug("Fetched a different verification key from UAA")
		}
		return key, nil
	}

	return u.getUaaPublicKey(), nil
}

func (u *UaaClient) getUaaPublicKey() string {
	u.rwlock.RLock()
	defer u.rwlock.RUnlock()
	return u.uaaPublicKey
}
