package router_group_port_checker

import (
	routing_api "code.cloudfoundry.org/routing-release/routing-api"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	uaaclient "code.cloudfoundry.org/uaa-go-client"
	"code.cloudfoundry.org/uaa-go-client/schema"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type PortChecker struct {
	routingAPIClient routing_api.Client
	uaaClient        uaaclient.Client
}

func NewPortChecker(routingAPIClient routing_api.Client, uaaClient uaaclient.Client) PortChecker {
	return PortChecker{
		routingAPIClient: routingAPIClient,
		uaaClient:        uaaClient,
	}
}

func (pc *PortChecker) Check(systemComponentPorts []int) (bool, error) {
	routerGroups, err := pc.getRouterGroups()
	if err != nil {
		return false, err
	}
	shouldExit, portErrors := validateRouterGroups(routerGroups, systemComponentPorts)

	if len(portErrors) == 0 {
		return false, nil
	}

	return shouldExit, errors.New(strings.Join(portErrors, "\n"))
}

func (pc *PortChecker) getRouterGroups() ([]models.RouterGroup, error) {
	var err error
	numRetries := 3

	for i := 0; i < numRetries; i++ {
		var token *schema.Token
		token, err = pc.uaaClient.FetchToken(false)
		if err != nil {
			continue
		}
		pc.routingAPIClient.SetToken(token.AccessToken)
		break
	}

	if err != nil {
		return nil, fmt.Errorf("error-fetching-uaa-token: \"%s\"", err.Error())
	}

	for i := 0; i < numRetries; i++ {
		var routerGroups []models.RouterGroup
		routerGroups, err = pc.routingAPIClient.RouterGroups()
		if err == nil {
			return routerGroups, nil
		}
	}
	return nil, fmt.Errorf("error-fetching-routing-groups: \"%s\"", err.Error())
}

func validateRouterGroups(routerGroups []models.RouterGroup, systemComponentPorts []int) (bool, []string) {
	var errors []string
	shouldExit := false

	for _, group := range routerGroups {
		reservablePorts := group.ReservablePorts
		ranges, _ := reservablePorts.Parse()
		for _, r := range ranges {
			start, end := r.Endpoints()

			overlappingPorts := []string{}
			for _, portInt := range systemComponentPorts {
				port := uint64(portInt)
				if port >= start && port <= end {
					overlappingPorts = append(overlappingPorts, strconv.FormatUint(port, 10))
				}
			}

			if len(overlappingPorts) > 0 {
				shouldExit = true
				formattedPorts := strings.Join(overlappingPorts, ", ")
				errors = append(errors, fmt.Sprintf("The reserved ports for router group '%v' contains the following reserved system component port(s): '%v'. Please update your router group accordingly.", group.Name, formattedPorts))
			}
		}
	}
	return shouldExit, errors
}
