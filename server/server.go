package server

import (
	"html/template"
	"net/http"

	"github.com/coreos/go-log/log"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-ini/ini"
	"github.com/gorilla/csrf"
	"github.com/gorilla/sessions"
	"github.com/pkg/errors"
	"github.com/setnicka/shrecker/game"
)

// Server represents HTTP server for the Shrecker
type Server struct {
	sessionStore sessions.Store
	templates    *template.Template
	game         *game.Game
	serverCfg    *ini.Section
	config       config
}

type config struct {
	BaseDir           string `ini:"base_dir"`
	StaticDir         string `ini:"static_dir"`
	TemplateDir       string `ini:"template_dir"`
	ListenAddress     string `ini:"listen_address"`
	CSRFKey           string `ini:"csrf_key"`
	OrgLogin          string `ini:"org_login"`
	OrgPassword       string `ini:"org_password"`
	SessionCookieName string `ini:"session_cookie_name"`
	SessionSecret     string `ini:"session_secret"`
	SessionMaxAge     int    `ini:"session_max_age"`
	FlashCookieName   string `ini:"flash_cookie_name"`
}

type contextKey int

const (
	orgStateKey contextKey = iota
	teamStateKey
)

// New creates new server
func New(config *ini.File, game *game.Game) (*Server, error) {
	serverCfg := config.Section("server")
	if serverCfg == nil {
		return nil, errors.Errorf("Config file does not contain game section")
	}

	s := Server{game: game}
	// Load config
	if err := serverCfg.MapTo(&s.config); err != nil {
		return nil, err
	}

	// Setup cookie store
	cookieStore := sessions.NewCookieStore([]byte(s.config.SessionSecret))
	cookieStore.MaxAge(s.config.SessionMaxAge)
	cookieStore.Options.Secure = true
	//cookieStore.Options.Domain = ".fuf.me"
	s.sessionStore = cookieStore

	return &s, nil
}

// Start HTTP server
func (s *Server) Start() error {

	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.CleanPath)
	r.Use(middleware.StripSlashes)
	r.Use(middleware.Compress(5))
	r.Use(csrf.Protect(
		[]byte(s.config.CSRFKey),
		csrf.Path("/"),
	))

	// Static resources
	fs := NoListFileSystem{http.Dir(s.config.StaticDir)}
	r.Mount("/static/", http.StripPrefix("/static/", http.FileServer(fs)))

	// Routes without authorization
	r.Get("/org/login", s.orgLogin)
	r.Post("/org/login", s.orgLoginPost)
	r.Get("/login", s.teamLogin)
	r.Post("/login", s.teamLoginPost)
	r.Post("/logout", s.logout)

	// Org api - fail on unauthorized
	r.Route("/org/api", func(r chi.Router) {
		r.Use(s.orgAuth())
		// TODO
	})

	// Org pages - redirect on unauthorized
	r.Route("/org", func(r chi.Router) {
		r.Use(s.orgAuth(s.basedir("/org/login")))
		r.Get("/", s.orgIndex)
		r.Get("/team/{id}/", s.orgTeam)
		r.Get("/cipher/{id}/download", s.orgCipherDownload)
	})

	// Team api - fail on unauthorized
	r.Route("/api", func(r chi.Router) {
		r.Use(s.teamAuth())
		r.Get("/calc-move", s.teamCalcMove)
	})

	// Team pages - redirect on unauthorized
	r.Route("/", func(r chi.Router) {
		r.Use(s.teamAuth(s.basedir("/login")))
		r.Get("/", s.teamIndex)
		r.Post("/", s.teamIndex)
		r.Get("/cipher/{id}/download", s.teamCipherDownload)
	})

	// 3. Listen on given port
	log.Info("Server started")
	return http.ListenAndServe(s.config.ListenAddress, r)
}
