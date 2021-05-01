package server

import (
	"net/http"
	"os"
)

////////////////////////////////////////////////////////////////////////////////

// NoListFileSystem is used for accessing static resources but without listing directory index
type NoListFileSystem struct {
	base http.FileSystem
}

type noListFile struct {
	http.File
}

func (f noListFile) Readdir(count int) ([]os.FileInfo, error) {
	return nil, nil
}

// Open opens dir/file on given path
func (fs NoListFileSystem) Open(name string) (http.File, error) {
	f, err := fs.base.Open(name)
	if err != nil {
		return nil, err
	}
	s, err := f.Stat()
	if s.IsDir() {
		return nil, os.ErrNotExist
	}
	return f, err
}

////////////////////////////////////////////////////////////////////////////////

func redirectOrForbidden(w http.ResponseWriter, r *http.Request, redirectPath ...string) {
	if len(redirectPath) > 0 {
		http.Redirect(w, r, redirectPath[0], http.StatusTemporaryRedirect)
	} else {
		http.Error(w, "403 Forbidden", http.StatusForbidden)
	}
}
