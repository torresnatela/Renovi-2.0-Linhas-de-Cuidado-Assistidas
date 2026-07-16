package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/models"
)

// SessionCookieName casa com o securityScheme cookieAuth do openapi.yaml.
const SessionCookieName = "renovi_session"

// Accounts e Sessions são o que o controller precisa dos models. Interfaces aqui
// (no consumidor) mantêm o teste do handler sem banco.
type Accounts interface {
	Register(ctx context.Context, in models.RegisterInput) (models.Account, error)
	Authenticate(ctx context.Context, cpf, password string) (models.Account, error)
}

type Sessions interface {
	Create(ctx context.Context, accountID uuid.UUID) (string, time.Time, error)
	Validate(ctx context.Context, token string) (models.Account, error)
	Revoke(ctx context.Context, token string) error
}

// AuthController expõe /auth/* e /me.
type AuthController struct {
	Accounts Accounts
	Sessions Sessions
	// CookieSecure vem da config. False só em desenvolvimento sem TLS.
	CookieSecure bool
	SessionTTL   time.Duration
	// RegisterDeadline é quanto o handler de cadastro pode escrever. Precisa
	// cobrir o orçamento da DAV (config.DAVBudget); zero cai num default.
	// Deriva da config em vez de ser um número solto: quando os dois divergiram,
	// a última tentativa passou a ser cortada no meio, silenciosamente.
	RegisterDeadline time.Duration
}

type ctxKey int

const accountCtxKey ctxKey = iota

// Register cadastra o paciente, vincula à DAV e já abre a sessão.
func (c AuthController) Register(w http.ResponseWriter, r *http.Request) {
	var body api.RegisterRequest
	if !decodeJSON(w, r, &body) {
		return
	}

	// O cadastro é síncrono e a DAV é lenta: até ~29s por tentativa, vezes as
	// tentativas. O RENOVI_HTTP_WRITE_TIMEOUT do servidor é 15s, então sem
	// estender o prazo AQUI a resposta seria cortada no meio e o usuário veria
	// uma falha intermitente e inexplicável num cadastro que deu certo.
	deadline := c.RegisterDeadline
	if deadline <= 0 {
		deadline = 90 * time.Second
	}
	// NewResponseController nunca devolve nil. Se o ResponseWriter não suportar
	// deadline (ex.: httptest.Recorder), o erro é irrelevante: nesse caso não há
	// conexão para cortar.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(deadline))

	acc, err := c.Accounts.Register(r.Context(), models.RegisterInput{
		FullName:  body.FullName,
		CPF:       body.Cpf,
		BirthDate: body.BirthDate.Time,
		Email:     string(body.Email),
		Phone:     body.Phone,
		Password:  body.Password,
		RequestIP: clientIP(r),
		Address: models.Address{
			ZipCode: body.Address.ZipCode, Street: body.Address.Street,
			Number: body.Address.Number, Complement: deref(body.Address.Complement),
			Neighborhood: body.Address.Neighborhood, City: body.Address.City,
			State: body.Address.State, Country: deref(body.Address.Country),
		},
	})
	if err != nil {
		c.writeRegisterError(w, r, err)
		return
	}

	if !c.openSession(w, r, acc) {
		return
	}
	WriteJSON(w, http.StatusCreated, toAPIAccount(acc))
}

// writeRegisterError traduz o erro do model em HTTP.
//
// Regra: o corpo é genérico, o log é específico. A exceção deliberada é o e-mail
// já usado na DAV — sem uma mensagem acionável, o usuário (tipicamente um casal
// que compartilha e-mail) fica sem saber o que fazer.
func (c AuthController) writeRegisterError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, models.ErrInvalidRegistration):
		WriteProblem(w, http.StatusBadRequest, "dados inválidos",
			"confira os dados informados e tente novamente")

	case errors.Is(err, models.ErrEmailTakenAtDAV):
		WriteProblem(w, http.StatusConflict, "e-mail indisponível",
			"este e-mail já está vinculado a outro paciente. Use um e-mail pessoal, não compartilhado.")

	case errors.Is(err, models.ErrAlreadyRegistered):
		// Genérico de propósito: dizer se colidiu o CPF ou o e-mail permitiria
		// varrer CPFs para descobrir quem tem conta.
		WriteProblem(w, http.StatusConflict, "não foi possível concluir o cadastro",
			"se você já tem conta, faça login ou recupere o acesso")

	case errors.Is(err, models.ErrDAVUnavailable):
		slog.WarnContext(r.Context(), "cadastro: a DAV não confirmou", "error", err)
		WriteProblem(w, http.StatusGatewayTimeout, "serviço indisponível no momento",
			"não conseguimos concluir seu cadastro agora. Tente novamente em alguns minutos.")

	default:
		slog.ErrorContext(r.Context(), "cadastro: erro inesperado", "error", err)
		WriteProblem(w, http.StatusInternalServerError, "erro interno",
			"não foi possível concluir seu cadastro")
	}
}

