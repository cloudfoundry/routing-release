package workflowhelpers

import (
	"time"

	"github.com/cloudfoundry-incubator/cf-test-helpers/commandstarter"
	"github.com/cloudfoundry-incubator/cf-test-helpers/workflowhelpers/internal"
)

type GenericResource struct {
	Metadata struct {
		Guid string `json:"guid"`
	} `json:"metadata"`
}

type QueryResponse struct {
	Resources []GenericResource `struct:"resources"`
}

var ApiRequest = func(method, endpoint string, response interface{}, timeout time.Duration, data ...string) {
	internal.ApiRequest(commandstarter.NewCommandStarter(), method, endpoint, response, timeout, data...)
}
