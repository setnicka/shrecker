package game

import (
	"fmt"
	"time"

	"github.com/coreos/go-log/log"
	"github.com/jmoiron/sqlx"
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
func (t *Team) GetConfig() *TeamConfig { return t.teamConfig }

// GetHash returns current hash representing state of the team
func (t *Team) GetHash() int { return t.gameConfig.teamHash[t.teamConfig.ID] }

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

		IDs := append(t.teamConfig.CompanionIDs, t.teamConfig.ID)
		query, args, err := sqlx.In("SELECT * FROM cipher_status WHERE team IN (?)", IDs)
		if err != nil {
			return nil, err
		}

		if err := t.tx.SelectE(&cipherStatuses, sqlx.Rebind(sqlx.DOLLAR, query), args...); err != nil {
			return nil, err
		}
		t.cipherStatus = map[string]CipherStatus{}
		for _, cs := range cipherStatuses {
			cs.init(t.gameConfig)
			t.cipherStatus[cs.Cipher] = cs
		}
		t.cipherStatusLoaded = true
	}
	return t.cipherStatus, nil
}

// GetLocations loads location history of this team from DB (or returns cached one)
func (t *Team) GetLocations() ([]TeamLocationEntry, error) {
	if !t.locationsLoaded {
		if err := t.tx.SelectE(&t.locations, "SELECT * FROM team_location_history WHERE team=$1 ORDER BY time", t.teamConfig.ID); err != nil {
			return nil, err
		}
		t.locationsLoaded = true
	}
	return t.locations, nil
}

// GetMessages loads messages of this team from DB (or returns cached ones)
func (t *Team) GetMessages() ([]Message, error) {
	if !t.messagesLoaded {
		if err := t.tx.SelectE(&t.messages, "SELECT * FROM messages WHERE team=$1 ORDER BY time DESC", t.teamConfig.ID); err != nil {
			return nil, err
		}
		t.messagesLoaded = true
	}
	return t.messages, nil
}

////////////////////////////////////////////////////////////////////////////////

// TeamStats holds statistics about ciphers of given team
type TeamStats struct {
	FoundCiphers      int
	SolvedCiphers     int
	FoundMiniCiphers  int
	SolvedMiniCiphers int
	FoundSimple       int
	UsedHints         int
	UsedSkips         int
	HintScore         int
}

// GetStats computes statistics about ciphers based on cipher statuses from the DB
func (t *Team) GetStats() (TeamStats, error) {
	statuses, err := t.GetCipherStatus()
	if err != nil {
		return TeamStats{}, err
	}
	stats := TeamStats{}
	for _, cipher := range t.gameConfig.ciphers {
		status, found := statuses[cipher.ID]
		if !found {
			continue
		}
		if cipher.Type == Cipher {
			stats.FoundCiphers++
			if status.Solved != nil {
				stats.SolvedCiphers++
			}
		} else if cipher.Type == MiniCipher {
			stats.FoundMiniCiphers++
			if status.Solved != nil {
				stats.SolvedMiniCiphers++
			}
		} else {
			stats.FoundSimple++
		}
		if status.Hint != nil {
			stats.UsedHints++
		}
		if status.Skip != nil {
			stats.UsedSkips++
		}
		stats.HintScore += status.HintScore
	}
	return stats, nil
}

// SumPoints runs through all ciphers and sum points for them
func (t *Team) SumPoints() (int, error) {
	if _, err := t.GetCipherStatus(); err != nil {
		return 0, err
	}
	sum := 0
	for _, c := range t.cipherStatus {
		sum += c.Points
	}
	return sum, nil
}

// GetDistanceTo returns distance in metres and cooldown duration after this move
func (t *Team) GetDistanceTo(target Point) (distance float64, cooldown time.Duration, err error) {
	status, err := t.GetStatus()
	if err != nil {
		return 0, 0, err
	}

	distance = status.Point.Distance(target)

	cooldown = time.Second * time.Duration(distance/t.gameConfig.MapSpeed)
	return distance, cooldown, nil
}

// increase hash to mark that something with the team changes
func (t *Team) incHash() { t.gameConfig.teamHash[t.teamConfig.ID]++ }

// MapMoveToPosition is used in online map mode and checks cooldown. It internally
// calls LogPosition. Cooldown check should be done by caller.
func (t *Team) MapMoveToPosition(target Point) error {
	if _, err := t.GetStatus(); err != nil {
		return err
	}

	_, cooldown, _ := t.GetDistanceTo(target) // err is checked by GetStatus above
	cooldownTo := t.Now().Add(cooldown)
	t.status.CooldownTo = &cooldownTo

	return t.LogPosition(target) // incHash is inside LogPosition
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
		Team:  t.teamConfig.ID,
		Time:  now,
		Point: pos,
	}, nil); err != nil {
		return err
	}
	log.Infof("Team '%s' (ID '%s') moved to new position %v", t.teamConfig.Name, t.teamConfig.ID, pos)
	t.incHash()
	return t.tx.Update("team_status", t.status, "WHERE team=:team", nil)
}

