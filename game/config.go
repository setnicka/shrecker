package game

import (
	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/coreos/go-log/log"
	"github.com/go-ini/ini"
	"github.com/pkg/errors"
	"github.com/setnicka/sqlxpp"
)

type gameMode string
type orderMode string

// Modes of the game
const (
	GameNormal      gameMode = "normal"
	GameOnlineCodes          = "online-codes"
	GameOnlineMap            = "online-map"
)

// Order modes
const (
	OrderPoints orderMode = "points"
)

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

	// Time settins
	HintLimit time.Duration `ini:"hint_limit"`
	SkipLimit time.Duration `ini:"skip_limit"`

	// Ordering settings
	OrderMode        orderMode `ini:"order_mode"`
	PointsSolved     int       `ini:"points_solved"`
	PointsSolvedHint int       `ini:"points_solved_hint"`
	PointsSkipped    int       `ini:"points_skipped"`

	ciphers    []CipherConfig
	ciphersMap map[string]CipherConfig
	teams      map[string]TeamConfig
}

// CipherConfig holds configuration of one cipher (parsed from JSON)
type CipherConfig struct {
	ID           string   `json:"id"`
	DependsOn    []string `json:"depends_on,omitempty"` // IDs of ciphers that must be discovered before this one could be discovered (online-map mode)
	StartVisible bool     `json:"start_visible"`        // Cipher is visible from start (online-map mode)
	Name         string   `json:"name"`                 // Displayed name of the cipher
	ArrivalCode  string   `json:"arrival_code"`         // code used on arrival
	ArrivalText  string   `json:"arrival_text"`         // text displayed on the arrival
	AdvanceCode  string   `json:"advance_code"`         // solution code deciphered from the cipher
	AdvanceText  string   `json:"advance_text"`         // text displayed when correct advance code is entered
	HintText     string   `json:"hint_text"`
	SkipText     string   `json:"skip_text"`
	Position     Point    `json:"position"`
	File         string   `json:"file"`
	// Messages    map[string]cipherMessage `json:messages`
}

// TeamConfig is parsed configuration from JSON
type TeamConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Login    string `json:"login"`
	Password string `json:"password"`
}

// Point represent one point on map (with optional radius)
type Point struct {
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Radius int     `json:"radius,omitempty"` // in meters
}

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
				Lat:  config.StartLat,
				Lon:  config.StartLon,
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
	if err := gamecfg.MapTo(&config); err != nil {
		return err
	}

	// Load ciphers
	ciphersFile := gamecfg.Key("ciphers").String()
	ciphersBytes, err := ioutil.ReadFile(ciphersFile)
	if err != nil {
		return errors.Wrapf(err, "Cannot read ciphers from file '%s'", ciphersFile)
	}
	if err := json.Unmarshal(ciphersBytes, &config.ciphers); err != nil {
		return errors.Wrapf(err, "Cannot unmarshal JSON from file '%s'", ciphersFile)
	}
	// create ciphers map and check that IDs are unique
	config.ciphersMap = map[string]CipherConfig{}
	for _, cipher := range config.ciphers {
		if _, found := config.ciphersMap[cipher.ID]; found {
			return errors.Errorf("Config error: Duplicit cipher ID '%s'!", cipher.ID)
		}
		config.ciphersMap[cipher.ID] = cipher
	}
	// check that cipher are unique and depends_on exists
	codes := map[string]CipherConfig{}
	for _, cipher := range config.ciphers {
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
		}
		for _, d := range cipher.DependsOn {
			if _, found := config.ciphersMap[d]; !found {
				return errors.Errorf("Config error: Cipher '%s' depends on '%s' but cipher with this ID does not exists", cipher.ID, d)
			}
		}
	}

	// Load teams
	teamsFile := gamecfg.Key("teams").String()
	teamsBytes, err := ioutil.ReadFile(teamsFile)
	if err != nil {
		return errors.Wrapf(err, "Cannot read teams from file '%s'", teamsFile)
	}
	teamConfigs := []TeamConfig{}
	if err := json.Unmarshal(teamsBytes, &teamConfigs); err != nil {
		return errors.Wrapf(err, "Cannot unmarshal JSON from file '%s'", teamsFile)
	}
	// create teams map and check that IDs are unique
	config.teams = map[string]TeamConfig{}
	for _, team := range teamConfigs {
		if _, found := config.teams[team.ID]; found {
			return errors.Errorf("Config error: Duplicit team ID '%s'!", team.ID)
		}
		config.teams[team.ID] = team
	}

	// Store config
	g.config.Store(config)
	return nil
}
