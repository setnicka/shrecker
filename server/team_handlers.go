package server

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/coreos/go-log/log"
	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"github.com/setnicka/shrecker/game"
	"github.com/setnicka/sqlxpp"
)

type teamState struct {
	team       *game.Team
	tx         *sqlxpp.Tx
	gameConfig *game.Config
}

// middleware for authentication
func (s *Server) teamAuth(redirectPath ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := s.sessionStore.Get(r, s.config.SessionCookieName)
			if err != nil {
				http.Error(w, fmt.Sprintf("Cannot get session '%s': %v", s.config.SessionCookieName, err), http.StatusInternalServerError)
				return
			}
			authenticated, _ := session.Values["authenticated"].(bool)
			teamID, _ := session.Values["team"].(string)
			if !authenticated || teamID == "" {
				redirectOrForbidden(w, r, redirectPath...)
				return
			}
			team, tx, gameConfig, err := s.game.GetTeamTx(r.Context(), teamID)
			if err == game.ErrTeamNotFound {
				redirectOrForbidden(w, r, redirectPath...)
				return
			} else if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			// Everything ok, save to context
			next.ServeHTTP(w, r.WithContext(context.WithValue(
				r.Context(), teamStateKey, teamState{
					team:       team,
					tx:         tx,
					gameConfig: gameConfig,
				})))
		})
	}
}

func getTeamState(r *http.Request) (*game.Team, *sqlxpp.Tx, *game.Config) {
	teamState := r.Context().Value(teamStateKey).(teamState)
	return teamState.team, teamState.tx, teamState.gameConfig
}

type teamGeneralData struct {
	GeneralData
	TeamConfig *game.TeamConfig
	GameConfig *game.Config
}

func (s *Server) getTeamGeneralData(title string, w http.ResponseWriter, r *http.Request) teamGeneralData {
	team, _, gameConfig := getTeamState(r)

	team.GetCipherStatus()

	return teamGeneralData{
		GeneralData: s.getGeneralData(title, w, r),
		TeamConfig:  team.GetConfig(),
		GameConfig:  gameConfig,
	}
}

////////////////////////////////////////////////////////////////////////////////

func (s *Server) teamLogin(w http.ResponseWriter, r *http.Request) {
	s.executeTemplate(w, "team_login", s.getGeneralData("Přihlášení do hry", w, r))
}
func (s *Server) teamLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.setFlashMessage(w, r, "danger", "Cannot parse login form")
	}
	login := r.PostFormValue("login")
	password := r.PostFormValue("password")
	team, _, err := s.game.LoginTeam(login, password)
	if err == game.ErrLogin {
		s.setFlashMessage(w, r, "danger", "Nesprávný login")
		http.Redirect(w, r, "login", http.StatusSeeOther)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	} else {
		log.Infof("Logged in team '%s'", team.GetConfig().Name)
		session, _ := s.sessionStore.Get(r, s.config.SessionCookieName)
		session.Values["authenticated"] = true
		session.Values["team"] = team.GetConfig().ID
		session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.setFlashMessage(w, r, "danger", "Nesprávný login")
	http.Redirect(w, r, "login", http.StatusSeeOther)
}

type cipherInfo struct {
	Config game.CipherConfig
	Status game.CipherStatus
}
type teamIndexData struct {
	teamGeneralData
	TeamStatus *game.TeamStatus
	Ciphers    []cipherInfo
}

