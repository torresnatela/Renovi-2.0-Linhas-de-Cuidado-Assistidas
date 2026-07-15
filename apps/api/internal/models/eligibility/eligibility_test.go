package eligibility

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Smoke test: garante que o contrato do motor compila e é chamável na fundação.
// A suíte table-driven de verdade (cota semanal, dependências N=2, "cancelou ->
// cota volta", "semana ISO vira na segunda"...) entra junto com a implementação
// de Evaluate — este arquivo é o esqueleto dela.
func TestEvaluate_ContractSmoke(t *testing.T) {
	tpl := Template{
		Code:    "saude-mental",
		Version: 1,
		Items: []Item{
			{Code: "consulta-psicologia", Type: "APPOINTMENT",
				Recurrence: &RecurrenceRule{Freq: "WEEKLY", Interval: 1, Quota: 1}},
		},
	}

	got := Evaluate(tpl, nil, time.Now())

	// RESERVADO: hoje Evaluate devolve nil. Quando implementado, troque por
	// asserções reais de veredito (ver docs/PROGRESSO.md).
	assert.Nil(t, got, "Evaluate ainda é stub na fundação")
}

func TestEvaluate_TableDriven_Pending(t *testing.T) {
	t.Skip("RESERVADO: implementar junto com Evaluate (SPEC §8, primeiro código de negócio).")
}
