package game

import (
	"github.com/pkg/errors"
	"gopkg.in/ini.v1"
)

// New creates new Game and loads configuration
func New(config *ini.File) (*Game, error) {
	gamecfg := config.Section("game")
	if gamecfg == nil {
		return nil, errors.Errorf("Config file does not contain game section")
	}

	// Load config
	g := &Game{}
	if err := gamecfg.MapTo(&g.config); err != nil {
		return nil, err
	}

	// TODO: load ciphers and teams

	// TODO init team status for all teams if not exists

	return g, nil
}
