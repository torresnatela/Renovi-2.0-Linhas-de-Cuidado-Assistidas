package models

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/db/gen"
)

// ErrInstrumentNotFound: não há instrumento ativo com o código pedido.
var ErrInstrumentNotFound = errors.New("instrument: instrumento não encontrado")

// InstrumentConfig é a config de um instrumento para o front desenhar a captura.
type InstrumentConfig struct {
	Codigo        string
	Versao        string
	Anel          string
	Dimensions    []InstrumentDimension
	EmotionLabels []EmotionLabel
	ContextTags   []ContextTag
}

// InstrumentDimension é uma dimensão do instrumento (ex.: valência 0–100).
type InstrumentDimension struct {
	Dimensao   string
	Polaridade string
	MinScore   float64
	MaxScore   float64
}

// EmotionLabel é um rótulo de emoção de um quadrante (vocabulário Renovi).
type EmotionLabel struct {
	Quadrante string
	Rotulo    string
}

// ContextTag é uma tag de contexto opcional do check-in (trabalho, sono...).
type ContextTag struct {
	Chave  string
	Rotulo string
}

// InstrumentStore lê o catálogo de instrumentos (reference data versionada).
type InstrumentStore struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

func NewInstrumentStore(pool *pgxpool.Pool) *InstrumentStore {
	return &InstrumentStore{pool: pool, q: gen.New(pool)}
}

// Config devolve a configuração do instrumento ATIVO com o código pedido, com
// dimensões e — para o front do GRID — os rótulos de emoção e tags de contexto.
func (s *InstrumentStore) Config(ctx context.Context, codigo string) (InstrumentConfig, error) {
	inst, err := s.q.GetActiveInstrument(ctx, codigo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return InstrumentConfig{}, ErrInstrumentNotFound
		}
		return InstrumentConfig{}, fmt.Errorf("carregar instrumento: %w", err)
	}

	dims, err := s.q.ListInstrumentDimensions(ctx, inst.ID)
	if err != nil {
		return InstrumentConfig{}, fmt.Errorf("listar dimensões: %w", err)
	}
	labels, err := s.q.ListEmotionLabels(ctx)
	if err != nil {
		return InstrumentConfig{}, fmt.Errorf("listar rótulos: %w", err)
	}
	tags, err := s.q.ListContextTags(ctx)
	if err != nil {
		return InstrumentConfig{}, fmt.Errorf("listar tags: %w", err)
	}

	cfg := InstrumentConfig{Codigo: inst.Codigo, Versao: inst.Versao, Anel: inst.Anel}
	for _, d := range dims {
		cfg.Dimensions = append(cfg.Dimensions, InstrumentDimension{
			Dimensao:   d.Dimensao,
			Polaridade: d.Polaridade,
			MinScore:   numericToFloat(d.MinScore),
			MaxScore:   numericToFloat(d.MaxScore),
		})
	}
	for _, l := range labels {
		cfg.EmotionLabels = append(cfg.EmotionLabels, EmotionLabel{Quadrante: l.Quadrante, Rotulo: l.Rotulo})
	}
	for _, t := range tags {
		cfg.ContextTags = append(cfg.ContextTags, ContextTag{Chave: t.Chave, Rotulo: t.Rotulo})
	}
	return cfg, nil
}

// numericToFloat converte um pgtype.Numeric em float64 (0 quando nulo/inválido).
func numericToFloat(n pgtype.Numeric) float64 {
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}
