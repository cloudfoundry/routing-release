package main

import (
	"flag"
	"log"

	"code.cloudfoundry.org/gorouter/config"
)

var (
	configFile string
)

func main() {
	flag.StringVar(&configFile, "c", "", "Configuration File")
	flag.Parse()

	_, err := config.InitConfigFromFile(configFile)
	if err != nil {
		log.Fatal("failed-to-load-config: ", err)
	}
	log.Print("config-loaded-successfully")
}