// TestHintAllowed tests if a hint for given cipher could be issued (used from templates)
func (t *Team) TestHintAllowed(cipher *CipherConfig, status CipherStatus) (bool, string, time.Time) {
	d := t.Now().Sub(status.Arrival)
	stats, _ := t.GetStats()
	title := ""

	switch t.gameConfig.HintMode {
	case HintsFree:
		if d < t.gameConfig.HintLimit {
			return false, fmt.Sprintf("Zatím uběhlo jen %v od příchodu na šifru, nápověda je dostupná až po %v od příchodu", d.Round(time.Second), t.gameConfig.HintLimit), status.Arrival.Add(t.gameConfig.HintLimit)
		}
	case HintsMiniCiphers:
		if stats.HintScore <= 0 && !t.gameConfig.HintMCAllowNegative {
			return false, fmt.Sprintf("Nemáte žádné nepoužité šifřičky pro zisk nápovědy, nejdříve nějakou vyluštěte"), time.Time{}
		}
		if d < t.gameConfig.HintLimit {
			return false, fmt.Sprintf("Zatím uběhlo jen %v od příchodu na šifru, nápověda je dostupná až po %v od příchodu", d.Round(time.Second), t.gameConfig.HintLimit), status.Arrival.Add(t.gameConfig.HintLimit)
		}

		if stats.HintScore <= 0 {
			title = "Nemáte žádné nepoužité šifřičky, ale můžete si vzít nápovědu na dluh"
		}
	}
	return true, title, time.Time{}
}

// RequestHint tests if hint is possible for this cipher and if so it logs it
// and returns message which should be returned to the team
func (t *Team) RequestHint(cipher *CipherConfig, status CipherStatus) (string, string, bool, error) {
	if status.Skip != nil {
		return "info", "Tuto šifru jste již přeskočili", false, nil
	} else if status.Solved != nil {
		return "info", "Tuto šifru jste již vyřešili", false, nil
	} else if cipher.HintText == "" {
		return "info", "Tato šifra nemá nápovědu", false, nil
	} else if status.Hint == nil {
		if allowed, reason, _ := t.TestHintAllowed(cipher, status); !allowed {
			return "error", reason, false, nil
		}
		err := t.LogCipherHint(cipher)
		return "success", fmt.Sprintf("Nápověda: %s", cipher.HintText), true, err
	}
	return "success", fmt.Sprintf("Nápověda: %s", cipher.HintText), false, nil
}

// TestSkipAllowed tests if a skip for given cipher could be done (used from templates)
func (t *Team) TestSkipAllowed(cipher *CipherConfig, status CipherStatus) (bool, string, time.Time) {
	d := t.Now().Sub(status.Arrival)
	if d < t.gameConfig.SkipLimit {
		return false, fmt.Sprintf("Zatím uběhlo jen %v od příchodu na šifru, přeskočení je dostupné až po %v od příchodu", d.Round(time.Second), t.gameConfig.SkipLimit), status.Arrival.Add(t.gameConfig.SkipLimit)
	}
	return true, "", time.Time{}
}

// RequestSkip tests if skip is possible for this cipher and if so it logs it
// and returns message which should be returned to the team
func (t *Team) RequestSkip(cipher *CipherConfig, status CipherStatus) (string, string, bool, error) {
	if status.Solved != nil {
		return "info", "Tuto šifru jste již vyřešili", false, nil
	} else if cipher.SkipText == "" {
		return "info", "Tato šifra nelze přeskočit", false, nil
	} else if status.Skip == nil {
		if allowed, reason, _ := t.TestSkipAllowed(cipher, status); !allowed {
			return "error", reason, false, nil
		}
		err := t.LogCipherSkip(cipher)
		return "success", fmt.Sprintf("Další stanoviště: %s", cipher.SkipText), true, err
	}
	return "success", fmt.Sprintf("Další stanoviště: %s", cipher.SkipText), false, nil
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
	if err := t.tx.Insert("cipher_status", cs, nil); err != nil {
		return err
	}
	log.Infof("Team '%s' (ID '%s') discovered cipher '%s'", t.teamConfig.Name, t.teamConfig.ID, cipher.ID)
	defer t.incHash()

	// log previous ciphers solved
	for _, prevID := range cipher.LogSolved {
		prevCipher := t.gameConfig.ciphersMap[prevID]
		prevCipherStatus := t.cipherStatus[prevID]
		_, found := t.cipherStatus[prevID]
		if prevCipherStatus.Solved == nil && prevCipherStatus.Skip == nil && found {
			if err := t.LogCipherSolved(prevCipher); err != nil {
				return err
			}
		}
	}

	if t.gameConfig.AutologPosition && !cipher.Position.Point.IsZero() {
		return t.LogPosition(cipher.Position.Point)
	}
	return nil
}

