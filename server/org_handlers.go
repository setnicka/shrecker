package server

import (
	"fmt"
	"net/http"
	"path"
	"sort"
	"strconv"

	"github.com/go-chi/chi"
	"github.com/setnicka/shrecker/game"
)

// middleware for authentication
func (s *Server) orgAuth(redirectPath ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := s.sessionStore.Get(r, s.config.SessionCookieName)
			if err != nil {
				http.Error(w, fmt.Sprintf("Cannot get session '%s': %v", s.config.SessionCookieName, err), http.StatusInternalServerError)
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
		session, _ := s.sessionStore.Get(r, s.config.SessionCookieName)
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
	teams, _, gameConfig, err := s.game.GetAll(r.Context(), true, true, true)
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

	switch gameConfig.Mode {
	case game.GameNormal:
		http.Error(w, "NOT YET IMPLEMENTED", http.StatusNotImplemented)
	case game.GameOnlineCodes:
		http.Error(w, "NOT YET IMPLEMENTED", http.StatusNotImplemented)
	case game.GameOnlineMap:
		s.executeTemplate(
			w, "org_index_map", orgIndexData{
				GeneralData: s.getGeneralData("Orgovský přehled", w, r),
				GameConfig:  gameConfig,
				GameHash:    gameConfig.GetGameHash(),
				Teams:       teamInfos,
				Ciphers:     gameConfig.GetCiphers(),
			},
		)
	}
}

func (s *Server) orgPlayback(w http.ResponseWriter, r *http.Request) {
	teams, _, gameConfig, err := s.game.GetAll(r.Context(), true, true, true)
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
