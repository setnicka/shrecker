package server

import (
	"context"
	"fmt"
	"image/png"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/go-chi/chi"
	"github.com/setnicka/shrecker/game"
)

// middleware for authentication
func (s *Server) orgAuth(redirectPath ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := s.sessionStore.Get(r, sessionCookieName)
			if err != nil {
				http.Error(w, fmt.Sprintf("Cannot get session '%s': %v", sessionCookieName, err), http.StatusInternalServerError)
				return
			}
			authenticated, _ := session.Values["authenticated"].(bool)
			isOrg, _ := session.Values["org"].(bool)

			if authenticated && isOrg {
				next.ServeHTTP(w, r) // Pass request down to the next handler
			} else {
				redirectOrForbidden(w, r, redirectPath...)
			}
		})
	}
}

////////////////////////////////////////////////////////////////////////////////

func (s *Server) orgLogin(w http.ResponseWriter, r *http.Request) {
	s.executeTemplate(w, "org_login", s.getGeneralData("Orgovský login", w, r))
}
func (s *Server) orgLoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.setFlashMessage(w, r, "danger", "Cannot parse login form")
	}
	login := r.PostFormValue("login")
	password := r.PostFormValue("password")
	if login == s.config.OrgLogin && password == s.config.OrgPassword {
		session, _ := s.sessionStore.Get(r, sessionCookieName)
		session.Values["authenticated"] = true
		session.Values["org"] = true
		session.Save(r, w)
		http.Redirect(w, r, s.basedir("/org/"), http.StatusSeeOther)
		return
	}
	s.setFlashMessage(w, r, "danger", "Nesprávný login")
	http.Redirect(w, r, s.basedir("/org/login"), http.StatusSeeOther)
}

////////////////////////////////////////////////////////////////////////////////

func (s *Server) orgGameHash(w http.ResponseWriter, r *http.Request) {
	config := s.game.GetConfig()
	w.Write([]byte(strconv.Itoa(config.GetGameHash())))
}

type teamInfo struct {
	Config    *game.TeamConfig
	Status    *game.TeamStatus
	Points    int
	Stats     game.TeamStats
	Locations []game.TeamLocationEntry
	Ciphers   map[string]game.CipherStatus
	Messages  []game.Message
}

func (s *Server) getTeamInfos(ctx context.Context) ([]teamInfo, *game.Config, error) {
	teams, _, gameConfig, err := s.game.GetAll(ctx, true, true, true, false)
	if err != nil {
		return nil, nil, err
	}
	teamInfos := []teamInfo{}
	for _, team := range teams {
		// everything is preloaded by GetAll, no err possible, no need to check
		status, _ := team.GetStatus()
		ciphers, _ := team.GetCipherStatus()
		locations, _ := team.GetLocations()
		points, _ := team.SumPoints()
		stats, _ := team.GetStats()
		teamInfos = append(teamInfos, teamInfo{
			Config:    team.GetConfig(),
			Status:    status,
			Points:    points,
			Stats:     stats,
			Locations: locations,
			Ciphers:   ciphers,
		})
	}
	sort.Slice(teamInfos, func(i, j int) bool {
		return teamInfos[i].Config.ID < teamInfos[j].Config.ID
	})

	return teamInfos, gameConfig, nil
}

type orgTeamsData struct {
	GeneralData
	GameConfig *game.Config
	GameHash   int
	Teams      []teamInfo
	Ciphers    game.CiphersSplitted
}

