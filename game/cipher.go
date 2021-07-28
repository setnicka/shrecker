package game

// GetCiphers returns ciphers in order as they are in config
func (c *Config) GetCiphers() []CipherConfig { return c.ciphers }

// GetCiphersMap returns map of ciphers by IDs
func (c *Config) GetCiphersMap() map[string]*CipherConfig { return c.ciphersMap }

// GetCipher returns cipher config by ID
func (c *Config) GetCipher(ID string) (*CipherConfig, bool) {
	cipher, found := c.ciphersMap[ID]
	return cipher, found
}

// Discoverable tests if Cipher could be discovered from given previously discovered ciphers
func (c *CipherConfig) Discoverable(discoveredCiphers map[string]CipherStatus) bool {
	if _, found := discoveredCiphers[c.ID]; found {
		return true
	}
	// dependencies [ [a, b, c], [d, e], [f] ] means (a AND b AND c) OR (d AND e) OR (f)
	for _, variant := range c.DependsOn {
		variantPossible := true
		for _, dependency := range variant {
			if _, found := discoveredCiphers[dependency]; !found {
				variantPossible = false
				break
			}
		}
		if variantPossible {
			return true
		}
	}
	return false
}

// DiscoverableFromPoint tests if Cipher could be discovered by standing on
// given Point with given previously discovered ciphers
func (c *CipherConfig) DiscoverableFromPoint(pos Point, discoveredCiphers map[string]CipherStatus) bool {
	if _, found := discoveredCiphers[c.ID]; found {
		return true
	}
	return c.Discoverable(discoveredCiphers) && c.Position.InRadius(pos)
}

// internal function for calculating rest of fields and setting link to CipherConfig
func (c *CipherStatus) init(gameConfig *Config) {
	c.Points = 0

	var found bool
	if c.Config, found = gameConfig.ciphersMap[c.Cipher]; !found {
		return
	}

	if c.Config.NotCipher {
		return
	} else if c.Skip != nil {
		c.Points = gameConfig.PointsSkipped
	} else if c.Solved != nil {
		if c.Hint != nil {
			c.Points = gameConfig.PointsSolvedHint
		} else {
			c.Points = gameConfig.PointsSolved
		}
		c.Points += c.ExtraPoints
	}
}
