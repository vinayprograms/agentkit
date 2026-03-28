package credentials

import (
	"time"
)

// OAuthToken holds OAuth2 details for a provider.
type OAuthToken struct {
	AccessToken  string    `toml:"access_token,required"`
	RefreshToken string    `toml:"refresh_token,required"`
	ExpiresAt    time.Time `toml:"expires_at,required"`
	ClientID     string    `toml:"client_id,omitempty"`
	Scopes       []string  `toml:"scopes,omitempty"`
	RefreshURL   string    `toml:"refresh_url,required"`
}

// IsExpired returns true if the token has expired or will expire within buffer.
func (t *OAuthToken) IsExpired() bool {
	if t.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().Add(5 * time.Minute).After(t.ExpiresAt)
}

// IsValid returns true if token exists and is not expired.
func (t *OAuthToken) IsValid() bool {
	return t != nil && t.AccessToken != "" && !t.IsExpired()
}
