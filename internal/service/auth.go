package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	accessTokenDuration  = 15 * time.Minute
	refreshTokenDuration = 7 * 24 * time.Hour
	bcryptCost           = 12
	siteTokenPrefix      = "tk_"
)

// TokenClaims holds values parsed from a JWT.
type TokenClaims struct {
	UserID      string
	Role        string
	JTI         string
	TokenString string // only populated on issue, empty on parse
}

// AuthService handles JWT issuance/validation, password hashing, and token generation.
type AuthService interface {
	HashPassword(password string) (string, error)
	CheckPassword(password, hash string) bool
	IssueAccessToken(userID, role string) (*TokenClaims, error)
	IssueRefreshToken(userID string) (*TokenClaims, error)
	ParseAccessToken(tokenString string) (*TokenClaims, error)
	ParseRefreshToken(tokenString string) (*TokenClaims, error)
	GenerateSiteToken() (string, error)
	GenerateSecureToken() (string, error)
}

type authService struct {
	accessSecret  []byte
	refreshSecret []byte
}

// NewAuth constructs an AuthService. Both secrets should be at least 32 bytes.
func NewAuth(accessSecret, refreshSecret []byte) AuthService {
	return &authService{
		accessSecret:  accessSecret,
		refreshSecret: refreshSecret,
	}
}

// HashPassword hashes password with bcrypt at cost 12.
func (s *authService) HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("authService.HashPassword: %w", err)
	}
	return string(b), nil
}

// CheckPassword uses bcrypt's constant-time comparison. Safe against timing attacks.
func (s *authService) CheckPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// IssueAccessToken creates a signed 15-minute JWT access token.
func (s *authService) IssueAccessToken(userID, role string) (*TokenClaims, error) {
	jti, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("authService.IssueAccessToken: %w", err)
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  userID,
		"role": role,
		"jti":  jti,
		"iat":  now.Unix(),
		"exp":  now.Add(accessTokenDuration).Unix(),
	})
	signed, err := token.SignedString(s.accessSecret)
	if err != nil {
		return nil, fmt.Errorf("authService.IssueAccessToken: sign: %w", err)
	}
	return &TokenClaims{UserID: userID, Role: role, JTI: jti, TokenString: signed}, nil
}

// IssueRefreshToken creates a signed 7-day JWT refresh token.
func (s *authService) IssueRefreshToken(userID string) (*TokenClaims, error) {
	jti, err := randomHex(16)
	if err != nil {
		return nil, fmt.Errorf("authService.IssueRefreshToken: %w", err)
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"jti": jti,
		"iat": now.Unix(),
		"exp": now.Add(refreshTokenDuration).Unix(),
	})
	signed, err := token.SignedString(s.refreshSecret)
	if err != nil {
		return nil, fmt.Errorf("authService.IssueRefreshToken: sign: %w", err)
	}
	return &TokenClaims{UserID: userID, JTI: jti, TokenString: signed}, nil
}

// ParseAccessToken validates and parses an access token string.
func (s *authService) ParseAccessToken(tokenString string) (*TokenClaims, error) {
	return s.parseToken(tokenString, s.accessSecret)
}

// ParseRefreshToken validates and parses a refresh token string.
func (s *authService) ParseRefreshToken(tokenString string) (*TokenClaims, error) {
	return s.parseToken(tokenString, s.refreshSecret)
}

func (s *authService) parseToken(tokenString string, secret []byte) (*TokenClaims, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return nil, fmt.Errorf("authService.parseToken: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("authService.parseToken: invalid claims")
	}
	return &TokenClaims{
		UserID: fmt.Sprint(claims["sub"]),
		Role:   fmt.Sprint(claims["role"]),
		JTI:    fmt.Sprint(claims["jti"]),
	}, nil
}

// GenerateSiteToken returns a unique token prefixed with "tk_" for tracking scripts.
func (s *authService) GenerateSiteToken() (string, error) {
	b, err := randomHex(8)
	if err != nil {
		return "", fmt.Errorf("authService.GenerateSiteToken: %w", err)
	}
	return siteTokenPrefix + b, nil
}

// GenerateSecureToken returns a 64-char hex string (32 random bytes) for password
// reset and email verification tokens.
func (s *authService) GenerateSecureToken() (string, error) {
	b, err := randomHex(32)
	if err != nil {
		return "", fmt.Errorf("authService.GenerateSecureToken: %w", err)
	}
	return b, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
