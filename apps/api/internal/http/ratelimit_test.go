package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/models"
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

// O reaper roda a cada minuto; entre duas passadas, nada segurava o mapa. Um
// atacante rotacionando IP de origem crescia a memória sem teto — exatamente o
// que o comentário do reaper alegava impedir.
func TestRateLimit_MapaTemTeto(t *testing.T) {
	const teto = 50
	l := &ipLimiter{limiters: make(map[string]*visitor), burst: 1, rate: 1, maxEntries: teto}

	for i := 0; i < teto*4; i++ {
		l.allow(fmt.Sprintf("198.51.100.%d:%d", i%256, i))
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	assert.LessOrEqual(t, len(l.limiters), teto, "o mapa cresceu além do teto")
	assert.NotEmpty(t, l.limiters, "o mapa não pode zerar: perderia a trava de quem está atacando")
}

// --- rate limit por CONTA (agendamento) ---

// fakeSess valida qualquer token como uma conta fixa, imitando o RequireSession.
type fakeSess struct{ acc models.Account }

func (f fakeSess) Create(context.Context, uuid.UUID) (string, time.Time, error) {
	return "t", time.Time{}, nil
}
func (f fakeSess) Validate(context.Context, string) (models.Account, error) { return f.acc, nil }
func (f fakeSess) Revoke(context.Context, string) error                     { return nil }

// hitAcct bate na rota autenticada com o cookie de sessão, variando só o IP.
func hitAcct(t *testing.T, h http.Handler, ip string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/appointments", nil)
	req.RemoteAddr = ip + ":40000"
	req.AddCookie(&http.Cookie{Name: controllers.SessionCookieName, Value: "tok"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

// A trava do agendamento é por CONTA, não por IP: trocar de IP (o que o spoofing
// de header permitiria) NÃO abre um balde novo — a mesma conta continua barrada.
func TestRateLimit_PorConta_NaoContornaComTrocaDeIP(t *testing.T) {
	acc := models.Account{ID: uuid.New()}
	guarded := controllers.RequireSession(fakeSess{acc})(rateLimitByAccount(3, 0.0001)(okHandler()))

	// Esgota o burst da conta a partir de um IP.
	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusOK, hitAcct(t, guarded, "203.0.113.7"), "req %d do burst", i+1)
	}
	// Mesma conta, IP DIFERENTE: continua barrada (a chave é a conta).
	assert.Equal(t, http.StatusTooManyRequests, hitAcct(t, guarded, "198.51.100.9"),
		"trocar de IP não pode furar a trava por conta")
}

// Contas diferentes têm baldes separados: uma não derruba a outra.
func TestRateLimit_PorConta_IsolaContas(t *testing.T) {
	a := models.Account{ID: uuid.New()}
	ha := controllers.RequireSession(fakeSess{a})(rateLimitByAccount(1, 0.0001)(okHandler()))
	require.Equal(t, http.StatusOK, hitAcct(t, ha, "203.0.113.7"))
	assert.Equal(t, http.StatusTooManyRequests, hitAcct(t, ha, "203.0.113.7"))

	b := models.Account{ID: uuid.New()}
	hb := controllers.RequireSession(fakeSess{b})(rateLimitByAccount(1, 0.0001)(okHandler()))
	assert.Equal(t, http.StatusOK, hitAcct(t, hb, "203.0.113.7"), "conta B tem balde próprio")
}
