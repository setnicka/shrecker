package game

import (
	"time"

	"github.com/setnicka/sqlxpp"
)

// Game holds game config and provides methods to do every action in the game
type Game struct {
	ciphers    []CipherConfig
	ciphersMap map[string]CipherConfig
	teams      map[string]TeamConfig

	db     *sqlxpp.DB
	config Config
}

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
	Mode  gameMode  `ini:"mode"`
	Start time.Time `ini:"start"`
	End   time.Time `ini:"end"`

	// Map settings
	StartLat       float64 `ini:"start_lat"`
	StartLon       float64 `ini:"start_lon"`
	MapDefaultZoom int     `ini:"map_default_zoom"`
	MapSpeed       float64 `ini:"map_speed"`

	// Ordering settings
	OrderMode        orderMode `ini:"order_mode"`
	PointsSolved     int       `ini:"points_solved"`
	PointsSolvedHint int       `ini:"points_solved_hint"`
	PointsSkipped    int       `ini:"points_skipped"`
}

// Point represent one point on map (with optional radius)
type Point struct {
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Radius int     `json:"radius,omitempty"` // in meters
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
	// Messages    map[string]cipherMessage `json:messages`
}

// type cipherMessage struct {
// 	Minutes int `json:minutes`
// }

// TeamConfig is parsed configuration from JSON
type TeamConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Login    string `json:"login"`
	Password string `json:"password"`
}

// TeamStatus is status of the team saved in DB
type TeamStatus struct {
	Team       string    `db:"team"`
	Lat        float64   `db:"lat"`
	Lon        float64   `db:"lon"`
	LastMoved  time.Time `db:"last_moved"`
	CooldownTo time.Time `db:"cooldown_to"`

	team *Team // internal link
}

// CipherStatus is status of the cipher for given team (saved in DB)
type CipherStatus struct {
	Team        string    `db:"team"`
	Cipher      string    `db:"cipher"`
	Arrival     time.Time `db:"arrival"`
	Advance     time.Time `db:"advance"`
	Hint        time.Time `db:"hint"`
	Skip        time.Time `db:"skip"`
	ExtraPoints int       `db:"extra_points"`
}
