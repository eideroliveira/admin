package login

import (
	"errors"
	"net/http"

	"github.com/golang-jwt/jwt/v4"
)

var (
	errNoTokenString = errors.New("no token string")
	errTokenExpired  = errors.New("token expired")
	errInvalidToken  = errors.New("invalid token")
)

type UserClaims struct {
	Provider      string
	Email         string
	Name          string
	UserID        string
	AvatarURL     string
	Location      string
	IDToken       string
	PassUpdatedAt string
	jwt.RegisteredClaims
}

func signClaims(claims jwt.Claims, secret string) (signed string, err error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err = token.SignedString([]byte(secret))
	if err != nil {
		return "", err
	}
	return signed, nil
}

func parseClaimsFromCookie(r *http.Request, cookieName string, claims jwt.Claims, secret string) (rc jwt.Claims, err error) {
	tc, err := r.Cookie(cookieName)
	if err != nil || tc.Value == "" {
		return nil, errNoTokenString
	}
	token, err := jwt.ParseWithClaims(tc.Value, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, errTokenExpired
		}
		return nil, errInvalidToken
	}
	if !token.Valid {
		return nil, errInvalidToken
	}
	return token.Claims, nil
}

func parseUserClaims(r *http.Request, cookieName string, secret string) (rc *UserClaims, err error) {
	c, err := parseClaimsFromCookie(r, cookieName, &UserClaims{}, secret)
	if err != nil {
		return nil, err
	}
	rc, ok := c.(*UserClaims)
	if !ok {
		return nil, errInvalidToken
	}
	return rc, nil
}
