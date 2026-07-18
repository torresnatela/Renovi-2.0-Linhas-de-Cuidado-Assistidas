package controllers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/models"
)

// InstrumentService é o que o controller precisa do catálogo de instrumentos. A
// interface vive no consumidor (ADR-012).
type InstrumentService interface {
	Config(ctx context.Context, codigo string) (models.InstrumentConfig, error)
}

// MoodCheckinService é o que o controller precisa do anel diário.
type MoodCheckinService interface {
	Record(ctx context.Context, patientID uuid.UUID, in models.MoodCheckinInput, now time.Time) (models.MoodCheckin, error)
	Today(ctx context.Context, patientID uuid.UUID, now time.Time) (models.MoodToday, error)
	History(ctx context.Context, patientID uuid.UUID, limit int) ([]models.MoodCheckin, error)
}

// MoodController expõe as rotas do Verificador de Humor (Anexo C), atrás de
// sessão (cookieAuth). `Now` é injetável para teste.
type MoodController struct {
	Instruments InstrumentService
	Checkins    MoodCheckinService
	Now         func() time.Time
}

func (c MoodController) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// GetInstrument devolve a config de um instrumento (dimensões, rótulos, tags) —
// o que o front usa para desenhar a captura.
func (c MoodController) GetInstrument(w http.ResponseWriter, r *http.Request) {
	if _, ok := AccountFrom(r.Context()); !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "sessão ausente ou inválida")
		return
	}
	codigo := chi.URLParam(r, "codigo")
	cfg, err := c.Instruments.Config(r.Context(), codigo)
	if err != nil {
		if errors.Is(err, models.ErrInstrumentNotFound) {
			WriteProblem(w, http.StatusNotFound, "Instrumento não encontrado", "código desconhecido ou inativo")
			return
		}
		WriteProblem(w, http.StatusInternalServerError, "Erro ao carregar instrumento", "")
		return
	}
	WriteJSON(w, http.StatusOK, toInstrumentConfig(cfg))
}

func toInstrumentConfig(c models.InstrumentConfig) api.InstrumentConfig {
	dims := make([]api.InstrumentDimension, 0, len(c.Dimensions))
	for _, d := range c.Dimensions {
		dims = append(dims, api.InstrumentDimension{
			Dimensao:   d.Dimensao,
			Polaridade: api.InstrumentDimensionPolaridade(d.Polaridade),
			MinScore:   float32(d.MinScore),
			MaxScore:   float32(d.MaxScore),
		})
	}
	labels := make([]api.EmotionLabel, 0, len(c.EmotionLabels))
	for _, l := range c.EmotionLabels {
		labels = append(labels, api.EmotionLabel{Quadrante: l.Quadrante, Rotulo: l.Rotulo})
	}
	tags := make([]api.ContextTag, 0, len(c.ContextTags))
	for _, t := range c.ContextTags {
		tags = append(tags, api.ContextTag{Chave: t.Chave, Rotulo: t.Rotulo})
	}
	return api.InstrumentConfig{
		Codigo:        c.Codigo,
		Versao:        c.Versao,
		Anel:          api.InstrumentConfigAnel(c.Anel),
		Dimensions:    dims,
		EmotionLabels: labels,
		ContextTags:   tags,
	}
}

// RecordCheckin registra (ou atualiza) o check-in de humor do dia.
func (c MoodController) RecordCheckin(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "sessão ausente ou inválida")
		return
	}
	var body api.RecordMoodCheckinRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	in := models.MoodCheckinInput{
		Valencia:     body.Valencia,
		Energia:      body.Energia,
		EmotionLabel: body.EmotionLabel,
	}
	if body.ContextTags != nil {
		in.ContextTags = *body.ContextTags
	}
	checkin, err := c.Checkins.Record(r.Context(), account.ID, in, c.now())
	if err != nil {
		switch {
		case errors.Is(err, models.ErrMoodCheckinInvalid):
			WriteProblem(w, http.StatusBadRequest, "Check-in inválido", "valência e energia devem estar entre 0 e 100")
		case errors.Is(err, models.ErrNoActiveConsent):
			WriteProblemReason(w, http.StatusForbidden, "Consentimento necessário",
				"registre o consentimento antes de responder", Reason{Code: models.ReasonConsentRequired})
		case errors.Is(err, models.ErrNotEnrolledInActivity):
			WriteProblemReason(w, http.StatusForbidden, "Sem matrícula elegível",
				"você não tem uma linha de cuidado ativa com o check-in de humor", Reason{Code: models.ReasonNotEnrolled})
		default:
			WriteProblem(w, http.StatusInternalServerError, "Erro ao registrar check-in", "")
		}
		return
	}
	WriteJSON(w, http.StatusOK, toAPIMoodCheckin(checkin))
}

// GetToday devolve a elegibilidade do dia e o check-in de hoje (se houver).
func (c MoodController) GetToday(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "sessão ausente ou inválida")
		return
	}
	today, err := c.Checkins.Today(r.Context(), account.ID, c.now())
	if err != nil {
		WriteProblem(w, http.StatusInternalServerError, "Erro ao consultar o dia", "")
		return
	}
	WriteJSON(w, http.StatusOK, toAPIMoodToday(today))
}

// GetHistory devolve a série recente de check-ins.
func (c MoodController) GetHistory(w http.ResponseWriter, r *http.Request) {
	account, ok := AccountFrom(r.Context())
	if !ok {
		WriteProblem(w, http.StatusUnauthorized, "Não autenticado", "sessão ausente ou inválida")
		return
	}
	limit := 30
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			limit = n
		}
	}
	checkins, err := c.Checkins.History(r.Context(), account.ID, limit)
	if err != nil {
		WriteProblem(w, http.StatusInternalServerError, "Erro ao listar check-ins", "")
		return
	}
	out := make([]api.MoodCheckin, 0, len(checkins))
	for _, ck := range checkins {
		out = append(out, toAPIMoodCheckin(ck))
	}
	WriteJSON(w, http.StatusOK, out)
}

func toAPIMoodCheckin(c models.MoodCheckin) api.MoodCheckin {
	m := api.MoodCheckin{
		Valencia:     c.Valencia,
		Energia:      c.Energia,
		Quadrante:    c.Quadrante,
		RespondidoEm: c.RespondidoEm,
		EmotionLabel: c.EmotionLabel,
	}
	if len(c.ContextTags) > 0 {
		tags := c.ContextTags
		m.ContextTags = &tags
	}
	return m
}

func toAPIMoodToday(t models.MoodToday) api.MoodToday {
	out := api.MoodToday{
		CanCheckin: t.CanCheckin,
		Dia:        openapi_types.Date{Time: t.Dia},
	}
	if t.Reason != "" {
		reason := api.MoodTodayReason(t.Reason)
		out.Reason = &reason
	}
	if t.Checkin != nil {
		ck := toAPIMoodCheckin(*t.Checkin)
		out.Checkin = &ck
	}
	return out
}