func (t *Team) logCipher(cipher *CipherConfig, action string, callback func(*CipherStatus)) error {
	if _, err := t.GetCipherStatus(); err != nil {
		return err
	}
	cs, found := t.cipherStatus[cipher.ID]
	if !found {
		return errors.Errorf("Cannot %s on not arrived cipher", action)
	}
	field, found := map[string]**time.Time{
		"solved": &cs.Solved,
		"hint":   &cs.Hint,
		"skip":   &cs.Skip,
	}[action]
	if !found {
		return errors.Errorf("Unknown action '%s'", action)
	}
	if (*field) != nil {
		return errors.Errorf("Already %s at %v, cannot log again", action, field)
	}
	now := t.Now()
	*field = &now
	if callback != nil {
		callback(&cs)
	}
	t.cipherStatus[cipher.ID] = cs
	log.Infof("Team '%s' (ID '%s'): %s on cipher '%s'", t.teamConfig.Name, t.teamConfig.ID, action, cipher.ID)
	t.incHash()
	return t.tx.Update("cipher_status", cs, "WHERE team=:team AND cipher=:cipher", []string{"team", "cipher"})
}

// LogCipherSolved logs solved time of the CipherStatus record in DB
func (t *Team) LogCipherSolved(cipher *CipherConfig) error {
	return t.logCipher(cipher, "solved", func(status *CipherStatus) {
		if cipher.Type == MiniCipher && t.gameConfig.HasMiniCipherHints() {
			status.HintScore++
		}
		log.Infof("Team '%s' (ID '%s'): hint_score++ on solved mini-cipher cipher '%s'", t.teamConfig.Name, t.teamConfig.ID, cipher.ID)
	})
}

// LogCipherHint logs hint time of the CipherStatus record in DB
func (t *Team) LogCipherHint(cipher *CipherConfig) error {
	stats, err := t.GetStats()
	if err != nil {
		return err
	}

	return t.logCipher(cipher, "hint", func(status *CipherStatus) {
		if t.gameConfig.HasMiniCipherHints() {
			if stats.HintScore > 0 {
				status.HintScore--
				log.Infof(
					"Team '%s' (ID '%s'): Used hint on cipher '%s', hint_score-- (new hint_score: %d)",
					t.teamConfig.Name, t.teamConfig.ID, cipher.ID, status.HintScore,
				)
			} else {
				status.HintScore -= t.gameConfig.HintMCNegativePrice
				log.Infof(
					"Team '%s' (ID '%s'): Used hint on cipher '%s', hint_score decreased by %d (new hint_score: %d)",
					t.teamConfig.Name, t.teamConfig.ID, cipher.ID, t.gameConfig.HintMCNegativePrice, status.HintScore,
				)
			}
		}
	})
}

// LogCipherSkip logs skip time of the CipherStatus record in DB
func (t *Team) LogCipherSkip(cipher *CipherConfig) error { return t.logCipher(cipher, "skip", nil) }

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
	t.incHash()
	return t.tx.Update("cipher_status", cs, "WHERE team=:team AND cipher=:cipher", []string{"team", "cipher"})
}

// AddHintScore adds given value to the CipherStatus field HintScore
func (t *Team) AddHintScore(cipher CipherConfig, add int) error {
	if _, err := t.GetCipherStatus(); err != nil {
		return err
	}
	cs, found := t.cipherStatus[cipher.ID]
	if !found {
		return errors.Errorf("Cannot add hint score on not arrived cipher")
	}
	cs.HintScore += add
	t.cipherStatus[cipher.ID] = cs
	t.incHash()
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
	for _, cipher := range t.gameConfig.ciphers {
		if _, found := t.cipherStatus[cipher.ID]; found {
			continue // already found
		}
		if cipher.StartVisible || (t.gameConfig.Mode == GameOnlineMap && cipher.DiscoverableFromPoint(t.status.Point, t.cipherStatus)) {
			discovered = append(discovered, cipher)
			if err := t.LogCipherArrival(cipher); err != nil {
				return nil, err
			}
		}
	}
	return discovered, nil
}
