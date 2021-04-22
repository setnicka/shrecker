package main

import (
	"fmt"
	"os"

	"github.com/Songmu/prompter"
	"github.com/coreos/go-log/log"
	"github.com/go-ini/ini"
	_ "github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "Shrecker"
	app.Version = "0.1.0"
	app.Usage = "Tracker for puzzle hunt games of multiple teams"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config, c",
			Usage: "Load configuration from `FILE`",
			Value: "config.ini",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:   "init-db",
			Usage:  "Initialize the DB.",
			Action: commandInitDB,
		},
		{
			Name:  "run",
			Usage: "Run the webserver",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "port,p",
					Usage: "Listen on port `PORT`",
					Value: 8000,
				},
			},
			Action: commandRunServer,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("Error while executing command: %v\n", err)
		os.Exit(1)
	}

	log.Info("Starting")
}

func commandRunServer(c *cli.Context) error {
	// 1. Get Config
	configfile := c.GlobalString("config")
	config, err := ini.Load(configfile)
	if err != nil {
		return errors.Wrapf(err, "Cannot open config file '%s'", configfile)
	}

	// 2. Load game data
	// TODO

	// 3. Open connection to the DB
	_, err = dbConnect(config)
	if err != nil {
		return err
	}

	// 4. Start the server
	// TODO

	return nil
}

func commandInitDB(c *cli.Context) error {
	// 1. Get Config
	configfile := c.GlobalString("config")
	config, err := ini.Load(configfile)
	if err != nil {
		return errors.Wrapf(err, "Cannot open config file '%s'", configfile)
	}

	// 2. Open connection to the DB
	db, err := dbConnect(config)
	if err != nil {
		return err
	}

	// 3. Confirm
	fmt.Println("WARNING: Init of the DB will erase all previous records!")
	if !prompter.YesNo("Really init the DB?", false) {
		return nil
	}

	// 4. Initialization of the DB
	return dbInit(db, config)
}
