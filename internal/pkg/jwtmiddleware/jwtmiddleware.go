package jwtmiddleware

import (
	"github.com/labstack/echo/v4"
	"net/url"
	"os"
	"time"

	jwtmiddleware "github.com/auth0/go-jwt-middleware/v2"
	"github.com/auth0/go-jwt-middleware/v2/jwks"
	"github.com/auth0/go-jwt-middleware/v2/validator"
)

func New() (echo.MiddlewareFunc, error) {
	var issuerURL, err = url.Parse("https://" + os.Getenv("AUTH0_DOMAIN") + "/")

	if err != nil {
		return nil, err
	}

	provider := jwks.NewCachingProvider(issuerURL, 5*time.Minute)

	validator, err := validator.New(
		provider.KeyFunc,
		validator.RS256,
		issuerURL.String(),
		[]string{os.Getenv("AUTH0_AUDIENCE")},
	)

	if err != nil {
		return nil, err
	}

	jwtMiddleware := jwtmiddleware.New(validator.ValidateToken)
	return echo.WrapMiddleware(jwtMiddleware.CheckJWT), nil
}

func UserID(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		validatedClaims := c.Request().Context().Value(jwtmiddleware.ContextKey{}).(*validator.ValidatedClaims)
		c.Set("userID", validatedClaims.RegisteredClaims.Subject)
		return next(c)
	}
}
