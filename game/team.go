package game

import (
	"time"

	"github.com/coreos/go-log/log"
	"github.com/pkg/errors"
)

// Now is used to cache same time for all requests
func (t *Team) Now() time.Time {
	if t.now.IsZero() {
		t.now = time.Now()
	}
	return t.now
}

// GetConfig returns team config
func (t *Team) GetConfig() *TeamConfig { return &t.teamConfig }

// GetStatus load team status from the DB (or returns cached one)
func (t *Team) GetStatus() (*TeamStatus, error) {
	if !t.statusLoaded {
		if err := t.tx.GetE(&t.status, "SELECT * FROM team_status WHERE team=$1", t.teamConfig.ID); err != nil {
			return nil, err
		}
		t.statusLoaded = true
	}
	return &t.status, nil
}

// GetCipherStatus load cipher status of this team from DB (or returns cached one)
func (t *Team) GetCipherStatus() (map[string]CipherStatus, error) {
	if !t.cipherStatusLoaded {
		cipherStatuses := []CipherStatus{}
		if err := t.tx.SelectE(&cipherStatuses, "SELECT * FROM cipher_status WHERE team=$1", t.teamConfig.ID); err != nil {
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
func (ts TeamStatus) GetPosition() Point { return Point{Lat: ts.Lat, Lon: ts.Lon} }

////////////////////////////////////////////////////////////////////////////////

// GetDistanceTo returns distance in metres and cooldown duration after this move
func (t *Team) GetDistanceTo(target Point) (distance float64, cooldown time.Duration, err error) {
	status, err := t.GetStatus()
	if err != nil {
		return 0, 0, err
	}

	pos := status.GetPosition()
	distance = pos.Distance(target)

	cooldown = time.Second * time.Duration(distance/t.gameConfig.MapSpeed)
	return distance, cooldown, nil
}

// MapMoveToPosition is used in online map mode and checks cooldown. It internally
// calls LogPosition
func (t *Team) MapMoveToPosition(target Point) error {
	if _, err := t.GetStatus(); err != nil {
		return err
	}
	if t.status.CooldownTo != nil && t.status.CooldownTo.After(t.Now()) {
		return errors.Errorf("Could not move now, cooldown to %v", t.status.CooldownTo)
	}
	_, cooldown, _ := t.GetDistanceTo(target) // err is checked by GetStatus above
	cooldownTo := t.Now().Add(cooldown)
	t.status.CooldownTo = &cooldownTo

	return t.LogPosition(target)
}

// LogPosition saves position to team status and logs it into team_location_history
func (t *Team) LogPosition(pos Point) error {
	if _, err := t.GetStatus(); err != nil {
		return err
	}
	now := t.Now()
	t.status.LastMoved = &now
	t.status.Lat = pos.Lat
	t.status.Lon = pos.Lon

	if err := t.tx.Insert("team_location_history", TeamLocationEntry{
		Team: t.teamConfig.ID,
		Time: now,
		Lat:  pos.Lat,
		Lon:  pos.Lon,
	}, nil); err != nil {
		return err
	}
	log.Infof("Team '%s' (ID '%s') moved to new position %v", t.teamConfig.Name, t.teamConfig.ID, pos)
	return t.tx.Update("team_status", t.status, "WHERE team=:team", nil)
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
		Team:    t.teamConfig.ID,
		Cipher:  cipher.ID,
		Arrival: t.Now(),
	}
	t.cipherStatus[cipher.ID] = cs
	// TODO: log previous ciphers solved?
	// TODO: move to cipher coordinates when not using mode=online-map?
	log.Infof("Team '%s' (ID '%s') discovered cipher '%s'", t.teamConfig.Name, t.teamConfig.ID, cipher.ID)

	return t.tx.Insert("cipher_status", cs, nil)
}

func (t *Team) logCipher(cipher CipherConfig, action string) error {
	if _, err := t.GetCipherStatus(); err != nil {
		return err
	}
	cs, found := t.cipherStatus[cipher.ID]
	if !found {
		return errors.Errorf("Cannot %s on not arrived cipher", action)
	}
	field, found := map[string]**time.Time{
		"advance": &cs.Advance,
		"hint":    &cs.Hint,
		"skip":    &cs.Skip,
	}[action]
	if !found {
		return errors.Errorf("Unknown action '%s'", action)
	}
	if (*field) != nil {
		return errors.Errorf("Already %s at %v, cannot log again", action, field)
	}
	now := t.Now()
	*field = &now
	t.cipherStatus[cipher.ID] = cs
	log.Infof("Team '%s' (ID '%s'): %s on cipher '%s'", t.teamConfig.Name, t.teamConfig.ID, action, cipher.ID)
	return t.tx.Update("cipher_status", cs, "WHERE team=:team AND cipher=:cipher", []string{"team", "cipher"})
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
	return t.tx.Update("cipher_status", cs, "WHERE team=:team AND cipher=:cipher", []string{"team", "cipher"})
}

// DiscoverCiphers test all not yet discovered ciphers (without CipherStatus in DB)
// and calls LogCipherArrival on all that could be discovered.
func (t *Team) DiscoverCiphers() ([]CipherConfig, error) {
	if _, err := t.GetStatus(); err != nil {
		return nil, err
	} else if _, err := t.GetCipherStatus(); err != nil {
		return nil, err
	}
	discovered := []CipherConfig{}
	pos := t.status.GetPosition()
	for _, cipher := range t.gameConfig.ciphers {
		if _, found := t.cipherStatus[cipher.ID]; found {
			continue // already found
		}
		if cipher.StartVisible || cipher.Discoverable(pos, t.cipherStatus) {
			discovered = append(discovered, cipher)
			if err := t.LogCipherArrival(cipher); err != nil {
				return nil, err
			}
		}
	}
	return discovered, nil
}
