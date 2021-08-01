package server

import (
	"fmt"
	"image/png"
	"net/http"
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

type orgIndexData struct {
	GeneralData
	GameConfig *game.Config
	GameHash   int
	Teams      []teamInfo
	Ciphers    []game.CipherConfig
}

type teamInfo struct {
	Config    *game.TeamConfig
	Status    *game.TeamStatus
	Points    int
	Locations []game.TeamLocationEntry
	Ciphers   map[string]game.CipherStatus
}

func (s *Server) orgIndex(w http.ResponseWriter, r *http.Request) {
	teams, _, gameConfig, err := s.game.GetAll(r.Context(), true, true, true, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	teamInfos := []teamInfo{}
	for _, team := range teams {
		// everything is preloaded by GetAll, no err possible, no need to check
		status, _ := team.GetStatus()
		ciphers, _ := team.GetCipherStatus()
		locations, _ := team.GetLocations()
		points, _ := team.SumPoints()
		teamInfos = append(teamInfos, teamInfo{
			Config:    team.GetConfig(),
			Status:    status,
			Points:    points,
			Locations: locations,
			Ciphers:   ciphers,
		})
	}
	sort.Slice(teamInfos, func(i, j int) bool {
		return teamInfos[i].Config.ID < teamInfos[j].Config.ID
	})

	templateName := "org_index"
	if gameConfig.HasMap() {
		templateName = "org_index_map"
	}

	s.executeTemplate(
		w, templateName, orgIndexData{
			GeneralData: s.getGeneralData("Orgovský přehled", w, r),
			GameConfig:  gameConfig,
			GameHash:    gameConfig.GetGameHash(),
			Teams:       teamInfos,
			Ciphers:     gameConfig.GetCiphers(),
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
		w, "org_playback", orgIndexData{
			GeneralData: s.getGeneralData("Orgovský přehled – playback", w, r),
			GameConfig:  gameConfig,
			Teams:       teamInfos,
			Ciphers:     gameConfig.GetCiphers(),
		},
	)
}

func (s *Server) orgTeam(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
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
