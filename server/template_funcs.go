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

func timestampFormat(t time.Time) template.HTML {
	ts, ds := timestampGeneric(t, time.Now())
	return template.HTML(fmt.Sprintf("%s (<span data-countdown='%s'>%s</span>)", ts, t.Format("2006-01-02 15:04:05"), ds))
}

var (
	templateFuncs = template.FuncMap{
		"timestamp": timestampFormat,
		"timestamp_hint": func(t time.Time) template.HTML {
			ts, ds := timestampGeneric(t, time.Now())
			return template.HTML(fmt.Sprintf("<span class='hint' data-countdown-title='%s' title='%s'>%s</span>", t.Format("2006-01-02 15:04:05"), ds, ts))
		},
		"latlon": func(p game.Point) template.JS {
			return template.JS(fmt.Sprintf("[%f, %f]", p.Lat, p.Lon))
		},
		"latlon_human": func(p game.Point) string {
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
