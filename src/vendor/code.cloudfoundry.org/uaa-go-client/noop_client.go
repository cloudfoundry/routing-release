package uaa_go_client

import "code.cloudfoundry.org/uaa-go-client/schema"

type NoOpUaaClient struct {
}

func NewNoOpUaaClient() Client {
	return &NoOpUaaClient{}
}

func (c *NoOpUaaClient) FetchToken(useCachedToken bool) (*schema.Token, error) {
	return &schema.Token{}, nil
}
func (c *NoOpUaaClient) DecodeToken(uaaToken string, desiredPermissions ...string) error {
	return nil
}
func (c *NoOpUaaClient) FetchKey() (string, error) {
	return "", nil
}
func (c *NoOpUaaClient) FetchIssuer() (string, error) {
	return "", nil
}
func (c *NoOpUaaClient) RegisterOauthClient(oauthClient *schema.OauthClient) (*schema.OauthClient, error) {
	return oauthClient, nil
}
