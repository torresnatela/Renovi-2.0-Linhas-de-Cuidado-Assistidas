package controllers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/models"
)

// OnboardingService é o que o controller precisa do model (interface no consumidor,
// ADR-012). O OnboardingStore a implementa.
type OnboardingService interface {
	Info(ctx context.Context, token string) (models.OnboardingInfo, error)
	Complete(ctx context.Context, token string, in models.RegisterInput) (models.Account, error)
	Decline(ctx context.Context, token string) error
}

// OnboardingController expõe as rotas públicas /onboarding/{token} (a conclusão do
// cadastro pelo convite da Gestão). O token na URL é a credencial — sem sessão.
type OnboardingController struct {
	Onboarding OnboardingService
	Sessions   Sessions
	// CookieSecure/SessionTTL vêm da config (mesmo cookie do cadastro/login).
	CookieSecure bool
	SessionTTL   time.Duration
	// CompleteDeadline cobre o orçamento da DAV, como o RegisterDeadline do cadastro.
	CompleteDeadline time.Duration
}

// GetInfo devolve o pré-preenchimento do convite (GET /onboarding/{token}).
func (c OnboardingController) GetInfo(w http.ResponseWriter, r *http.Request) {
	info, err := c.Onboarding.Info(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeOnboardingError(w, r, err)
		return
	}

	out := api.OnboardingInfo{InviteName: info.InviteName, Companies: info.Companies}
	if info.InviteEmail != "" {
		out.InviteEmail = &info.InviteEmail
	}
	if info.InvitePhone != "" {
		out.InvitePhone = &info.InvitePhone
	}
	WriteJSON(w, http.StatusOK, out)
}

// Complete conclui o cadastro pelo convite e abre a sessão
// (POST /onboarding/{token}/complete).
func (c OnboardingController) Complete(w http.ResponseWriter, r *http.Request) {
	var body api.RegisterRequest
	if !decodeJSON(w, r, &body) {
		return
	}

	// Como o cadastro, a conclusão é síncrona e a DAV é lenta: estende o prazo de
	// escrita para a resposta não ser cortada no meio de um cadastro que deu certo.
	deadline := c.CompleteDeadline
	if deadline <= 0 {
		deadline = 90 * time.Second
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(deadline))

	acc, err := c.Onboarding.Complete(r.Context(), chi.URLParam(r, "token"), models.RegisterInput{
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
		writeOnboardingError(w, r, err)
		return
	}

	if !issueSession(w, r, c.Sessions, sessionCookies{secure: c.CookieSecure, ttl: c.SessionTTL}, acc) {
		return
	}
	WriteJSON(w, http.StatusCreated, toAPIAccount(acc))
}

// Decline registra que a pessoa recusou o vínculo (POST /onboarding/{token}/decline).
func (c OnboardingController) Decline(w http.ResponseWriter, r *http.Request) {
	if err := c.Onboarding.Decline(r.Context(), chi.URLParam(r, "token")); err != nil {
		writeOnboardingError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeOnboardingError traduz os erros do onboarding em HTTP. Os erros do token e da
// conclusão têm reason.code para o front distinguir; o restante (erros do cadastro)
// cai no writeRegisterError, a mesma tradução do /auth/register.
func writeOnboardingError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, models.ErrTokenNotFound):
		WriteProblemReason(w, http.StatusNotFound, "convite não encontrado",
			"o link do convite é inválido", Reason{Code: "TOKEN_NOT_FOUND"})
	case errors.Is(err, models.ErrTokenExpired):
		WriteProblemReason(w, http.StatusGone, "convite expirado",
			"peça um novo link ao RH da sua empresa", Reason{Code: "TOKEN_EXPIRED"})
	case errors.Is(err, models.ErrTokenUsed):
		WriteProblemReason(w, http.StatusGone, "convite já utilizado",
			"este convite já foi usado; se já se cadastrou, entre na sua conta", Reason{Code: "TOKEN_USED"})
	case errors.Is(err, models.ErrTokenRevoked):
		WriteProblemReason(w, http.StatusGone, "convite indisponível",
			"este convite não vale mais; peça um novo ao RH", Reason{Code: "TOKEN_REVOKED"})
	case errors.Is(err, models.ErrCPFMismatch):
		WriteProblemReason(w, http.StatusBadRequest, "CPF não confere",
			"o CPF informado não corresponde ao convite", Reason{Code: "CPF_MISMATCH"})
	case errors.Is(err, models.ErrAlreadyHasAccount):
		WriteProblemReason(w, http.StatusConflict, "você já tem cadastro",
			"entre na sua conta para aceitar o vínculo com a empresa", Reason{Code: "ALREADY_HAS_ACCOUNT"})
	case errors.Is(err, models.ErrOnboardingDeclined):
		WriteProblemReason(w, http.StatusConflict, "convite recusado",
			"este vínculo foi recusado", Reason{Code: "ONBOARDING_DECLINED"})
	case errors.Is(err, models.ErrOnboardingAlreadyDone):
		WriteProblemReason(w, http.StatusConflict, "cadastro já concluído",
			"este convite já foi concluído; entre na sua conta", Reason{Code: "ONBOARDING_ALREADY_DONE"})
	default:
		// ErrInvalidRegistration / ErrEmailTakenAtDAV / ErrAlreadyRegistered /
		// ErrDAVUnavailable / inesperado — mesma tradução do cadastro.
		writeRegisterError(w, r, err)
	}
}
