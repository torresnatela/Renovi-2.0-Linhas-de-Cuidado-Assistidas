// Package models é a camada "M" do MVC: regra de negócio + acesso a dados do
// renovi_care. Usa o SQL tipado gerado pelo sqlc (internal/db/gen).
//
// example_widget.go é um EXEMPLO de referência do padrão (repository sobre sqlc).
// Ao criar os models reais (PatientAccount, Enrollment, Appointment...), siga
// esta estrutura e remova o exemplo.
//
// Lembrete de arquitetura: a lógica PURA de decisão (ex.: motor da linha de
// cuidado) vive em pacotes como models/careline, isolada e sem I/O. Models
// como este cuidam da orquestração com o banco.
package models

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/db/gen"
)

// ExampleWidget é a representação de domínio (desacoplada da linha gerada).
type ExampleWidget struct {
	ID     uuid.UUID       `json:"id"`
	Name   string          `json:"name"`
	Status string          `json:"status"`
	Config json.RawMessage `json:"config,omitempty"`
}

// ExampleWidgetStore expõe operações sobre example_widget.
type ExampleWidgetStore struct {
	q *gen.Queries
}

// NewExampleWidgetStore recebe o pool (que satisfaz gen.DBTX).
func NewExampleWidgetStore(pool *pgxpool.Pool) *ExampleWidgetStore {
	return &ExampleWidgetStore{q: gen.New(pool)}
}

// Create insere um widget novo, gerando o UUID v7 na aplicação (convenção do
// projeto: PKs UUID v7, ordenáveis no tempo — ver CLAUDE.md).
func (s *ExampleWidgetStore) Create(ctx context.Context, name, status string, config json.RawMessage) (ExampleWidget, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return ExampleWidget{}, fmt.Errorf("gerar uuid v7: %w", err)
	}
	row, err := s.q.CreateExampleWidget(ctx, gen.CreateExampleWidgetParams{
		ID:     id,
		Name:   name,
		Status: status,
		Config: config,
	})
	if err != nil {
		return ExampleWidget{}, err
	}
	return toExampleWidget(row), nil
}

// Get busca um widget por id.
func (s *ExampleWidgetStore) Get(ctx context.Context, id uuid.UUID) (ExampleWidget, error) {
	row, err := s.q.GetExampleWidget(ctx, id)
	if err != nil {
		return ExampleWidget{}, err
	}
	return toExampleWidget(row), nil
}

func toExampleWidget(row gen.ExampleWidget) ExampleWidget {
	return ExampleWidget{
		ID:     row.ID,
		Name:   row.Name,
		Status: row.Status,
		Config: row.Config,
	}
}
