package commands

import (
	"code.cloudfoundry.org/routing-release/routing-api"
	"code.cloudfoundry.org/routing-release/routing-api/models"
)

func List(client routing_api.Client) ([]models.Route, error) {
	return client.Routes()
}
