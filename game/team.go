package game

import (
	"time"

	"github.com/pkg/errors"
	"github.com/setnicka/sqlxpp"
)

// Team represents team and provides methods on this team
type Team struct {
	game *Game      // link to game
	tx   *sqlxpp.Tx // transaction for all DB changes

	config             TeamConfig
	status             TeamStatus
	statusLoaded       bool
	cipherStatus       map[string]CipherStatus
	cipherStatusLoaded bool
}

// GetTeam returns object which represent one team in game and DB transaction,
// which is used for all changes on the Team. Changes must be committed by
// Commit call on the transaction.
func (g *Game) GetTeam(ID string) (*Team, *sqlxpp.Tx, error) {
	config, found := g.teams[ID]
	if !found {
		return nil, nil, errors.Errorf("Team '%s' not found", ID)
	}
	tx, err := g.db.Begin()
	if err != nil {
		return nil, nil, err
	}
	return &Team{game: g, tx: tx, config: config}, tx, nil
}

// GetStatus load team status from the DB (or returns cached one)
func (t *Team) GetStatus() (TeamStatus, error) {
	if !t.statusLoaded {
		if err := t.tx.GetE(&t.status, "SELECT * FROM team_status WHERE team=$1", t.config.ID); err != nil {
			return TeamStatus{}, err
		}
		t.statusLoaded = true
	}
	return t.status, nil
}

// GetCipherStatus load cipher status of this team from DB (or returns cached one)
func (t *Team) GetCipherStatus() (map[string]CipherStatus, error) {
	if !t.cipherStatusLoaded {
		cipherStatuses := []CipherStatus{}
		if err := t.tx.SelectE(cipherStatuses, "SELECT * FROM cipher_status WHERE team=$1", t.config.ID); err != nil {
			return nil, err
		}
		t.cipherStatus = map[string]CipherStatus{}
		for _, cs := range cipherStatuses {
			t.cipherStatus[cs.Cipher] = cs
		}
		t.cipherStatusLoaded = true
	}
	return t.cipherStatus, nil
}

////////////////////////////////////////////////////////////////////////////////

// GetPosition load current team position from DB (or returns starting position if not set in DB yet)
func (ts *TeamStatus) GetPosition() Point { return Point{Lat: ts.Lat, Lon: ts.Lon} }

// GetDistanceTo returns distance in metres and cooldown duration after this move
func (ts *TeamStatus) GetDistanceTo(target Point) (distance float64, cooldown time.Duration) {
	pos := ts.GetPosition()
	distance = pos.Distance(target)
	cooldown = time.Second * time.Duration(distance/ts.team.game.config.MapSpeed)
	return distance, cooldown
}

////////////////////////////////////////////////////////////////////////////////

// MoveToPosition moves team to given position and updates last_moved and cooldown_to times
func (t *Team) MoveToPosition(target Point) error {
	if _, err := t.GetStatus(); err != nil {
		return err
	}
	if t.status.CooldownTo.After(time.Now()) {
		return errors.Errorf("Could not move now, cooldown to %v", t.status.CooldownTo)
	}
	_, cooldown := t.status.GetDistanceTo(target)
	t.status.LastMoved = time.Now()
	t.status.CooldownTo = time.Now().Add(cooldown)
	t.status.Lat = target.Lat
	t.status.Lon = target.Lon

	return t.tx.Update("team_status", t.status, "WHERE id=:id", nil)
}

// LogCipherArrival adds new CipherStatus to the DB with logged time
func (t *Team) LogCipherArrival(cipher CipherConfig) error {
	if _, err := t.GetCipherStatus(); err != nil {
		return err
	}
	if cs, found := t.cipherStatus[cipher.ID]; found {
		return errors.Errorf("Arrival already logged at %v", cs.Arrival)
	}
	cs := CipherStatus{
		Team:    t.config.ID,
		Cipher:  cipher.ID,
		Arrival: time.Now(),
	}
	t.cipherStatus[cipher.ID] = cs
	return t.game.db.Insert("cipher_status", cs, nil)

	// TODO: log previous ciphers solved?

	// TODO: move to cipher coordinates when not using mode=online-map?
}

func (t *Team) logCipher(cipher CipherConfig, action string) error {
	if _, err := t.GetCipherStatus(); err != nil {
		return err
	}
	cs, found := t.cipherStatus[cipher.ID]
	if !found {
		return errors.Errorf("Cannot %s on not arrived cipher", action)
	}
	field, found := map[string]*time.Time{
		"advance": &cs.Advance,
		"hint":    &cs.Hint,
		"skip":    &cs.Skip,
	}[action]
	if !found {
		return errors.Errorf("Unknown action '%s'", action)
	}
	if !field.IsZero() {
		return errors.Errorf("Already %s at %v, cannot log again", action, field)
	}
	*field = time.Now()
	t.cipherStatus[cipher.ID] = cs
	return t.game.db.Update("cipher_status", cs, "WHERE team=:team AND cipher=:cipher", []string{"team", "cipher"})
}

// LogCipherAdvance logs advance time of the CipherStatus record in DB
func (t *Team) LogCipherAdvance(cipher CipherConfig) error { return t.logCipher(cipher, "advance") }

// LogCipherHint logs hint time of the CipherStatus record in DB
func (t *Team) LogCipherHint(cipher CipherConfig) error { return t.logCipher(cipher, "hint") }

// LogCipherSkip logs skip time of the CipherStatus record in DB
func (t *Team) LogCipherSkip(cipher CipherConfig) error { return t.logCipher(cipher, "skip") }

// SetCipherExtraPoints logs extra points to given CipherStatus
func (t *Team) SetCipherExtraPoints(cipher CipherConfig, extraPoints int) error {
	if _, err := t.GetCipherStatus(); err != nil {
		return err
	}
	cs, found := t.cipherStatus[cipher.ID]
	if !found {
		return errors.Errorf("Cannot set extra points on not arrived cipher")
	}
	cs.ExtraPoints = extraPoints
	t.cipherStatus[cipher.ID] = cs
	return t.game.db.Update("cipher_status", cs, "WHERE team=:team AND cipher=:cipher", []string{"team", "cipher"})
}

// DiscoverCiphers test all not yet discovered ciphers (without CipherStatus in DB)
// and calls LogCipherArrival on all that could be discovered.
func (t *Team) DiscoverCiphers() error {
	if _, err := t.GetStatus(); err != nil {
		return err
	} else if _, err := t.GetCipherStatus(); err != nil {
		return err
	}
	pos := t.status.GetPosition()
	for _, cipher := range t.game.ciphers {
		if _, found := t.cipherStatus[cipher.ID]; found {
			continue // already found
		}
		if cipher.Discoverable(pos, t.cipherStatus) {
			if err := t.LogCipherArrival(cipher); err != nil {
				return err
			}
		}
	}
	return nil
}
