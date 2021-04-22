package main

import (
	"fmt"
	"io/ioutil"

	"github.com/go-ini/ini"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	"github.com/setnicka/sqlxpp"
)

// Connect to the DB specified in 'database' section of given config
func dbConnect(config *ini.File) (*sqlxpp.DB, error) {
	dbcfg := config.Section("database")
	if dbcfg == nil {
		return nil, errors.Errorf("Config file does not contain database section")
	}

	dbType := dbcfg.Key("type").String()
	if dbType == "postgres" {
		connStr := fmt.Sprintf("user=%s password=%s dbname=%s", dbcfg.Key("user").String(), dbcfg.Key("password").String(), dbcfg.Key("dbname").String())
		db, err := sqlx.Open("postgres", connStr)
		if err != nil {
			return nil, errors.Wrap(err, "Cannot open connection to the database")
		}
		if err := db.Ping(); err != nil {
			return nil, errors.Wrap(err, "Cannot connect to the database")
		}
		return sqlxpp.New(db), nil
	}
	return nil, errors.Errorf("Unknown DB type '%s' in config file", dbType)
}

// Init DB with SQL schema from 'database.schema'
func dbInit(db *sqlxpp.DB, config *ini.File) error {
	dbcfg := config.Section("database")
	if dbcfg == nil {
		return errors.Errorf("Config file does not contain database section")
	}

	schema, err := ioutil.ReadFile(dbcfg.Key("schema").String())
	if err != nil {
		return errors.Wrap(err, "Cannot read DB schema from file")
	}

	_, err = db.Exec(string(schema))
	return errors.Wrap(err, "Cannot init the DB")
}
