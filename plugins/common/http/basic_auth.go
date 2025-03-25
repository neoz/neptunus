package http

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"
)

type BasicUsernameContextKey struct{}

type BasicAuth struct {
	Username string `mapstructure:"basic_username"`
	Password string `mapstructure:"basic_password"`

	encoded string
}

func (b *BasicAuth) Init() bool {
	if len(b.Username) > 0 && len(b.Password) > 0 {
		b.encoded = base64.StdEncoding.EncodeToString([]byte(b.Username + ":" + b.Password))
	}

	return len(b.encoded) > 0
}

func (b *BasicAuth) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if len(auth) == 0 {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("credentials not provided"))
			return
		}

		if b.encoded != strings.TrimPrefix(auth, "Basic ") {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("invalid credentials"))
			return
		}

		r = r.WithContext(context.WithValue(r.Context(), BasicUsernameContextKey{}, b.Username))
		next.ServeHTTP(w, r)
	})
}
