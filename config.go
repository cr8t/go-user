package user

import (
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
	"net/http"
	"regexp"
	"time"
)

var defaultCookieName = "auth-jwt"

// AuthConfig is the configuration for auth check handler
type AuthConfig struct {
	Cookie         string
	Issuer         string
	Bypass         *regexp.Regexp
	ExpireTime     time.Duration
	GetUser        func(username string) (Auther, error)
	GetLoginClaims func(username, password string) (jwt.Claims, error)
	UpdateClaims   func(token string) (jwt.Claims, error)
	ErrorHandler   func(error, http.ResponseWriter, *http.Request)
}

func (c *AuthConfig) cookieName() string {
	if c.Cookie == "" {
		return defaultCookieName
	}
	return c.Cookie
}

func (c *AuthConfig) getIssuer() (string, error) {
	if c.Issuer == "" {
		return "", errors.New("Issuer must be defined on the provided AuthConfig")
	}
	return c.Issuer, nil
}

func (c *AuthConfig) getTokenExpireTime() int64 {
	if c.ExpireTime == 0 {
		return time.Now().Add(time.Hour * 6).Unix()
	}
	return time.Now().Add(c.ExpireTime).Unix()
}

func (c *AuthConfig) getUser(username string) (Auther, error) {
	if c.GetUser == nil {
		return nil, errors.New("GetUser must be defined on the provided AuthConfig")
	}
	return c.GetUser(username)
}

func (c *AuthConfig) updateClaims(t string) (jwt.Claims, error) {
	if c.UpdateClaims != nil {
		return c.UpdateClaims(t)
	}
	claims, err := ExtractDefaultClaims(t)
	if err != nil {
		return nil, err
	}
	claims.ExpiresAt = c.getTokenExpireTime()
	return claims, nil
}

func (c *AuthConfig) handleError(err error, w http.ResponseWriter, r *http.Request) {
	if c.ErrorHandler != nil {
		c.ErrorHandler(err, w, r)
		return
	}
	w.Header().Set("content-type", "application/json")
	message := err.Error()
	statusCode := http.StatusInternalServerError
	code := http.StatusInternalServerError

	if err == http.ErrNoCookie {
		statusCode = http.StatusUnauthorized
		code = http.StatusUnauthorized
		message = "no auth"
	} else if aErr, ok := err.(AuthErr); ok {
		code = aErr.Code()
		statusCode = code
		if http.StatusText(code) == "" {
			statusCode = http.StatusInternalServerError
		}
		message = aErr.Err().Error()
	}

	w.WriteHeader(statusCode)
	w.Write([]byte(fmt.Sprintf(`{"code": "%d", "message": "%s"}`, code, message)))
}

func (c *AuthConfig) updateAndSetCookie(token string, w http.ResponseWriter, r *http.Request) (jwt.Claims, string, error) {
	claims, err := c.updateClaims(token)
	if err != nil {
		return nil, "", err
	}

	token, err = MakeTokenString(claims)
	if err != nil {
		return nil, "", err
	}

	cookie := MakeCookie(token, r.Header.Get("origin"), r.Host, c.cookieName())
	http.SetCookie(w, cookie)

	return claims, token, nil
}

func (c *AuthConfig) getLoginClaims(username, password string) (jwt.Claims, error) {
	if c.GetLoginClaims != nil {
		return c.GetLoginClaims(username, password)
	}
	u, err := c.getUser(username)
	if err != nil {
		return nil, err
	}

	if !passwordValid(u, password) {
		return nil, errLoginIncorrectUserPass
	}

	issuer, err := c.getIssuer()
	if err != nil {
		return nil, err
	}

	return &Claims{
		Username: u.GetUsername(),
		UserID:   u.GetId(),
		StandardClaims: jwt.StandardClaims{
			// TODO: lower expire time
			ExpiresAt: c.getTokenExpireTime(),
			Issuer:    issuer,
		},
	}, nil
}