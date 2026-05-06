package service_test

import (
	"testing"

	"github.com/sidneydekoning/analytics/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testAccessSecret  = []byte("test-access-secret-32-bytes-xxxxx")
	testRefreshSecret = []byte("test-refresh-secret-32-bytes-xxxx")
)

func newTestAuthService() service.AuthService {
	return service.NewAuth(testAccessSecret, testRefreshSecret)
}

func TestHashPassword_AndCompare(t *testing.T) {
	svc := newTestAuthService()

	hash, err := svc.HashPassword("correct-horse-battery-staple")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, "correct-horse-battery-staple", hash)

	assert.True(t, svc.CheckPassword("correct-horse-battery-staple", hash))
	assert.False(t, svc.CheckPassword("wrong-password", hash))
}

func TestIssueAndParseAccessToken(t *testing.T) {
	svc := newTestAuthService()

	claims, err := svc.IssueAccessToken("user-uuid-123", "user")
	require.NoError(t, err)
	assert.NotEmpty(t, claims.TokenString)
	assert.Equal(t, "user-uuid-123", claims.UserID)
	assert.Equal(t, "user", claims.Role)
	assert.NotEmpty(t, claims.JTI)

	parsed, err := svc.ParseAccessToken(claims.TokenString)
	require.NoError(t, err)
	assert.Equal(t, "user-uuid-123", parsed.UserID)
	assert.Equal(t, "user", parsed.Role)
}

func TestIssueAndParseRefreshToken(t *testing.T) {
	svc := newTestAuthService()

	claims, err := svc.IssueRefreshToken("user-uuid-456")
	require.NoError(t, err)
	assert.NotEmpty(t, claims.TokenString)
	assert.Equal(t, "user-uuid-456", claims.UserID)

	parsed, err := svc.ParseRefreshToken(claims.TokenString)
	require.NoError(t, err)
	assert.Equal(t, "user-uuid-456", parsed.UserID)
}

func TestParseAccessToken_WrongSecret(t *testing.T) {
	svc := newTestAuthService()
	wrongSvc := service.NewAuth([]byte("wrong-secret-32-bytes-xxxxxxxxxxx"), testRefreshSecret)

	claims, err := svc.IssueAccessToken("user-uuid-789", "admin")
	require.NoError(t, err)

	_, err = wrongSvc.ParseAccessToken(claims.TokenString)
	assert.Error(t, err)
}

func TestGenerateSiteToken_Format(t *testing.T) {
	svc := newTestAuthService()

	t1, err := svc.GenerateSiteToken()
	require.NoError(t, err)
	assert.True(t, len(t1) > 8, "token should be longer than 8 chars")

	t2, err := svc.GenerateSiteToken()
	require.NoError(t, err)
	assert.NotEqual(t, t1, t2, "tokens should be unique")
}

func TestGenerateSecureToken_IsRandom(t *testing.T) {
	svc := newTestAuthService()

	t1, err := svc.GenerateSecureToken()
	require.NoError(t, err)
	assert.Equal(t, 64, len(t1), "should be 32 bytes as hex = 64 chars")

	t2, err := svc.GenerateSecureToken()
	require.NoError(t, err)
	assert.NotEqual(t, t1, t2)
}
