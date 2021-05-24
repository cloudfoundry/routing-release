package schema

type Token struct {
	AccessToken string `json:"access_token"`
	// Expire time in seconds
	ExpiresIn int64 `json:"expires_in"`
}

type UaaKey struct {
	Alg   string `json:"alg"`
	Value string `json:"value"`
}

type OauthClient struct {
	ClientId             string `json:"client_id"`
	Name                 string `json:"name"`
	ClientSecret         string `json:"client_secret"`
	Scope                []string `json:"scope"`
	ResourceIds          []string `json:"resource_ids"`
	Authorities          []string `json:"authorities"`
	AuthorizedGrantTypes []string `json:"authorized_grant_types"`
	AccessTokenValidity  int `json:"access_token_validity"`
	RedirectUri          []string `json:"redirect_uri"`
}
