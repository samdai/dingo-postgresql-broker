package clicmd

import (
	"fmt"
	"os"

	"github.com/codegangsta/cli"
	"github.com/samdai/dingo-postgresql-broker/broker"
)

// RunBroker runs the Cloud Foundry service broker API
func RunBroker(c *cli.Context) {
	cfg := loadConfig(c.String("config"))

	broker, err := broker.NewBroker(cfg)
	if err != nil {
		fmt.Println("Could not start broker")
		os.Exit(1)
		return
	}

	broker.Run()
}
