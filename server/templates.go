package server

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/coreos/go-log/log"
)

// Execute template given by its name and with given data with all the error handling.
func (s *Server) executeTemplate(w http.ResponseWriter, templateName string, data interface{}) {
	log.Debugf("Executing template '%s'", templateName)
	template, err := s.getTemplates()
	if err != nil || template == nil {
		msg := fmt.Sprintf("Error getting templates: %v", err)
		log.Errorf(msg, err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	err = template.ExecuteTemplate(w, templateName, data)
	if err != nil {
		msg := fmt.Sprintf("Error executing template '%s': %v", templateName, err)
		log.Errorf(msg)
		http.Error(w, msg, http.StatusInternalServerError)
	}
}

func findFiles(dir string, ext string) ([]string, error) {

	files := []string{}
	err := filepath.Walk(dir, func(path string, f os.FileInfo, err error) error {
		if filepath.Ext(path) == ext {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// Scan directory with templates and if there is some changed file reload all templates,
// then return these loaded templates.
func (s *Server) getTemplates() (*template.Template, error) {
	templateFiles, err := findFiles(s.config.TemplateDir, ".tmpl")
	if err != nil {
		return nil, err
	}
	changed := false
	for _, file := range templateFiles {
		if fileChanged(file) {
			log.Debugf("Found (new/changed) template file '%s'", file)
			changed = true
		}
	}

	if changed {
		log.Debug("Parsing all template files because of new/changed template files")
		s.templates, err = template.New("").Funcs(templateFuncs).Funcs(template.FuncMap{
			"basedir": func() string { return s.config.BaseDir },
		}).ParseFiles(templateFiles...)
		if err != nil {
			return nil, err
		}
	}
	return s.templates, nil
}

var fileModMap = make(map[string]time.Time)

func fileChanged(path string) bool {
	stats, err := os.Stat(path)
	if err != nil {
		return true // missing file is also change
	}
	modTime, exists := fileModMap[path]
	if !exists || modTime != stats.ModTime() {
		fileModMap[path] = stats.ModTime()
		return true
	}
	return false
}
