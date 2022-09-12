package server

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/setnicka/shrecker/game"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
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

func unaccent(text string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ := transform.String(t, text)
	return result
}

const (
	htmlTagStart = 60 // Unicode `<`
	htmlTagEnd   = 62 // Unicode `>`
)

// Aggressively strips HTML tags from a string.
// It will only keep anything between `>` and `<`.
func stripHTMLTags(s string) string {
	// Setup a string builder and allocate enough memory for the new string.
	var builder strings.Builder
	builder.Grow(len(s) + utf8.UTFMax)

	in := false // True if we are inside an HTML tag.
	start := 0  // The index of the previous start tag character `<`
	end := 0    // The index of the previous end tag character `>`

	for i, c := range s {
		// Keep going if the character is not `<` or `>`
		if c != htmlTagStart && c != htmlTagEnd {
			continue
		}

		if c == htmlTagStart {
			// Only update the start if we are not in a tag.
			// This make sure we strip out `<<br>` not just `<br>`
			if !in {
				start = i
			}
			in = true

			// Write the valid string between the close and start of the two tags.
			builder.WriteString(s[end:start])
			continue
		}
		// else c == htmlTagEnd
		in = false
		end = i + 1
	}
	// After the last character and we are not in an HTML tag, save it.
	if end >= start {
		builder.WriteString(s[end:])
	}

	s = builder.String()
	return s
}
