package exchange

// APIPermissions describes the trading and withdrawal permissions detected on
// an exchange API key.
type APIPermissions struct {
	Spot     bool
	Futures  bool
	Withdraw bool
}

// ToJSON converts permissions to the jsonb format used by the database.
func (p *APIPermissions) ToJSON() map[string]bool {
	if p == nil {
		return map[string]bool{
			"spot":     false,
			"futures":  false,
			"withdraw": false,
		}
	}
	return map[string]bool{
		"spot":     p.Spot,
		"futures":  p.Futures,
		"withdraw": p.Withdraw,
	}
}
