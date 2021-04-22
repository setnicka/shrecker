package game

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
