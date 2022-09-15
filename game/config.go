package game

import (
	"encoding/json"
	"io/ioutil"
	"math/rand"
	"time"

	"github.com/coreos/go-log/log"
	"github.com/go-ini/ini"
	"github.com/pkg/errors"
	"github.com/setnicka/sqlxpp"
)

type gameMode string
type orderMode string
type cipherType string

// Modes of the game
const (
	GameNormal      gameMode = "normal"
	GameNormalMap            = "normal-map"
	GameOnlineCodes          = "online-codes"
	GameOnlineMap            = "online-map"
)

// Order modes
const (
	OrderPoints orderMode = "points"
)

// Cipher types
const (
	Cipher     cipherType = "cipher"      // normal cipher with all features
	MiniCipher cipherType = "mini-cipher" // special type of cipher with less requirements (no need for advance text, ...), is counted in a special counter for mini ciphers
	Simple     cipherType = "simple"      // has only arrival code, no hints, skips, ...
)

func (ct *cipherType) UnmarshalJSON(data []byte) (err error) {
	var ctp string
	if err := json.Unmarshal(data, &ctp); err != nil {
		return err
	}
	switch cipherType(ctp) {
	case Cipher, "":
		*ct = Cipher
	case MiniCipher:
		*ct = MiniCipher
	case Simple:
		*ct = Simple
	default:
		return errors.Errorf("unknown cipher type '%s'", ctp)
	}
	return nil
}

// Config holds parsed game configuration from the ini file
type Config struct {
	Mode          gameMode  `ini:"mode"`
	Start         time.Time `ini:"start"`
	End           time.Time `ini:"end"`
	CiphersFolder string    `ini:"ciphers_folder"`

	// Map settings
	StartLat       float64 `ini:"start_lat"`
	StartLon       float64 `ini:"start_lon"`
	MapDefaultZoom int     `ini:"map_default_zoom"`
	MapSpeed       float64 `ini:"map_speed"`

	AutologPosition bool `ini:"autolog_position"`

	// Time settins
	HintLimit time.Duration `ini:"hint_limit"`
	SkipLimit time.Duration `ini:"skip_limit"`

	OrderPickupMessage bool `ini:"order_pickup_message"`
	LastPickupMessage  bool `ini:"last_pickup_message"`

	// Ordering settings
	OrderMode        orderMode `ini:"order_mode"`
	PointsSolved     int       `ini:"points_solved"`
	PointsSolvedHint int       `ini:"points_solved_hint"`
	PointsSkipped    int       `ini:"points_skipped"`

	ciphers    []CipherConfig
	ciphersMap map[string]*CipherConfig
	teams      map[string]*TeamConfig
	teamHash   map[string]int // changed everytime when something for the team changes
}

// CipherConfig holds configuration of one cipher (parsed from JSON)
type CipherConfig struct {
	ID              string      `json:"id"`
	Type            cipherType  `json:"type"`
	NotCipher       bool        `json:"not_cipher"`           // used for PDF with game rules, ...
	DependsOn       [][]string  `json:"depends_on,omitempty"` // IDs of ciphers that must be discovered before this one could be discovered ((a AND b AND c) OR (d AND e) OR (f))
	LogSolved       []string    `json:"log_solved"`           // list of ciphers to log as solved when this one is discovered
	SharedStandings []string    `json:"shared_standings"`     // Cipher has share stanging with another cipher (used when the standing is showed to team)
	StartVisible    bool        `json:"start_visible"`        // Cipher is visible from start (online-map mode)
	Name            string      `json:"name"`                 // Displayed name of the cipher
	ArrivalCode     string      `json:"arrival_code"`         // code used on arrival
	ArrivalText     string      `json:"arrival_text"`         // text displayed on the arrival
	AdvanceCode     string      `json:"advance_code"`         // solution code deciphered from the cipher
	AdvanceText     string      `json:"advance_text"`         // text displayed when correct advance code is entered
	HintText        string      `json:"hint_text"`
	SkipText        string      `json:"skip_text"`
	Position        PointRadius `json:"position"`
	File            string      `json:"file"`
	// Messages    map[string]cipherMessage `json:messages`
}

