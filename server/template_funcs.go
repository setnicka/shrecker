package server

import (
	"fmt"
	"html/template"
	"regexp"
	"strings"
	"time"

	"github.com/setnicka/shrecker/game"
)

var (
	isPhoneNumber = regexp.MustCompile(`^\+?[0-9]+$`).MatchString
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

	return t.Local().Format(formatString), outD
}

func timestampFormat(t time.Time) template.HTML {
	ts, ds := timestampGeneric(t, time.Now())
	return template.HTML(fmt.Sprintf("%s (<span data-countdown='%s'>%s</span>)", ts, t.Format(time.RFC3339), ds))
}

var (
	templateFuncs = template.FuncMap{
		"timestamp_js": func(t time.Time) string { return t.Format(time.RFC3339) },
		"timestamp":    timestampFormat,
		"timestamp_hint": func(t time.Time) template.HTML {
			ts, ds := timestampGeneric(t, time.Now())
			return template.HTML(fmt.Sprintf("<span class='hint' data-countdown-title='%s' title='%s'>%s</span>", t.Format(time.RFC3339), ds, ts))
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
		"contact_link": func(name string, contact string) template.HTML {
			if strings.Contains(contact, "@") {
				return template.HTML(fmt.Sprintf("<a href='mailto:%%22%s%%22 %%3C%s%%3E'>%s</a>", name, contact, name))
			} else if isPhoneNumber(contact) {
				return template.HTML(fmt.Sprintf("<a href='tel:%s'>%s</a>", contact, name))
			}
			return template.HTML(name)
		},
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
	}
)
