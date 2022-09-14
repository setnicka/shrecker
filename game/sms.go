package game

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"github.com/pkg/errors"
)

const (
	codeSifricka = "SIFRICKA" // SIFRICKA <CODE1> <CODE2> <...>	- NOT IMPLEMENTED YET
	codeHint     = "HINT"     // HINT <CODE> ...
	codeHintAlt  = "HELP"     // HELP <CODE> ...
	codeSkip     = "SKIP"     // SKIP <CODE> ...

	actionHint    = codeHint
	actionSkip    = codeSkip
	actionArrive  = "ARRIVE"
	actionAdvance = "ADVANCE"
)

// ProcessMessage parses message from SMS or from web input and does some actions
func (t *Team) ProcessMessage(text string, sender string, smsID int) (string, string, error) {
	// 0. Check smsID
	if smsID > 0 {
		var msg Message
		err := t.tx.Get(&msg, "SELECT * FROM messages WHERE sms_id=$1", smsID)
		if err == nil || !errors.Is(err, sql.ErrNoRows) {
			return "", "", errors.Errorf("SMS s tímto smsid již byla zpracována.")
		}
	}

	action := actionArrive
	parts := strings.SplitN(strings.TrimSpace(text), " ", 2)
	code := strings.TrimSpace(strings.ToUpper(parts[0]))

	if code == "" {
		return "error", "Schází kód šifry", nil
	}

	log.Printf("Processing message '%s' from team %s with code '%s'", text, t.teamConfig.ID, code)

	// 1. Handle special
	if code == codeSifricka {
		if len(parts) == 1 {
			return "error", "Schází kódy šifřiček", nil
		}
		codes := strings.Fields(parts[1])
		if len(codes) == 0 {
			return "error", "Schází kódy šifřiček", nil
		}
		return t.processSifricky(codes)
	}

	// 2. Split parts of message
	if code == codeHintAlt {
		code = codeHint
	}

	if code == codeHint || code == codeSkip {
		if len(parts) == 1 {
			return "error", "Neplatný tvar zprávy, schází kód stanoviště", nil
		}
		parts = strings.SplitN(strings.TrimSpace(parts[1]), " ", 2)
		action = code
		code = strings.TrimSpace(strings.ToUpper(parts[0]))
	}

	// 3. Get cipher by code
	var cipher CipherConfig
	found := false
	for _, c := range t.gameConfig.ciphers {
		if strings.ToUpper(c.ArrivalCode) == code {
			cipher = c
			found = true
			break
		} else if action == actionArrive && strings.ToUpper(c.AdvanceCode) == code {
			cipher = c
			found = true
			action = actionAdvance
			break
		}
	}

	// Helper for logging the message into DB
	msg := func(msgType string, msg string, a ...interface{}) (string, string, error) {
		resp := fmt.Sprintf(msg, a...)

		err := t.tx.Insert("messages", Message{
			Team:        t.teamConfig.ID,
			Cipher:      cipher.ID,
			Time:        t.Now(),
			PhoneNumber: sender,
			SMSID:       smsID,
			Text:        text,
			Response:    resp,
		}, []string{"id"})

		return msgType, resp, err
	}

	notFoundMessage := "Neplatný kód stanoviště, zkontrolujte prosím správnost: " + code
	if !found {
		return msg("error", notFoundMessage)
	}

	// 4. Process action
	cipherStatus, err := t.GetCipherStatus()
	if err != nil {
		return "", "", err
	}
	status, statusFound := cipherStatus[cipher.ID]
	discoverable := cipher.Discoverable(cipherStatus)

	if !statusFound {
		if !discoverable {
			//return msg("error", notFoundMessage)
			return msg("error", "Kód je správný, ale u této šifry byste neměli být. Nepřeskočili jste nějakou?")
		} else if action == actionHint {
			return msg("error", "Nemůžete žádat nápovědu na nenavštíveném stanovišti! Nejdříve prosím odešlete příchodovou zprávu.")
		} else if action == actionSkip {
			return msg("error", "Nemůžete žádat přeskočení na nenavštíveném stanovišti! Nejdříve prosím odešlete příchodovou zprávu.")
		} else if action == actionAdvance {
			if cipher.ArrivalCode != "" {
				return msg("error", "Nemůžete zadat postupový kód nenavštíveného stanoviště! Nejdříve prosím odešlete příchodovou zprávu.")
			}
			if err := t.LogCipherArrival(cipher); err != nil {
				return "", "", err
			}
			if err := t.LogCipherSolved(&cipher); err != nil {
				return "", "", err
			}
			return msg("success", "Správně! <b>%s</b>", cipher.AdvanceText)
		} else {
			if err := t.LogCipherArrival(cipher); err != nil {
				return "", "", err
			}

			finalOrder := 0
			ciphersToStandings := cipher.SharedStandings
			ciphersToStandings = append(ciphersToStandings, cipher.ID)
			for _, cipherID := range ciphersToStandings {
				order := 0
				t.tx.Get(&order, "SELECT COUNT(team) FROM cipher_status WHERE cipher=$1", cipherID)
				finalOrder += order
			}

			msgParts := []string{"Kód přijat"}
			if t.gameConfig.OrderPickupMessage {
				msgParts = append(msgParts, fmt.Sprintf(", jste %d na tomto stanovišti", finalOrder))
			}
			if t.gameConfig.LastPickupMessage {
				if finalOrder == len(t.gameConfig.teams) {
					msgParts = append(msgParts, " <b>(jste poslední, seberte ho prosím)</b>")
				}
			}
			msgParts = append(msgParts, ".")
			if cipher.ArrivalText != "" {
				msgParts = append(msgParts, " <b>"+cipher.ArrivalText+"</b>")
			}
			return msg("success", strings.Join(msgParts, ""))
		}
	} else {
		if action == actionHint {
			msgType, msgText, _, err := t.RequestHint(&cipher, status)
			if err != nil {
				return "", "", err
			}
			return msg(msgType, msgText)
		} else if action == actionSkip {
			msgType, msgText, _, err := t.RequestSkip(&cipher, status)
			if err != nil {
				return "", "", err
			}
			return msg(msgType, msgText)
		} else if action == actionAdvance {
			t.LogCipherSolved(&cipher)
			return msg("success", "Správně! <b>%s</b>", cipher.AdvanceText)
		} else {
			return msg("info", "Kód tohoto stanoviště jsme již od vás přijali, nemusíte ho zadávat vícekrát.")
		}
	}
}

func (t *Team) processSifricky(codes []string) (string, string, error) {
	// FIXME: imlementace
	return "danger", "Šifřičky zatím nebyly implementovány", nil
}
