package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"time"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagerflags"
	"code.cloudfoundry.org/routing-release/routing-api/models"
	uaaclient "code.cloudfoundry.org/uaa-go-client"
	uaaconfig "code.cloudfoundry.org/uaa-go-client/config"

	"code.cloudfoundry.org/clock"
	"code.cloudfoundry.org/routing-release/routing-api"
	"code.cloudfoundry.org/routing-release/routing-api-cli/commands"
	trace "code.cloudfoundry.org/trace-logger"
	"github.com/urfave/cli"
)

const (
	RTR_TRACE                      = "RTR_TRACE"
	DefaultTokenFetchRetryInterval = 5 * time.Second
	DefaultTokenFetchNumRetries    = uint32(1)
	DefaultExpirationBufferTime    = int64(30)
)

var version string

var skipVerificationFlag = cli.BoolFlag{
	Name:  "skip-tls-verification, k",
	Usage: "Skip OAuth TLS Verification (optional)",
}

var flags = []cli.Flag{
	cli.StringFlag{
		Name:  "api",
		Usage: "Endpoint for the routing-api. (required)",
	},
	cli.StringFlag{
		Name:  "client-id",
		Usage: "Id of the OAuth client. (required)",
	},
	cli.StringFlag{
		Name:  "client-secret",
		Usage: "Secret for OAuth client. (required)",
	},
	cli.StringFlag{
		Name:  "oauth-url",
		Usage: "URL for OAuth client. (required)",
	},
	skipVerificationFlag,
	cli.StringFlag{
		Name:  "ca-certs",
		Usage: "CA for UAA client (optional)",
	},
}

var eventsFlags = []cli.Flag{
	cli.BoolFlag{
		Name:  "http",
		Usage: "Stream HTTP events",
	},
	cli.BoolFlag{
		Name:  "tcp",
		Usage: "Stream TCP events",
	},
}

var cliCommands = []cli.Command{
	{
		Name:  "register",
		Usage: "Registers routes with the routing-api",
		Description: `Routes must be specified in JSON format, like so:
'[{"route":"foo.com", "port":12345, "ip":"1.2.3.4", "ttl":5, "log_guid":"log-guid"}]'`,
		Action: registerRoutes,
		Flags:  flags,
	},
	{
		Name:  "unregister",
		Usage: "Unregisters routes with the routing-api",
		Description: `Routes must be specified in JSON format, like so:
'[{"route":"foo.com", "port":12345, "ip":"1.2.3.4"]'`,
		Action: unregisterRoutes,
		Flags:  flags,
	},
	{
		Name:   "list",
		Usage:  "Lists the currently registered routes",
		Action: listRoutes,
		Flags:  flags,
	},
	{
		Name:   "events",
		Usage:  "Stream events from the Routing API",
		Action: streamEvents,
		Flags:  append(flags, eventsFlags...),
	},
}

var environmentVariableHelp = `ENVIRONMENT VARIABLES:
   RTR_TRACE=true	Print API request diagnostics to stdout`

func main() {
	lagerflags.AddFlags(flag.CommandLine)
	fmt.Println()
	app := cli.NewApp()
	app.Name = "rtr"
	app.Usage = "A CLI for the Router API server."
	authors := []cli.Author{cli.Author{Name: "Cloud Foundry Routing Team", Email: "cf-dev@lists.cloudfoundry.org"}}
	app.Authors = authors
	app.Commands = cliCommands
	app.CommandNotFound = commandNotFound
	app.Version = version
	app.Flags = []cli.Flag{skipVerificationFlag}

	cli.AppHelpTemplate = cli.AppHelpTemplate + environmentVariableHelp + "\n"

	trace.NewLogger(os.Getenv(RTR_TRACE))

	app.Run(os.Args)
	os.Exit(0)
}

func registerRoutes(c *cli.Context) {
	issues := checkFlags(c)
	errorMessage := "route registration failed:"
	issues = append(issues, checkArguments(c, "register")...)

	if len(issues) > 0 {
		printHelpForCommand(c, issues, "register")
	}

	desiredRoutes := c.Args().First()
	var routes []models.Route

	err := json.Unmarshal([]byte(desiredRoutes), &routes)
	checkError(errorMessage, err)

	client, err := newRoutingApiClient(c)
	checkError(errorMessage, err)

	err = commands.Register(client, routes)
	checkError(errorMessage, err)

	fmt.Printf("Successfully registered routes: %s\n", desiredRoutes)
}

func unregisterRoutes(c *cli.Context) {
	issues := checkFlags(c)
	errorMessage := "route unregistration failed:"
	issues = append(issues, checkArguments(c, "unregister")...)

	if len(issues) > 0 {
		printHelpForCommand(c, issues, "unregister")
	}

	desiredRoutes := c.Args().First()
	var routes []models.Route
	err := json.Unmarshal([]byte(desiredRoutes), &routes)
	checkError(errorMessage, err)

	client, err := newRoutingApiClient(c)
	checkError(errorMessage, err)

	err = commands.UnRegister(client, routes)
	checkError(errorMessage, err)

	fmt.Printf("Successfully unregistered routes: %s\n", desiredRoutes)
}

func listRoutes(c *cli.Context) {
	errorMessage := "listing routes failed:"
	issues := checkFlags(c)
	issues = append(issues, checkArguments(c, "list")...)

	if len(issues) > 0 {
		printHelpForCommand(c, issues, "list")
	}

	client, err := newRoutingApiClient(c)
	checkError(errorMessage, err)

	routes, err := commands.List(client)
	if err != nil {
		fmt.Println("listing routes failed:", err)
		os.Exit(3)
	}

	prettyRoutes, _ := json.Marshal(routes)

	fmt.Printf("%v\n", string(prettyRoutes))
}

