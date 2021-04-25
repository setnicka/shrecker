package server

import (
	"fmt"
	"html/template"
	"time"

	"github.com/setnicka/shrecker/game"
)

func timestampGeneric(t time.Time, now time.Time) (string, string) {
	word := "před"
	d := now.Sub(t)
	if d < 0 {
		word = "za"
		d *= -1
	}
	formatString := "15:04:05"
	if now.Day() != t.Day() {
		formatString = "2.1. 15:04"
	}
	if now.Year() != t.Year() {
		formatString = "2.1.2006 15:04"
	}

	var outD string
	if d.Hours() > 48 {
		outD = fmt.Sprintf("%s %d dny", word, int(d.Hours()/24))
	} else if d.Hours() >= 1 {
		h := int(d.Hours())
		outD = fmt.Sprintf("%s %dh %dm", word, h, int(d.Minutes())-h*60)
	} else {
		m := int(d.Minutes())
		outD = fmt.Sprintf("%s %dm %ds", word, m, int(d.Seconds())-m*60)
	}

	return t.Format(formatString), outD
}

func timestampFormat(t time.Time) string {
	ts, ds := timestampGeneric(t, time.Now())
	return fmt.Sprintf("%s (%s)", ts, ds)
}

var (
	templateFuncs = template.FuncMap{
		"timestamp": timestampFormat,
		"timestamp_hint": func(t time.Time) template.HTML {
			ts, ds := timestampGeneric(t, time.Now())
			return template.HTML(fmt.Sprintf("<span class='hint' title='%s'>%s</span>", ds, ts))
		},
		"latlon": func(p game.Point) string {
			latL := 'N'
			if p.Lat < 0 {
				latL = 'S'
				p.Lat *= -1
			}
			lonL := 'E'
			if p.Lon < 0 {
				lonL = 'W'
				p.Lon *= -1
			}
			return fmt.Sprintf("%f%c, %f%c", p.Lat, latL, p.Lon, lonL)
		},
	}
)
