package requestctx

import "context"

const localeKey ctxKey = "locale"

// GetLocale returns the locale from context.
func GetLocale(ctx context.Context) string {
	val, _ := ctx.Value(localeKey).(string)
	return val
}

// SetLocale sets the locale in the context.
func SetLocale(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, localeKey, locale)
}
