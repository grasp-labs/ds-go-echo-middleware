package middleware

import (
	"strings"

	"github.com/labstack/echo/v4"
	"golang.org/x/text/language"
)

// Your supported locales (first is the default/fallback).
var matcher = language.NewMatcher([]language.Tag{
	language.English,         // "en"
	language.MustParse("nb"), // Norwegian Bokmål
})

func LocaleFromHeader(c echo.Context, def string) string {
	al := strings.TrimSpace(c.Request().Header.Get("Accept-Language"))
	if al == "" {
		return def
	}

	// Parse header respecting q-weights.
	tags, _, err := language.ParseAcceptLanguage(al)
	if err != nil || len(tags) == 0 {
		return def
	}

	// Match against supported tags.
	tag, _, _ := matcher.Match(tags...)
	if tag.IsRoot() { // == language.Und
		return def
	}

	// If you want "en" instead of "en-US", take the base.
	base, _ := tag.Base()
	return base.String()
}

// Locale is a helper function that safely parse
// inerface to string, returning def (default) on err
func Locale(c echo.Context, def string) string {
	v := c.Get("locale")
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

// LocaleMiddleware to
//
// # Example:
// loc := c.Get("locale").(string)
func LocaleMiddleware(def string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			loc := LocaleFromHeader(c, def)
			c.Set("locale", loc)
			return next(c)
		}
	}
}
