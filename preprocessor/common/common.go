// common contains common types and functions shared across multiple preprocessors.
package common

import "github.com/starlinglab/integrity-v2/config"

// AllowedKey holds a public key (or domain name) that is known, and assets signed
// with this key are allowed into the system. The project and type for this key
// are presumed to be already known/filtered from context.
type AllowedKey struct {
	Key  string
	Name string
}

// BrowsertrixSigningDomains from config file, using default of "signing.app.browsertrix.com"
// if nothing is specified in config.
func BrowsertrixSigningDomains() []*AllowedKey {
	sds := config.GetConfig().Browsertrix.SigningDomains
	if len(sds) > 0 {
		bsd := make([]*AllowedKey, len(sds))
		for i, sd := range sds {
			bsd[i] = &AllowedKey{Key: sd, Name: sd}
		}
		return bsd
	}
	return []*AllowedKey{
		{Key: "signing.app.browsertrix.com", Name: "signing.app.browsertrix.com"},
	}
}
