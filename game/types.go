package game

import (
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/setnicka/sqlxpp"
)

// ErrLogin is returned on failed login
var ErrLogin = errors.Errorf("Unknown login")

// ErrTeamNotFound is returned when team with given ID does not exists
var ErrTeamNotFound = errors.Errorf("Team not found")

// Game holds game config and provides methods to do every action in the game
type Game struct {
	config atomic.Value
	db     *sqlxpp.DB
}

// Team represents team and provides methods on this team
type Team struct {
	tx                 *sqlxpp.Tx // transaction for all DB changes
	now                time.Time  // cached time of time.Now()
	gameConfig         *Config    // configuration of the game
	teamConfig         TeamConfig
	status             TeamStatus
	statusLoaded       bool
	cipherStatus       map[string]CipherStatus
	cipherStatusLoaded bool
}

////////////////////////////////////////////////////////////////////////////////
// DB structs:

// TeamStatus is status of the team saved in DB
type TeamStatus struct {
	Team       string     `db:"team"`
	Lat        float64    `db:"lat"`
	Lon        float64    `db:"lon"`
	LastMoved  *time.Time `db:"last_moved"`
	CooldownTo *time.Time `db:"cooldown_to"`
}

// CipherStatus is status of the cipher for given team (saved in DB)
type CipherStatus struct {
	Team        string     `db:"team"`
	Cipher      string     `db:"cipher"`
	Arrival     time.Time  `db:"arrival"`
	Advance     *time.Time `db:"advance"`
	Hint        *time.Time `db:"hint"`
	Skip        *time.Time `db:"skip"`
	ExtraPoints int        `db:"extra_points"`
}

// TeamLocationEntry is one record from team_location_history table
type TeamLocationEntry struct {
	Team string    `db:"team"`
	Time time.Time `db:"time"`
	Lat  float64   `db:"lat"`
	Lon  float64   `db:"lon"`
}
