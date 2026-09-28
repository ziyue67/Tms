package httpapi

import (
	"context"
	"net/http"

	"github.com/ziyue67/tms/go-backend/internal/auth"
)

type claimsContextKey struct{}

func withClaims(r *http.Request, claims auth.Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), claimsContextKey{}, claims))
}

func claimsFrom(r *http.Request) (auth.Claims, bool) {
	claims, ok := r.Context().Value(claimsContextKey{}).(auth.Claims)
	return claims, ok
}
