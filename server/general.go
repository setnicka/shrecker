package server

import (
	"encoding/gob"
	"fmt"
	"html/template"
	"net/http"
	"path"
	"time"

	"github.com/coreos/go-log/log"
	"github.com/gorilla/csrf"
)

// GeneralData for rendering page
type GeneralData struct {
	Title    string
	Now      time.Time
	Messages []flashMessage
	CSRF     template.HTML
	Basedir  string
}

func (s *Server) getGeneralData(title string, w http.ResponseWriter, r *http.Request) GeneralData {
	data := GeneralData{
		Title:    title,
		Now:      time.Now(),
		Messages: s.getFlashMessages(w, r),
		CSRF:     csrf.TemplateField(r),
		Basedir:  s.config.BaseDir,
	}
	return data
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	session, _ := s.sessionStore.Get(r, s.config.SessionCookieName)
	session.Options.MaxAge = -1
	session.Save(r, w)
	http.Redirect(w, r, s.basedir("/"), http.StatusSeeOther)
}

func (s *Server) basedir(url string) string {
	return path.Join(s.config.BaseDir, url)
}

////////////////////////////////////////////////////////////////////////////////

// flashMessage holds type and content of flash message displayed to the user
type flashMessage struct {
	Type    string
	Message template.HTML
}

func (s *Server) setFlashMessage(w http.ResponseWriter, r *http.Request, mtype string, messageFormat string, a ...interface{}) {
	// Register the struct so encoding/gob knows about it
	gob.Register(flashMessage{})

	message := fmt.Sprintf(messageFormat, a...)
	session, err := s.sessionStore.Get(r, s.config.FlashCookieName)
	if err != nil {
		return
	}
	session.AddFlash(flashMessage{Type: mtype, Message: template.HTML(message)})
	err = session.Save(r, w)
	if err != nil {
		log.Errorf("Cannot save flash message: %v", err)
	}
}

func (s *Server) getFlashMessages(w http.ResponseWriter, r *http.Request) []flashMessage {
	// 1. Get session
	session, err := s.sessionStore.Get(r, s.config.FlashCookieName)
	if err != nil {
		return nil
	}

	// 2. Get flash messages
	parsedFlashes := []flashMessage{}
	if flashes := session.Flashes(); len(flashes) > 0 {
		for _, flash := range flashes {
			parsedFlashes = append(parsedFlashes, flash.(flashMessage))
		}
	}

	// 3. Delete flash messages by saving session
	err = session.Save(r, w)
	if err != nil {
		log.Errorf("Problem during loading flash messages: %v", err)
	}

	return parsedFlashes
}