// Login abre uma sessão.
func (c AuthController) Login(w http.ResponseWriter, r *http.Request) {
	var body api.LoginRequest
	if !decodeJSON(w, r, &body) {
		return
	}

	acc, err := c.Accounts.Authenticate(r.Context(), body.Cpf, body.Password)
	if err != nil {
		if !errors.Is(err, models.ErrInvalidCredentials) {
			slog.ErrorContext(r.Context(), "login: erro inesperado", "error", err)
		}
		// Resposta idêntica para CPF inexistente, senha errada e conta pendente.
		// O model já equaliza o TEMPO; aqui equalizamos o texto.
		WriteProblem(w, http.StatusUnauthorized, "credenciais inválidas",
			"não foi possível entrar com esses dados")
		return
	}

	if !c.openSession(w, r, acc) {
		return
	}
	WriteJSON(w, http.StatusOK, toAPIAccount(acc))
}

// Logout revoga a sessão no servidor e limpa o cookie.
func (c AuthController) Logout(w http.ResponseWriter, r *http.Request) {
	if ck, err := r.Cookie(SessionCookieName); err == nil {
		if err := c.Sessions.Revoke(r.Context(), ck.Value); err != nil {
			slog.ErrorContext(r.Context(), "logout: falha ao revogar sessão", "error", err)
		}
	}
	http.SetCookie(w, c.cookie("", -1))
	w.WriteHeader(http.StatusNoContent)
}

// Me devolve a conta do portador da sessão. Só roda atrás de RequireSession.
func (c AuthController) Me(w http.ResponseWriter, r *http.Request) {
	acc, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "não autenticado", "faça login para continuar")
		return
	}
	WriteJSON(w, http.StatusOK, toAPIAccount(acc))
}

// RequireSession valida o cookie e injeta a conta no contexto.
func RequireSession(sessions Sessions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ck, err := r.Cookie(SessionCookieName)
			if err != nil || ck.Value == "" {
				WriteProblem(w, http.StatusUnauthorized, "não autenticado", "faça login para continuar")
				return
			}

			acc, err := sessions.Validate(r.Context(), ck.Value)
			if err != nil {
				// Falha de INFRA (banco fora) não é veredito sobre a sessão.
				// Devolver 401 aqui deslogaria todo mundo durante um incidente e
				// esconderia a causa atrás de "sua sessão expirou".
				if !errors.Is(err, models.ErrNoSession) {
					slog.ErrorContext(r.Context(), "sessão: erro ao validar", "error", err)
					WriteProblem(w, http.StatusServiceUnavailable, "serviço indisponível",
						"tente novamente em instantes")
					return
				}
				WriteProblem(w, http.StatusUnauthorized, "sessão expirada", "faça login novamente")
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), accountCtxKey, acc)))
		})
	}
}

// AccountFrom recupera a conta injetada pelo RequireSession.
func AccountFrom(ctx context.Context) (models.Account, bool) {
	acc, ok := ctx.Value(accountCtxKey).(models.Account)
	return acc, ok
}

// ---------------------------------------------------------------------------
// Auxiliares
// ---------------------------------------------------------------------------

func (c AuthController) openSession(w http.ResponseWriter, r *http.Request, acc models.Account) bool {
	token, _, err := c.Sessions.Create(r.Context(), acc.ID)
	if err != nil {
		slog.ErrorContext(r.Context(), "sessão: falha ao criar", "error", err)
		WriteProblem(w, http.StatusInternalServerError, "erro interno", "não foi possível iniciar a sessão")
		return false
	}
	http.SetCookie(w, c.cookie(token, int(c.SessionTTL.Seconds())))
	return true
}

// cookie monta o cookie de sessão.
//
// HttpOnly é fixo, não configurável: é ele que impede JavaScript (e portanto
// XSS) de ler o token. Secure é configurável só porque o desenvolvimento local
// roda sem TLS.
func (c AuthController) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   c.CookieSecure,
		// Lax e não Strict: com Strict, quem chega por link externo (e-mail de
		// convite) apareceria deslogado mesmo tendo sessão válida.
		SameSite: http.SameSiteLaxMode,
	}
}

// decodeJSON lê o corpo. Devolve false (e já respondeu 400) se não der.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	// Campo desconhecido é erro: protege contra o cliente achar que mandou algo
	// que silenciosamente ignoramos.
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		// LGPD: o corpo de autenticação NUNCA vai para o log, nem no erro — ele
		// carrega a senha em claro. Só o fato de ter falhado.
		slog.WarnContext(r.Context(), "corpo de requisição inválido", "path", r.URL.Path)
		WriteProblem(w, http.StatusBadRequest, "requisição inválida",
			"o corpo enviado não está no formato esperado")
		return false
	}
	return true
}

// clientIP extrai o IP para a auditoria. O middleware.RealIP do chi já resolveu
// os cabeçalhos de proxy antes de chegar aqui.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func toAPIAccount(a models.Account) api.Account {
	return api.Account{
		Id:       a.ID,
		FullName: a.FullName,
		Email:    openapi_types.Email(a.Email),
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
