package credentials

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidOauthToken(t *testing.T) {
	validToken := OAuthToken{
		AccessToken:  "valid_token",
		RefreshToken: "refresh_token",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		ClientID:     "client_id",
	}

	assert.True(t, validToken.IsValid(), "expected token to be valid")
}

func TestExpiredOauthToken(t *testing.T) {
	expiredToken := OAuthToken{
		AccessToken:  "expired_token",
		RefreshToken: "refresh_token",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
		ClientID:     "client_id",
	}

	assert.False(t, expiredToken.IsValid(), "expected token to be expired")
}

func TestOauthTokenWithoutExpiryInformation(t *testing.T) {
	tokenWithoutExpiry := OAuthToken{
		AccessToken:  "token_without_expiry",
		RefreshToken: "refresh_token",
		ClientID:     "client_id",
	}

	assert.True(t, tokenWithoutExpiry.IsValid(), "expected token without expiry information to be valid")
}
