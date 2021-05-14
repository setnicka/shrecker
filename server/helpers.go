package server

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/setnicka/shrecker/game"
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

func outputGPX(w http.ResponseWriter, r *http.Request, team game.TeamConfig, locations []game.TeamLocationEntry) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"trasa_%s.gpx\"", team.ID))
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, "<?xml version='1.0' encoding='UTF-8'?>\n")
	fmt.Fprintf(w, "<gpx version='1.0'>\n")
	fmt.Fprintf(w, "<name>Shrecker trasa týmu %s</name>\n", team.Name)
	fmt.Fprintf(w, "<trk>\n\t<name>Trasa týmu %s</name>\n\t<number>1</number>\n\t<trkseg>\n", team.Name)

	for _, location := range locations {
		fmt.Fprintf(w, "\t\t<trkpt lat='%f' lon='%f'><time>%s</time></trkpt>\n", location.Lat, location.Lon, location.Time.Format(time.RFC3339))
	}
	fmt.Fprintf(w, "\t</trkseg>\n</trk>\n</gpx>")
}
