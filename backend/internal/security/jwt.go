package security

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type TokenManager struct {
	secret []byte
	issuer string
}

type Claims struct {
	UserID       uint64 `json:"uid"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	Type         string `json:"typ"`
	TokenVersion int    `json:"ver"`
	jwt.RegisteredClaims
}

func NewTokenManager(secret string) *TokenManager {
	return &TokenManager{secret: []byte(secret), issuer: "smartdoc-vision"}
}

func (m *TokenManager) Access(userID uint64, username, role string, tokenVersion int) (string, error) {
	return m.sign(userID, username, role, "access", tokenVersion, 24*time.Hour)
}

func (m *TokenManager) Refresh(userID uint64, username, role string, tokenVersion int) (string, error) {
	return m.sign(userID, username, role, "refresh", tokenVersion, 7*24*time.Hour)
}

func (m *TokenManager) sign(userID uint64, username, role, typ string, tokenVersion int, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:       userID,
		Username:     username,
		Role:         role,
		Type:         typ,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        uuid.NewString(),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *TokenManager) Parse(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	}, jwt.WithIssuer(m.issuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
