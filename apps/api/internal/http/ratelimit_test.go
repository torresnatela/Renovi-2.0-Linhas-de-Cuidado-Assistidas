package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func hit(t *testing.T, h http.Handler, ip string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = ip + ":40000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

// Sem trava por IP, o login vira alvo de força bruta: a senha é o único fator.
func TestRateLimit_BloqueiaAposORajada(t *testing.T) {
	h := rateLimitByIP(3, 0.0001)(okHandler()) // 3 de burst, recarga desprezível

	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusOK, hit(t, h, "203.0.113.7"), "requisição %d do burst", i+1)
	}
	assert.Equal(t, http.StatusTooManyRequests, hit(t, h, "203.0.113.7"), "a 4ª tinha que ser barrada")
}

// A trava é POR IP: um atacante não pode derrubar o login de todo mundo.
func TestRateLimit_IsolaPorIP(t *testing.T) {
	h := rateLimitByIP(2, 0.0001)(okHandler())

	hit(t, h, "203.0.113.7")
	hit(t, h, "203.0.113.7")
	require.Equal(t, http.StatusTooManyRequests, hit(t, h, "203.0.113.7"))

	assert.Equal(t, http.StatusOK, hit(t, h, "198.51.100.9"), "outro IP não pode herdar a punição")
}

func TestRateLimit_RespondeProblemJSON(t *testing.T) {
	h := rateLimitByIP(1, 0.0001)(okHandler())
	hit(t, h, "203.0.113.7")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "203.0.113.7:40000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")
}
