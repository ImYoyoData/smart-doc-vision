package security

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenManager struct {
	secret []byte
	issuer string
}

type Claims struct {
	UserID   uint64 `json:"uid"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Type     string `json:"typ"`
	jwt.RegisteredClaims
}

func NewTokenManager(secret string) *TokenManager {
	return &TokenManager{secret: []byte(secret), issuer: "smartdoc-vision"}
}

func (m *TokenManager) Access(userID uint64, username, role string) (string, error) {
	return m.sign(userID, username, role, "access", 24*time.Hour)
}

func (m *TokenManager) Refresh(userID uint64, username, role string) (string, error) {
	return m.sign(userID, username, role, "refresh", 7*24*time.Hour)
}

func (m *TokenManager) sign(userID uint64, username, role, typ string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		Type:     typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        now.Format("20060102150405") + username,
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
