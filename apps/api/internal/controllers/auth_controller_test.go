package controllers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/models"
)

// ---------------------------------------------------------------------------
// Dublês
// ---------------------------------------------------------------------------

type fakeAccounts struct {
	account  models.Account
	err      error
	gotInput models.RegisterInput
	gotCPF   string
}

func (f *fakeAccounts) Register(_ context.Context, in models.RegisterInput) (models.Account, error) {
	f.gotInput = in
	return f.account, f.err
}

func (f *fakeAccounts) Authenticate(_ context.Context, cpf, _ string) (models.Account, error) {
	f.gotCPF = cpf
	return f.account, f.err
}

type fakeSessions struct {
	token      string
	err        error
	validated  models.Account
	validErr   error
	revoked    string
	revokeCall int
}

func (f *fakeSessions) Create(_ context.Context, _ uuid.UUID) (string, time.Time, error) {
	return f.token, time.Now().Add(time.Hour), f.err
}

func (f *fakeSessions) Validate(_ context.Context, _ string) (models.Account, error) {
	return f.validated, f.validErr
}

func (f *fakeSessions) Revoke(_ context.Context, token string) error {
	f.revoked = token
	f.revokeCall++
	return nil
}

func newAuthController(a *fakeAccounts, s *fakeSessions) controllers.AuthController {
	return controllers.AuthController{
		Accounts: a, Sessions: s,
		CookieSecure: true, SessionTTL: time.Hour,
	}
}

var contaValida = models.Account{
	ID:       uuid.MustParse("019f6c3e-7659-7e86-8c93-b0009ced58d7"),
	FullName: "Roberval Juvencio Lazaroti", Email: "roberval@example.com",
}

const corpoValido = `{
  "full_name": "Roberval Juvencio Lazaroti",
  "cpf": "948.190.898-46",
  "birth_date": "1976-01-23",
  "email": "roberval@example.com",
  "phone": "11912345678",
  "password": "cavalo-bateria-grampo-correto",
  "address": {"zip_code":"06472000","street":"Avenida Copacabana","number":"238",
              "neighborhood":"Dezoito do Forte","city":"Barueri","state":"SP"}
}`

func postJSON(t *testing.T, h http.HandlerFunc, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

// ---------------------------------------------------------------------------
// Register
// ---------------------------------------------------------------------------

func TestRegister_201ComCookieDeSessao(t *testing.T) {
	a := &fakeAccounts{account: contaValida}
	s := &fakeSessions{token: "token-opaco-secreto"}
	c := newAuthController(a, s)

	rec := postJSON(t, c.Register, "/api/v1/auth/register", corpoValido)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	ck := cookies[0]
	assert.Equal(t, "renovi_session", ck.Name)
	assert.Equal(t, "token-opaco-secreto", ck.Value)
	assert.True(t, ck.HttpOnly, "sem HttpOnly o token vaza por XSS")
	assert.True(t, ck.Secure)
	assert.Equal(t, http.SameSiteLaxMode, ck.SameSite)
	assert.Equal(t, "/", ck.Path)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, contaValida.ID.String(), body["id"])
	assert.Equal(t, contaValida.Email, body["email"])
}

// O controller precisa repassar o IP para a auditoria do vínculo — é o que vai
// permitir revisar anexações quando o fator de posse existir.
func TestRegister_PassaIPParaAuditoria(t *testing.T) {
	a := &fakeAccounts{account: contaValida}
	c := newAuthController(a, &fakeSessions{token: "t"})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(corpoValido))
	req.RemoteAddr = "203.0.113.7:54321"
	c.Register(httptest.NewRecorder(), req)

	assert.Equal(t, "203.0.113.7", a.gotInput.RequestIP)
}

func TestRegister_MapeiaErrosDoModel(t *testing.T) {
	tests := []struct {
		nome       string
		err        error
		wantStatus int
	}{
		{"dados inválidos", models.ErrInvalidRegistration, http.StatusBadRequest},
		{"já cadastrado", models.ErrAlreadyRegistered, http.StatusConflict},
		{"e-mail em uso na DAV", models.ErrEmailTakenAtDAV, http.StatusConflict},
		{"DAV fora", models.ErrDAVUnavailable, http.StatusGatewayTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			c := newAuthController(&fakeAccounts{err: tt.err}, &fakeSessions{})
			rec := postJSON(t, c.Register, "/api/v1/auth/register", corpoValido)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/problem+json")
			assert.Empty(t, rec.Result().Cookies(), "erro não pode abrir sessão")
		})
	}
}

// O 409 de "já existe" não pode dizer SE foi o CPF ou o e-mail: isso permitiria
// varrer CPFs para descobrir quem tem conta.
func TestRegister_409NaoRevelaQualCampoColidiu(t *testing.T) {
	c := newAuthController(&fakeAccounts{err: models.ErrAlreadyRegistered}, &fakeSessions{})
	rec := postJSON(t, c.Register, "/api/v1/auth/register", corpoValido)

	corpo := strings.ToLower(rec.Body.String())
	assert.NotContains(t, corpo, "cpf")
	assert.NotContains(t, corpo, "948190898")
}

// Já o e-mail em uso NA DAV precisa ser acionável: é o caso do casal que
// compartilha e-mail, e "dados inválidos" deixaria o usuário sem saída.
func TestRegister_EmailNaDAVTemMensagemAcionavel(t *testing.T) {
	c := newAuthController(&fakeAccounts{err: models.ErrEmailTakenAtDAV}, &fakeSessions{})
	rec := postJSON(t, c.Register, "/api/v1/auth/register", corpoValido)

	assert.Contains(t, strings.ToLower(rec.Body.String()), "e-mail")
}

