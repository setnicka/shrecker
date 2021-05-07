package game

import (
	"math"
	"time"
)

const earthRadius = 6_371_000 // in metres

// degreesToRadians converts from degrees to radians.
func degreesToRadians(d float64) float64 {
	return d * math.Pi / 180
}

// Distance calculates the shortest path between two coordinates on the surface
// of the Earth, returns distance in metres
func (p *Point) Distance(q Point) (distance float64) {
	lat1 := degreesToRadians(p.Lat)
	lon1 := degreesToRadians(p.Lon)
	lat2 := degreesToRadians(q.Lat)
	lon2 := degreesToRadians(q.Lon)

	diffLat := lat2 - lat1
	diffLon := lon2 - lon1

	a := math.Pow(math.Sin(diffLat/2), 2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Pow(math.Sin(diffLon/2), 2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return c * earthRadius
}

// InRadius tests if given pos is in radius of this Point
func (p *PointRadius) InRadius(pos Point) bool {
	return p.Distance(pos) <= float64(p.Radius)
}

// CouldTeamDownloadCiphers tests if game mode allows to download ciphers
func (c *Config) CouldTeamDownloadCiphers() bool {
	return c.Mode == GameOnlineCodes || c.Mode == GameOnlineMap
}

// NotStarted returns true if the game has set start time and it is in the future
func (c *Config) NotStarted(now time.Time) bool {
	return !c.Start.IsZero() && c.Start.After(now)
}

// Ended returns true if the game has set end time and it is already ended
func (c *Config) Ended(now time.Time) bool {
	return !c.End.IsZero() && c.End.Before(now)
}

// GetGameHash returns combined hash of all teams (changed on every change)
func (c *Config) GetGameHash() int {
	hash := 0
	for _, h := range c.teamHash {
		hash += h
	}
	return hash
}