func streamEvents(c *cli.Context) {
	issues := checkFlags(c)
	issues = append(issues, checkArguments(c, "events")...)

	if len(issues) > 0 {
		printHelpForCommand(c, issues, "events")
	}

	streamHttp := c.Bool("http")
	streamTcp := c.Bool("tcp")

	if !streamHttp && !streamTcp {
		streamHttp = true
		streamTcp = true
	}

	client, err := newRoutingApiClient(c)
	checkError("streaming events failed:", err)
	errorChan := make(chan error)
	eventChan := make(chan string)

	numOfSubscriptions := 0

	if streamHttp {
		numOfSubscriptions++
		go streamHttpEvents(client, eventChan, errorChan)
	}

	if streamTcp {
		numOfSubscriptions++
		go streamTcpEvents(client, eventChan, errorChan)
	}

	errorCount := 0

loop:
	for {
		select {
		case eventMessage := <-eventChan:
			fmt.Println(eventMessage)
		case err := <-errorChan:
			errorCount++
			fmt.Printf("Connection closed: %s", err.Error())
			if errorCount >= numOfSubscriptions {
				break loop
			}
		}
	}
}

func streamHttpEvents(client routing_api.Client, eventChan chan string, errorChan chan error) {
	eventSource, err := client.SubscribeToEvents()
	if err != nil {
		fmt.Println("streaming events failed:", err)
		return
	}
	for {
		e, err := eventSource.Next()
		if err != nil {
			errorChan <- err
			break
		}

		event, _ := json.Marshal(e)
		eventChan <- fmt.Sprintf("%v\n", string(event))
	}
}

func streamTcpEvents(client routing_api.Client, eventChan chan string, errorChan chan error) {
	eventSource, err := client.SubscribeToTcpEvents()
	if err != nil {
		fmt.Println("streaming events failed:", err)
		return
	}
	for {
		e, err := eventSource.Next()
		if err != nil {
			errorChan <- err
			break
		}

		event, _ := json.Marshal(e)
		eventChan <- fmt.Sprintf("%v\n", string(event))
	}
}

func buildOauthConfig(c *cli.Context) *uaaconfig.Config {

	return &uaaconfig.Config{
		UaaEndpoint:           c.String("oauth-url"),
		SkipVerification:      c.Bool("skip-tls-verification"),
		ClientName:            c.String("client-id"),
		ClientSecret:          c.String("client-secret"),
		MaxNumberOfRetries:    3,
		RetryInterval:         500 * time.Millisecond,
		ExpirationBufferInSec: 30,
		CACerts:               c.String("ca-certs"),
	}

}

func checkFlags(c *cli.Context) []string {
	var issues []string

	if c.String("api") == "" {
		issues = append(issues, "Must provide an API endpoint for the routing-api component.")
	}

	if c.String("client-id") == "" {
		issues = append(issues, "Must provide the id of an OAuth client.")
	}

	if c.String("client-secret") == "" {
		issues = append(issues, "Must provide an OAuth secret.")
	}

	if c.String("oauth-url") == "" {
		issues = append(issues, "Must provide an URL to the OAuth client.")
	}

	_, err := url.Parse(c.String("oauth-url"))
	if err != nil {
		issues = append(issues, "Invalid OAuth client URL")
	}

	return issues
}

func checkArguments(c *cli.Context, cmd string) []string {
	var issues []string

	switch cmd {
	case "register", "unregister":
		if len(c.Args()) > 1 {
			issues = append(issues, "Unexpected arguments.")
		} else if len(c.Args()) < 1 {
			issues = append(issues, "Must provide routes JSON.")
		}
	case "list", "events":
		if len(c.Args()) > 0 {
			issues = append(issues, "Unexpected arguments.")
		}
	}

	return issues
}

func printHelpForCommand(c *cli.Context, issues []string, cmd string) {
	for _, issue := range issues {
		fmt.Println(issue)
	}
	fmt.Println()
	cli.ShowCommandHelp(c, cmd)
	os.Exit(1)
}

func commandNotFound(c *cli.Context, cmd string) {
	fmt.Println("Not a valid command:", cmd)
	os.Exit(1)
}

func newRoutingApiClient(c *cli.Context) (routing_api.Client, error) {

	uaaClient, err := newUaaClient(c)
	if err != nil {
		return nil, err
	}

	token, err := uaaClient.FetchToken(true)
	if err != nil {
		return nil, err
	}

	routingApiClient := routing_api.NewClient(c.String("api"), c.Bool("skip-tls-verification"))
	routingApiClient.SetToken(token.AccessToken)
	return routingApiClient, nil

}

func checkError(message string, err error) {
	if err != nil {
		fmt.Println(message, err.Error())
		os.Exit(3)
	}
}

func newUaaClient(c *cli.Context) (uaaclient.Client, error) {
	rtr_trace := os.Getenv(RTR_TRACE)
	var logger lager.Logger

	if rtr_trace == "true" {
		logger, _ = lagerflags.New("rtr")
	} else {
		logger = lager.NewLogger("rtr")
	}

	cfg := buildOauthConfig(c)
	klok := clock.NewClock()

	uaaClient, err := uaaclient.NewClient(logger, cfg, klok)

	if err != nil {
		return nil, err
	}
	return uaaClient, nil
}