func TestRegister_RejeitaCorpoInvalido(t *testing.T) {
	tests := []struct{ nome, corpo string }{
		{"JSON quebrado", `{"full_name":`},
		{"corpo vazio", ``},
		{"data de nascimento não é data", strings.Replace(corpoValido, `"1976-01-23"`, `"ontem"`, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			c := newAuthController(&fakeAccounts{account: contaValida}, &fakeSessions{token: "t"})
			rec := postJSON(t, c.Register, "/api/v1/auth/register", tt.corpo)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

// A resposta do cadastro não pode devolver nada que veio da DAV nem a senha.
func TestRegister_RespostaNaoVazaSegredo(t *testing.T) {
	c := newAuthController(&fakeAccounts{account: contaValida}, &fakeSessions{token: "t"})
	rec := postJSON(t, c.Register, "/api/v1/auth/register", corpoValido)

	corpo := rec.Body.String()
	assert.NotContains(t, corpo, "cavalo-bateria-grampo-correto", "a senha voltou na resposta")
	assert.NotContains(t, corpo, "94819089846", "o CPF voltou na resposta")
	assert.NotContains(t, strings.ToLower(corpo), "dav")
}

// ---------------------------------------------------------------------------
// Login / Logout
// ---------------------------------------------------------------------------

func TestLogin_200ComCookie(t *testing.T) {
	a := &fakeAccounts{account: contaValida}
	c := newAuthController(a, &fakeSessions{token: "tok"})

	rec := postJSON(t, c.Login, "/api/v1/auth/login",
		`{"cpf":"948.190.898-46","password":"cavalo-bateria-grampo-correto"}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, rec.Result().Cookies(), 1)
	assert.Equal(t, "tok", rec.Result().Cookies()[0].Value)
	assert.Equal(t, "948.190.898-46", a.gotCPF, "o CPF vai cru para o model, que normaliza")
}

func TestLogin_401Generico(t *testing.T) {
	c := newAuthController(&fakeAccounts{err: models.ErrInvalidCredentials}, &fakeSessions{})
	rec := postJSON(t, c.Login, "/api/v1/auth/login", `{"cpf":"948.190.898-46","password":"errada-mas-longa"}`)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Empty(t, rec.Result().Cookies())
	// Nem "senha errada" nem "cpf não existe": o texto não pode diferenciar.
	corpo := strings.ToLower(rec.Body.String())
	assert.NotContains(t, corpo, "senha")
	assert.NotContains(t, corpo, "não existe")
}

func TestLogout_RevogaELimpaOCookie(t *testing.T) {
	s := &fakeSessions{validated: contaValida}
	c := newAuthController(&fakeAccounts{}, s)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "renovi_session", Value: "tok-vivo"})
	rec := httptest.NewRecorder()
	c.Logout(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "tok-vivo", s.revoked, "a sessão precisa morrer no servidor, não só no browser")

	require.Len(t, rec.Result().Cookies(), 1)
	ck := rec.Result().Cookies()[0]
	assert.Empty(t, ck.Value)
	assert.True(t, ck.MaxAge < 0, "o cookie precisa ser expirado no browser")
}

// ---------------------------------------------------------------------------
// Me + middleware de sessão
// ---------------------------------------------------------------------------

func TestMe_DevolveAContaDaSessao(t *testing.T) {
	s := &fakeSessions{validated: contaValida}
	c := newAuthController(&fakeAccounts{}, s)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "renovi_session", Value: "tok"})
	rec := httptest.NewRecorder()

	controllers.RequireSession(s)(http.HandlerFunc(c.Me)).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, contaValida.Email, body["email"])
}

func TestRequireSession_Recusa(t *testing.T) {
	tests := []struct {
		nome    string
		cookie  *http.Cookie
		session *fakeSessions
	}{
		{"sem cookie", nil, &fakeSessions{}},
		{"cookie vazio", &http.Cookie{Name: "renovi_session", Value: ""}, &fakeSessions{}},
		{"sessão inválida", &http.Cookie{Name: "renovi_session", Value: "x"},
			&fakeSessions{validErr: models.ErrNoSession}},
	}

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rec := httptest.NewRecorder()

			chamou := false
			controllers.RequireSession(tt.session)(http.HandlerFunc(
				func(http.ResponseWriter, *http.Request) { chamou = true },
			)).ServeHTTP(rec, req)

			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.False(t, chamou, "o handler protegido rodou sem sessão válida")
		})
	}
}

func TestRegister_CookieInseguroSoQuandoConfigurado(t *testing.T) {
	c := controllers.AuthController{
		Accounts: &fakeAccounts{account: contaValida}, Sessions: &fakeSessions{token: "t"},
		CookieSecure: false, SessionTTL: time.Hour,
	}
	rec := postJSON(t, c.Register, "/api/v1/auth/register", corpoValido)

	require.Len(t, rec.Result().Cookies(), 1)
	assert.False(t, rec.Result().Cookies()[0].Secure)
	assert.True(t, rec.Result().Cookies()[0].HttpOnly, "HttpOnly nunca é opcional")
}

func TestRegister_MaxAgeCasaComOTTL(t *testing.T) {
	c := controllers.AuthController{
		Accounts: &fakeAccounts{account: contaValida}, Sessions: &fakeSessions{token: "t"},
		CookieSecure: true, SessionTTL: 90 * time.Minute,
	}
	rec := postJSON(t, c.Register, "/api/v1/auth/register", corpoValido)

	require.Len(t, rec.Result().Cookies(), 1)
	assert.Equal(t, int((90 * time.Minute).Seconds()), rec.Result().Cookies()[0].MaxAge,
		fmt.Sprintf("cookie e sessão precisam expirar juntos"))
}
