package trigger_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/renovisaude/renovi-care/internal/models/mood/trigger"
)

// Esta tabela É a especificação do gatilho (Anexo C.5.4): o caminho
// NORMAL → OFERECER_WHO5 → OFERECER_PHQ4 → ESCALAR_CLINICO e as saídas para NORMAL.
func TestEvaluate(t *testing.T) {
	tests := []struct {
		nome string
		snap trigger.Snapshot
		p    trigger.Params
		want trigger.State
	}{
		{"sem sinais: normal", trigger.Snapshot{}, trigger.Params{}, trigger.StateNormal},
		{"risco abaixo de N: normal", trigger.Snapshot{RiskStreak: 3}, trigger.Params{N: 4}, trigger.StateNormal},
		{"N dias em risco: oferece WHO-5", trigger.Snapshot{RiskStreak: 4}, trigger.Params{N: 4}, trigger.StateOferecerWHO5},
		{"N default (4)", trigger.Snapshot{RiskStreak: 4}, trigger.Params{}, trigger.StateOferecerWHO5},
		{"WHO-5 respondido e OK: volta ao normal", trigger.Snapshot{RiskStreak: 9, WHO5Recent: true, WHO5Sinaliza: false}, trigger.Params{N: 4}, trigger.StateNormal},
		{"WHO-5 sinaliza: oferece PHQ-4", trigger.Snapshot{RiskStreak: 9, WHO5Recent: true, WHO5Sinaliza: true}, trigger.Params{N: 4}, trigger.StateOferecerPHQ4},
		{"PHQ-4 positivo: escala clínico", trigger.Snapshot{WHO5Recent: true, WHO5Sinaliza: true, PHQ4Recent: true, PHQ4Positivo: true}, trigger.Params{N: 4}, trigger.StateEscalarClinico},
		{"PHQ-4 negativo: encerra o ciclo (normal)", trigger.Snapshot{WHO5Recent: true, WHO5Sinaliza: true, PHQ4Recent: true, PHQ4Positivo: false}, trigger.Params{N: 4}, trigger.StateNormal},
		{"PHQ-4 tem precedência sobre WHO-5", trigger.Snapshot{WHO5Recent: true, WHO5Sinaliza: true, PHQ4Recent: true, PHQ4Positivo: true}, trigger.Params{}, trigger.StateEscalarClinico},
	}
	for _, tc := range tests {
		t.Run(tc.nome, func(t *testing.T) {
			got := trigger.Evaluate(tc.snap, tc.p)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestOfferEEscalate(t *testing.T) {
	assert.Equal(t, "WHO5", trigger.StateOferecerWHO5.Offer())
	assert.Equal(t, "PHQ4", trigger.StateOferecerPHQ4.Offer())
	assert.Equal(t, "", trigger.StateNormal.Offer())
	assert.Equal(t, "", trigger.StateEscalarClinico.Offer())
	assert.True(t, trigger.StateEscalarClinico.Escalate())
	assert.False(t, trigger.StateOferecerPHQ4.Escalate())
}
