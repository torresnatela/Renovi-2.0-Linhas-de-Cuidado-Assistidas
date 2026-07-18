package controllers

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/models"
)

// InstrumentService é o que o controller precisa do catálogo de instrumentos. A
// interface vive no consumidor (ADR-012).
type InstrumentService interface {
	Config(ctx context.Context, codigo string) (models.InstrumentConfig, error)
}

// MoodController expõe as rotas do Verificador de Humor (Anexo C), atrás de
// sessão (cookieAuth). Cresce ao longo dos módulos (check-in, hoje, histórico).
type MoodController struct {
	Instruments InstrumentService
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
