package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/renovisaude/renovi-care/internal/models/mood/scoring"
)

// TestRiskStreak fixa a semântica do gatilho (Anexo C.5.4): a sequência é de dias
// de CALENDÁRIO consecutivos em risco a partir do mais recente — uma lacuna de dia
// (ou um dia fora de risco) a encerra. Impede que check-ins avulsos só nos dias
// ruins formem um streak falso.
func TestRiskStreak(t *testing.T) {
	d := func(dia int) time.Time { return time.Date(2026, time.July, dia, 0, 0, 0, 0, time.UTC) }
	risco := scoring.QuadranteDesagradavelCalmo
	ok := scoring.QuadranteAgradavelAtivado

	tests := []struct {
		nome string
		rows []riskDay
		want int
	}{
		{"sem check-ins", nil, 0},
		{"mais recente fora de risco", []riskDay{{d(19), ok}, {d(18), risco}}, 0},
		{"quatro dias contíguos em risco", []riskDay{
			{d(19), risco}, {d(18), risco}, {d(17), risco}, {d(16), risco},
		}, 4},
		{"lacuna de dia quebra a sequência", []riskDay{
			{d(19), risco}, {d(18), risco}, {d(16), risco}, {d(15), risco},
		}, 2},
		{"dia fora de risco quebra a sequência", []riskDay{
			{d(19), risco}, {d(18), risco}, {d(17), ok}, {d(16), risco},
		}, 2},
		{"um único dia em risco", []riskDay{{d(19), risco}}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.nome, func(t *testing.T) {
			assert.Equal(t, tc.want, riskStreak(tc.rows))
		})
	}
}
