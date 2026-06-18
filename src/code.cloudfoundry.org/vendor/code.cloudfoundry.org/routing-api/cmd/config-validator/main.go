package main

import (
	"flag"
	"log"

	"code.cloudfoundry.org/routing-api/config"
)

var (
	configFile string
	devMode    bool
)

func main() {
	flag.StringVar(&configFile, "config", "", "Configuration File")
	flag.BoolVar(&devMode, "devMode", false, "Disable authentication for easier development iteration")
	flag.Parse()

	_, err := config.NewConfigFromFile(configFile, devMode)
	if err != nil {
		log.Fatal("failed-to-load-config: ", err)
	}
	log.Print("config-loaded-successfully")
}
