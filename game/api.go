package game

import (
	"context"
	"time"

	"github.com/go-ini/ini"
	"github.com/jmoiron/sqlx"
	"github.com/setnicka/sqlxpp"
)

// New creates new Game and loads configuration
func New(globalConfig *ini.File, db *sqlxpp.DB) (*Game, error) {
	g := &Game{db: db}
	if err := g.loadConfig(globalConfig); err != nil {
		return nil, err
	}
	if err := g.initStatus(); err != nil {
		return nil, err
	}
	return g, nil
}

// GetConfig returns game config at this moment
func (g *Game) GetConfig() Config {
	return g.config.Load().(Config)
}

// GetTeamTx returns object which represent one team in game and DB transaction,
// which is used for all changes on the Team. Changes must be committed by
// Commit call on the transaction.
func (g *Game) GetTeamTx(ctx context.Context, ID string) (*Team, *sqlxpp.Tx, *Config, error) {
	gameConfig := g.GetConfig()
	team, found := gameConfig.teams[ID]
	if !found {
		return nil, nil, &gameConfig, ErrTeamNotFound
	}
	tx, err := g.db.BeginCtx(ctx)
	if err != nil {
		return nil, nil, &gameConfig, err
	}
	return &Team{gameConfig: &gameConfig, tx: tx, teamConfig: team}, tx, &gameConfig, nil
}

// LoginTeam returns Team with given login and password or fails with ErrLogin
// when login and password does not match any team.
func (g *Game) LoginTeam(login, password string) (*Team, *Config, error) {
	gameConfig := g.GetConfig()
	for _, team := range gameConfig.teams {
		if team.Login == login && team.Password == password {
			return &Team{gameConfig: &gameConfig, teamConfig: team}, &gameConfig, nil
		}
	}
	return nil, &gameConfig, ErrLogin
}

// GetAll returns all teams, game config and DB transaction,
// which is used for all changes on the Teams. Changes must be committed by
// Commit call on the transaction.
func (g *Game) GetAll(ctx context.Context, loadStatus, loadCiphers, loadLocations bool) (map[string]*Team, *sqlxpp.Tx, *Config, error) {
	gameConfig := g.GetConfig()
	tx, err := g.db.BeginCtx(ctx)
	now := time.Now()
	if err != nil {
		return nil, nil, nil, err
	}

	teams := map[string]*Team{}
	teamIDs := []string{}
	for _, t := range gameConfig.teams {
		teams[t.ID] = &Team{gameConfig: &gameConfig, tx: tx, teamConfig: t, now: now, cipherStatus: map[string]CipherStatus{}}
		teamIDs = append(teamIDs, t.ID)
	}

	if loadStatus {
		statuses := []TeamStatus{}
		query, args, err := sqlx.In("SELECT * FROM team_status WHERE team IN (?)", teamIDs)
		if err != nil {
			return nil, nil, nil, err
		}
		if err := tx.SelectE(&statuses, tx.Rebind(query), args...); err != nil {
			return nil, nil, nil, err
		}
		for _, status := range statuses {
			teams[status.Team].status = status
			teams[status.Team].statusLoaded = true
		}
	}
	if loadCiphers {
		cipherStatuses := []CipherStatus{}
		query, args, err := sqlx.In("SELECT * FROM cipher_status WHERE team IN (?)", teamIDs)
		if err != nil {
			return nil, nil, nil, err
		}
		if err := tx.SelectE(&cipherStatuses, tx.Rebind(query), args...); err != nil {
			return nil, nil, nil, err
		}
		for _, cs := range cipherStatuses {
			cs.init(&gameConfig)
			teams[cs.Team].cipherStatus[cs.Cipher] = cs
			teams[cs.Team].cipherStatusLoaded = true
		}
	}
	if loadLocations {
		locationEntries := []TeamLocationEntry{}
		query, args, err := sqlx.In("SELECT * FROM team_location_history WHERE team IN (?) ORDER BY time", teamIDs)
		if err != nil {
			return nil, nil, nil, err
		}
		if err := tx.SelectE(&locationEntries, tx.Rebind(query), args...); err != nil {
			return nil, nil, nil, err
		}
		for _, entry := range locationEntries {
			teams[entry.Team].locations = append(teams[entry.Team].locations, entry)
			teams[entry.Team].locationsLoaded = true
		}
	}

	return teams, tx, &gameConfig, nil
}
