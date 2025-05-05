package main

import (
	"flag"
	"log"

	"code.cloudfoundry.org/cf-tcp-router/config"
)

var (
	configFile string
)

func main() {
	flag.StringVar(&configFile, "config", "", "Configuration File")
	flag.Parse()

	_, err := config.New(configFile)
	if err != nil {
		log.Fatal("failed-to-load-config: ", err)
	}
	log.Print("config-loaded-successfully")
}