// TeamConfig is parsed configuration from JSON
type TeamConfig struct {
	ID           string            `json:"id"`
	CompanionIDs []string          `json:"companion_ids"` // IDs of other teams that should share information
	Name         string            `json:"name"`
	Jitsi        string            `json:"jitsi"` // link for Jitsi room (online-map mode)
	Login        string            `json:"login"`
	Password     string            `json:"password"`
	SMSCode      string            `json:"sms_code"` // used in SMS to identify this team
	Members      map[string]string `json:"members"`  // maps name -> email or name -> phone number
}

// PointRadius represent one point on map with radius
type PointRadius struct {
	Point
	Radius int `json:"radius,omitempty"` // in meters
}

////////////////////////////////////////////////////////////////////////////////

// Initial checks for all teams (create team_status, discover ciphers)
func (g *Game) initStatus() error {
	config := g.GetConfig()
	tx, err := g.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()
	for _, teamConfig := range config.teams {
		team := Team{teamConfig: teamConfig, gameConfig: &config, tx: tx, now: now}
		if _, err := team.GetStatus(); sqlxpp.IsNotFoundError(err) {
			// create new team status
			log.Printf("Creating team status record for team '%s' with ID '%s'", teamConfig.Name, teamConfig.ID)
			err = tx.Insert("team_status", TeamStatus{
				Team: teamConfig.ID,
				Point: Point{
					Lat: config.StartLat,
					Lon: config.StartLon,
				},
			}, nil)
			if err == nil && config.Mode == GameOnlineMap {
				// Add initial team position
				err = team.LogPosition(Point{Lat: config.StartLat, Lon: config.StartLon})
			}
		}
		if err != nil {
			return err
		}
		if config.Mode == GameOnlineMap {
			// Discover ciphers from this starting position
			if _, err := team.DiscoverCiphers(); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (g *Game) loadConfig(globalConfig *ini.File) error {
	gamecfg := globalConfig.Section("game")
	if gamecfg == nil {
		return errors.Errorf("Config file does not contain game section")
	}

	var config Config
	if err := gamecfg.StrictMapTo(&config); err != nil {
		return err
	}

	if err := config.loadCiphers(gamecfg.Key("ciphers").String()); err != nil {
		return err
	}

	// Load teams
	if err := config.loadTeams(gamecfg.Key("teams").String()); err != nil {
		return err
	}

	// Store config
	g.config.Store(config)
	return nil
}

func (c *Config) loadCiphers(ciphersFile string) error {
	ciphersBytes, err := ioutil.ReadFile(ciphersFile)
	if err != nil {
		return errors.Wrapf(err, "Cannot read ciphers from file '%s'", ciphersFile)
	}
	if err := json.Unmarshal(ciphersBytes, &c.ciphers); err != nil {
		return errors.Wrapf(err, "Cannot unmarshal JSON from file '%s'", ciphersFile)
	}
	// create ciphers map and check that IDs are unique
	c.ciphersMap = map[string]*CipherConfig{}
	for i := range c.ciphers {
		cipher := &c.ciphers[i]
		if _, found := c.ciphersMap[cipher.ID]; found {
			return errors.Errorf("Config error: Duplicit cipher ID '%s'!", cipher.ID)
		}
		if cipher.Type == "" {
			cipher.Type = Cipher
		}
		c.ciphersMap[cipher.ID] = cipher
	}
	// check that cipher codes are unique, all texts are there and ciphers in depends_on and log_solved exists
	codes := map[string]CipherConfig{}
	for _, cipher := range c.ciphers {
		if cipher.Type == Simple {
			if cipher.AdvanceCode != "" || cipher.HintText != "" || cipher.SkipText != "" {
				return errors.Errorf("Config error: Cipher '%s' could not have hint, skip or advance code (because its type is 'simple')!", cipher.ID)
			}
		}

		if cipher.ArrivalCode != "" {
			if cipher.ArrivalCode == cipher.AdvanceCode {
				return errors.Errorf("Config error: Cipher '%s' has same arrival and advance codes '%s'!", cipher.ID, cipher.ArrivalCode)
			}
			if otherCipher, found := codes[cipher.ArrivalCode]; found {
				return errors.Errorf("Config error: Ciphers '%s' and '%s' uses same code '%s'!", otherCipher.ID, cipher.ID, cipher.ArrivalCode)
			}
			codes[cipher.ArrivalCode] = cipher
		}
		if cipher.AdvanceCode != "" {
			if otherCipher, found := codes[cipher.AdvanceCode]; found {
				return errors.Errorf("Config error: Ciphers '%s' and '%s' uses same code '%s'!", otherCipher.ID, cipher.ID, cipher.AdvanceCode)
			}
			codes[cipher.AdvanceCode] = cipher
			if cipher.AdvanceText == "" && cipher.Type != MiniCipher {
				return errors.Errorf("Config error: Cipher '%s' has advance_code but missing advance_text!", cipher.ID)
			}
		}
		for _, variant := range cipher.DependsOn {
			for _, d := range variant {
				if _, found := c.ciphersMap[d]; !found {
					return errors.Errorf("Config error: Cipher '%s' depends on '%s' but cipher with this ID does not exists", cipher.ID, d)
				}
			}
		}
		for _, id := range cipher.LogSolved {
			if _, found := c.ciphersMap[id]; !found {
				return errors.Errorf("Config error: Cipher '%s' has ID '%s' in 'log_solved' field but cipher with this ID does not exists", cipher.ID, id)
			}
		}
		for _, id := range cipher.SharedStandings {
			if _, found := c.ciphersMap[id]; !found {
				return errors.Errorf("Config error: Cipher '%s' has ID '%s' in 'shared_standings' field but cipher with this ID does not exists", cipher.ID, id)
			}
		}
	}

	return nil
}

func (c *Config) loadTeams(teamsFile string) error {
	teamsBytes, err := ioutil.ReadFile(teamsFile)
	if err != nil {
		return errors.Wrapf(err, "Cannot read teams from file '%s'", teamsFile)
	}
	teamConfigs := []*TeamConfig{}
	if err := json.Unmarshal(teamsBytes, &teamConfigs); err != nil {
		return errors.Wrapf(err, "Cannot unmarshal JSON from file '%s'", teamsFile)
	}
	// create teams map and check that IDs, logins and SMS codes are unique
	c.teams = map[string]*TeamConfig{}
	c.teamHash = map[string]int{}
	logins := map[string]string{}
	smsCodes := map[string]string{}
	for _, team := range teamConfigs {
		if _, found := c.teams[team.ID]; found {
			return errors.Errorf("Config error: Duplicit team ID '%s'!", team.ID)
		}
		c.teams[team.ID] = team
		c.teamHash[team.ID] = rand.Int() // init with random hash to let know if something with the team changed

		if otherID, found := logins[team.Login]; found {
			return errors.Errorf("Config error: Teams '%s' and '%s' have same login '%s'!", team.ID, otherID, team.Login)
		}
		logins[team.Login] = team.ID

		if otherID, found := smsCodes[team.SMSCode]; found {
			return errors.Errorf("Config error: Teams '%s' and '%s' have same SMS code '%s'!", team.ID, otherID, team.SMSCode)
		}
		smsCodes[team.SMSCode] = team.ID
	}

	// Check companions
	for _, team := range c.teams {
		for _, companionID := range team.CompanionIDs {
			if companionID == team.ID {
				return errors.Errorf("Config error: Team '%s' cannot have itself as a companion!", team.ID)
			}
			if _, found := c.teams[companionID]; !found {
				return errors.Errorf("Config error: Team '%s' has unknown companion '%s'!", team.ID, companionID)
			}
		}
	}

	return nil
}