func (s *Server) orgIndex(w http.ResponseWriter, r *http.Request) {
	teamInfos, gameConfig, err := s.getTeamInfos(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	templateName := "org_index"
	if gameConfig.HasMap() {
		templateName = "org_index_map"
	}
	s.executeTemplate(
		w, templateName, orgTeamsData{
			GeneralData: s.getGeneralData("Orgovský přehled", w, r),
			GameConfig:  gameConfig,
			GameHash:    gameConfig.GetGameHash(),
			Teams:       teamInfos,
			Ciphers:     gameConfig.GetCiphersByType(),
		},
	)
}

func (s *Server) orgTeams(w http.ResponseWriter, r *http.Request) {
	teamInfos, gameConfig, err := s.getTeamInfos(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	s.executeTemplate(
		w, "org_teams", orgTeamsData{
			GeneralData: s.getGeneralData("Týmy", w, r),
			GameConfig:  gameConfig,
			GameHash:    gameConfig.GetGameHash(),
			Teams:       teamInfos,
			Ciphers:     gameConfig.GetCiphersByType(),
		},
	)
}

func (s *Server) orgPlayback(w http.ResponseWriter, r *http.Request) {
	teams, _, gameConfig, err := s.game.GetAll(r.Context(), true, true, true, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	teamInfos := []teamInfo{}
	for _, team := range teams {
		// everything is preloaded by GetAll, no err possible, no need to check
		status, _ := team.GetStatus()
		locations, _ := team.GetLocations()
		teamInfos = append(teamInfos, teamInfo{
			Config:    team.GetConfig(),
			Status:    status,
			Locations: locations,
		})
	}
	sort.Slice(teamInfos, func(i, j int) bool {
		return teamInfos[i].Config.ID < teamInfos[j].Config.ID
	})

	s.executeTemplate(
		w, "org_playback", orgTeamsData{
			GeneralData: s.getGeneralData("Playback", w, r),
			GameConfig:  gameConfig,
			Teams:       teamInfos,
			Ciphers:     gameConfig.GetCiphersByType(),
		},
	)
}

type orgTeamData struct {
	GeneralData
	GameConfig    *game.Config
	Team          teamInfo
	TeamLoginLink string
	Ciphers       game.CiphersSplitted
	CiphersMap    map[string]*game.CipherConfig
}

func (s *Server) orgTeam(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	team, _, gameConfig, err := s.game.GetTeamTx(r.Context(), teamID)
	if err == game.ErrTeamNotFound {
		http.NotFound(w, r)
		return
	}

	teamConfig := team.GetConfig()
	teamStatus, err := team.GetStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	teamCiphers, err := team.GetCipherStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	teamPoints, err := team.SumPoints()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	teamStats, err := team.GetStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	teamLocations, err := team.GetLocations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	teamMessages, err := team.GetMessages()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	sort.Slice(teamMessages, func(i, j int) bool {
		return teamMessages[i].Time.After(teamMessages[j].Time)
	})

	s.executeTemplate(
		w, "org_team", orgTeamData{
			GeneralData: s.getGeneralData("Tým", w, r),
			GameConfig:  gameConfig,
			Ciphers:     gameConfig.GetCiphersByType(),
			CiphersMap:  gameConfig.GetCiphersMap(),
			Team: teamInfo{
				Config:    teamConfig,
				Ciphers:   teamCiphers,
				Status:    teamStatus,
				Points:    teamPoints,
				Stats:     teamStats,
				Locations: teamLocations,
				Messages:  teamMessages,
			},
			TeamLoginLink: fmt.Sprintf(
				"%s%s/quick-login?l=%s&p=%s",
				s.config.BaseURL, s.config.BaseDir,
				url.QueryEscape(teamConfig.Login),
				url.QueryEscape(teamConfig.Password),
			),
		},
	)
}

type orgTeamCipherData struct {
	GeneralData
	GameConfig    *game.Config
	CiphersMap    map[string]*game.CipherConfig
	Team          *game.TeamConfig
	Cipher        game.CipherConfig
	Found         bool
	CipherStatus  game.CipherStatus
	CiphersStatus map[string]game.CipherStatus
	Messages      []game.Message
}

func (s *Server) orgTeamCipher(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "teamID")
	team, tx, gameConfig, err := s.game.GetTeamTx(r.Context(), teamID)
	if err == game.ErrTeamNotFound {
		http.NotFound(w, r)
		return
	}
	cipherID := chi.URLParam(r, "cipherID")
	cipherConfig, found := gameConfig.GetCiphersMap()[cipherID]
	if !found {
		http.NotFound(w, r)
		return
	}

	// Get cipher status
	teamCiphers, err := team.GetCipherStatus()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	cipherStatus, found := teamCiphers[cipherID]

	if r.Method == http.MethodPost {
		redirectPath := s.basedir("/org/team/%s/cipher/%s", teamID, cipherID)

		// New cipher status for not-found cipher
		if r.FormValue("submit") == "set-found" {
			if found {
				s.setFlashMessage(w, r, "danger", "Šifra již objevena, nejde nastavit znovu")
				http.Redirect(w, r, redirectPath, http.StatusSeeOther)
				return
			}
			if err := team.LogCipherArrival(*cipherConfig); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			} else if err := tx.Commit(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			} else {
				http.Redirect(w, r, redirectPath, http.StatusSeeOther)
			}
			return
		}

		// Edit of existing cipher status
		if !found {
			s.setFlashMessage(w, r, "danger", "Nelze provádět jiné akce na dosud neobjevené šifře")
			http.Redirect(w, r, redirectPath, http.StatusSeeOther)
		}
		var err error
		switch r.FormValue("submit") {
		case "set-solved":
			err = team.LogCipherSolved(cipherConfig)
		case "set-hint":
			err = team.LogCipherHint(cipherConfig)
		case "set-skip":
			err = team.LogCipherSkip(cipherConfig)
		case "set-extra-points":
			points, ierr := strconv.Atoi(r.FormValue("extra-points"))
			if ierr != nil {
				http.Error(w, ierr.Error(), http.StatusBadRequest)
				return
			}
			err = team.SetCipherExtraPoints(*cipherConfig, points)
		case "add-hint-score":
			add, ierr := strconv.Atoi(r.FormValue("add-hint-score"))
			if ierr != nil {
				http.Error(w, ierr.Error(), http.StatusBadRequest)
				return
			}
			err = team.AddHintScore(*cipherConfig, add)
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else if err := tx.Commit(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else {
			http.Redirect(w, r, redirectPath, http.StatusSeeOther)
		}
		return
	}

	// Filter messages only for this cipher
	teamMessages, err := team.GetMessages()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	cipherMessages := []game.Message{}
	for _, msg := range teamMessages {
		if msg.Cipher == cipherID {
			cipherMessages = append(cipherMessages, msg)
		}
	}
	sort.Slice(cipherMessages, func(i, j int) bool {
		return cipherMessages[i].Time.After(cipherMessages[j].Time)
	})

	s.executeTemplate(
		w, "org_team_cipher", orgTeamCipherData{
			GeneralData:   s.getGeneralData("Tým–šifra", w, r),
			GameConfig:    gameConfig,
			CiphersMap:    gameConfig.GetCiphersMap(),
			Team:          team.GetConfig(),
			Cipher:        *cipherConfig,
			Found:         found,
			CipherStatus:  cipherStatus,
			CiphersStatus: teamCiphers,
			Messages:      cipherMessages,
		},
	)
}

func (s *Server) orgTeamGPX(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	team, _, _, err := s.game.GetTeamTx(r.Context(), teamID)
	if err == game.ErrTeamNotFound {
		http.NotFound(w, r)
		return
	}
	locations, err := team.GetLocations()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	teamConfig := team.GetConfig()
	outputGPX(w, r, *teamConfig, locations)
}

type orgCiphersData struct {
	GeneralData
	GameConfig  *game.Config
	Ciphers     []game.CipherConfig
	CiphersMap  map[string]*game.CipherConfig
	ArrivalLink func(game.CipherConfig) string
}

func (s *Server) orgCiphers(w http.ResponseWriter, r *http.Request) {
	gameConfig := s.game.GetConfig()

	s.executeTemplate(
		w, "org_ciphers", orgCiphersData{
			GeneralData: s.getGeneralData("Šifry", w, r),
			GameConfig:  &gameConfig,
			Ciphers:     gameConfig.GetCiphers(),
			CiphersMap:  gameConfig.GetCiphersMap(),
			ArrivalLink: func(cipher game.CipherConfig) string {
				return fmt.Sprintf(
					"%s%s/quick-log/%s",
					s.config.BaseURL, s.config.BaseDir,
					url.PathEscape(cipher.ArrivalCode),
				)
			},
		},
	)
}

func (s *Server) orgCipherDownload(w http.ResponseWriter, r *http.Request) {
	cipherID := chi.URLParam(r, "id")
	gameConfig := s.game.GetConfig()
	cipher, found := gameConfig.GetCipher(cipherID)
	if !found || cipher.File == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("filename=%s.pdf", cipher.ID))
	http.ServeFile(w, r, path.Join(gameConfig.CiphersFolder, cipher.File))
}

type orgMessagesData struct {
	GeneralData
	GameConfig *game.Config
	Messages   []game.Message
	CiphersMap map[string]*game.CipherConfig
	TeamsMap   map[string]*game.TeamConfig
}

func (s *Server) orgMessages(w http.ResponseWriter, r *http.Request) {
	messages, _, gameConfig, err := s.game.GetAllMessages(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.executeTemplate(
		w, "org_messages", orgMessagesData{
			GeneralData: s.getGeneralData("Zprávy", w, r),
			GameConfig:  gameConfig,
			Messages:    messages,
			CiphersMap:  gameConfig.GetCiphersMap(),
			TeamsMap:    gameConfig.GetTeamsConfigMap(),
		},
	)
}

func (s *Server) orgQRCodeGen(w http.ResponseWriter, r *http.Request) {
	text := r.FormValue("text")
	size := 128
	sizeS := r.FormValue("size")
	if sizeS != "" {
		var err error
		if size, err = strconv.Atoi(sizeS); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}

	qrCode, _ := qr.Encode(text, qr.L, qr.Auto)
	qrCode, _ = barcode.Scale(qrCode, size, size)

	png.Encode(w, qrCode)
}