func (s *Server) teamIndex(w http.ResponseWriter, r *http.Request) {
	team, tx, gameConfig := getTeamState(r)

	status, err := team.GetStatus()
	if err != nil {
		log.Errorf("Cannot get team status: %+v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cipherStatus, err := team.GetCipherStatus()
	if err != nil {
		log.Errorf("Cannot get team ciphers status: %+v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ciphers := []cipherInfo{}
	for _, cipher := range gameConfig.GetCiphers() {
		if cs, found := cipherStatus[cipher.ID]; found {
			ciphers = append(ciphers, cipherInfo{Config: cipher, Status: cs})
		}
	}

	if r.Method == http.MethodPost {
		// Handle hint and skip
		hint := r.PostFormValue("hint") != ""
		skip := r.PostFormValue("skip") != ""
		cipherID := r.PostFormValue("cipher")
		if hint || skip {
			// Test if we could hint
			cipher, found := gameConfig.GetCipher(cipherID)
			status, statusFound := cipherStatus[cipherID]
			if !found {
				s.setFlashMessage(w, r, "danger", "Šifra s tímto ID neexistuje")
			} else if !statusFound {
				s.setFlashMessage(w, r, "danger", "Tuto šifru jste zatím nenavštívili, nelze na ni žádat o nápovědu")
			} else if hint {
				if cipher.HintText == "" {
					s.setFlashMessage(w, r, "danger", "Tato šifra nemá nápovědu")
				} else if status.Hint != nil {
					s.setFlashMessage(w, r, "info", "O nápovědu na tuto šifru jste již požádali")
				} else if d := time.Now().Sub(status.Arrival); d < gameConfig.HintLimit {
					s.setFlashMessage(w, r, "danger", "Zatím uběhlo jen %v od příchodu na šifru, nápověda je dostupná až po %v od příchodu", d, gameConfig.HintLimit)
				} else {
					if err := team.LogCipherHint(cipher); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					tx.Commit()
					s.setFlashMessage(w, r, "success", "Nápověda: %s", cipher.HintText)
				}
			} else if skip {
				if cipher.SkipText == "" {
					s.setFlashMessage(w, r, "danger", "Tato šifra nelze přeskočit")
				} else if status.Skip != nil {
					s.setFlashMessage(w, r, "info", "Tuto šifru jste již přeskočili")
				} else if d := time.Now().Sub(status.Arrival); d < gameConfig.SkipLimit {
					s.setFlashMessage(w, r, "danger", "Zatím uběhlo jen %v od příchodu na šifru, přeskočení je dostupné až po %v od příchodu", d, gameConfig.SkipLimit)
				} else {
					if err := team.LogCipherSkip(cipher); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
					tx.Commit()

					s.setFlashMessage(w, r, "success", "Šifra přeskočena. Umístění další šifry: %s", cipher.SkipText)
				}
			}
		}
		// Handle move to position
		latStr := r.PostFormValue("move-lat")
		lonStr := r.PostFormValue("move-lon")
		if latStr != "" && lonStr != "" {
			lat, latErr := strconv.ParseFloat(latStr, 32)
			lon, lonErr := strconv.ParseFloat(lonStr, 32)
			if latErr != nil || lonErr != nil {
				http.Error(w, latErr.Error()+lonErr.Error(), http.StatusBadRequest)
				return
			}
			if status.CooldownTo != nil && status.CooldownTo.After(team.Now()) {
				s.setFlashMessage(w, r, "danger", "Nelze se přesunout, ještě do %v máte cooldown", status.CooldownTo)
			} else {
				if err := team.MapMoveToPosition(game.Point{Lat: lat, Lon: lon}); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				} else if discovered, err := team.DiscoverCiphers(); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				} else {
					status, _ := team.GetStatus()
					tx.Commit()
					if len(discovered) == 0 {
						s.setFlashMessage(w, r, "warning", "Přesun dokončen, ale žádná šifra neobjevena. Před dalším přesunem je nutné počkat do %s", timestampFormat(*status.CooldownTo))
					}
					for _, cipher := range discovered {
						s.setFlashMessage(w, r, "success", "Objevena šifra %s", cipher.Name)
					}
				}
			}
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	switch gameConfig.Mode {
	case game.GameNormal:
		http.Error(w, "NOT YET IMPLEMENTED", http.StatusNotImplemented)
	case game.GameOnlineCodes:
		http.Error(w, "NOT YET IMPLEMENTED", http.StatusNotImplemented)
	case game.GameOnlineMap:
		s.executeTemplate(
			w, "team_index_map", teamIndexData{
				teamGeneralData: s.getTeamGeneralData("Mapa šifrovačky", w, r),
				TeamStatus:      status,
				Ciphers:         ciphers,
			},
		)
	}
}

func (s *Server) teamCalcMove(w http.ResponseWriter, r *http.Request) {
	lat, latErr := strconv.ParseFloat(r.URL.Query().Get("lat"), 32)
	lon, lonErr := strconv.ParseFloat(r.URL.Query().Get("lon"), 32)
	if latErr != nil || lonErr != nil {
		http.Error(w, latErr.Error()+lonErr.Error(), http.StatusBadRequest)
		return
	}

	team, _, _ := getTeamState(r)
	status, err := team.GetStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if status.CooldownTo != nil && status.CooldownTo.After(team.Now()) {
		render.JSON(w, r, map[string]interface{}{
			"error":       "cooldown",
			"cooldown_to": timestampFormat(*status.CooldownTo),
		})
	} else {
		distance, cooldown, _ := team.GetDistanceTo(game.Point{Lat: lat, Lon: lon}) // err is checked by GetStatus above
		render.JSON(w, r, map[string]interface{}{
			"distance": distance,
			"cooldown": cooldown.String(),
		})
	}
}

func (s *Server) teamCipherDownload(w http.ResponseWriter, r *http.Request) {
	cipherID := chi.URLParam(r, "id")
	team, _, gameConfig := getTeamState(r)
	cipher, found := gameConfig.GetCipher(cipherID)
	if !gameConfig.CouldTeamDownloadCiphers() || !found || cipher.File == "" {
		http.NotFound(w, r)
		return
	}

	cipherStatus, err := team.GetCipherStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, found := cipherStatus[cipherID]; !found {
		http.NotFound(w, r) // exists but this team does not know about it
		return
	}

	// everything ok, serve file
	w.Header().Set("Content-Disposition", fmt.Sprintf("filename=%s.pdf", cipher.ID))
	http.ServeFile(w, r, path.Join(gameConfig.CiphersFolder, cipher.File))
}
