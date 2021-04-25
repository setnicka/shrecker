package game

// GetCiphers returns ciphers in order as they are in config
func (c *Config) GetCiphers() []CipherConfig { return c.ciphers }

// GetCiphersMap returns map of ciphers by IDs
func (c *Config) GetCiphersMap() map[string]CipherConfig { return c.ciphersMap }

// GetCipher returns cipher config by ID
func (c *Config) GetCipher(ID string) (CipherConfig, bool) {
	cipher, found := c.ciphersMap[ID]
	return cipher, found
}

// Discoverable tests if Cipher could be discovered by standing on given Point with
// given previously discovered ciphers
func (c *CipherConfig) Discoverable(pos Point, discoveredCiphers map[string]CipherStatus) bool {
	for _, dependency := range c.DependsOn {
		if _, found := discoveredCiphers[dependency]; !found {
			return false
		}
	}
	return c.Position.InRadius(pos)
}
