package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/renovisaude/renovi-care/internal/models/mood/scoring"
)

// TestRiskStreak fixa a semântica do gatilho (Anexo C.5.4): a sequência é de dias
// de CALENDÁRIO consecutivos em risco a partir do mais recente — uma lacuna de dia
// (ou um dia fora de risco) a encerra — E precisa ser RECENTE (o mais novo é hoje
// ou ontem). Impede tanto o streak falso de check-ins avulsos quanto um streak
// antigo e interrompido oferecendo o WHO-5 para sempre.
func TestRiskStreak(t *testing.T) {
	d := func(dia int) time.Time { return time.Date(2026, time.July, dia, 0, 0, 0, 0, time.UTC) }
	hoje := d(19)
	risco := scoring.QuadranteDesagradavelCalmo
	ok := scoring.QuadranteAgradavelAtivado

	tests := []struct {
		nome  string
		rows  []riskDay
		today time.Time
		want  int
	}{
		{"sem check-ins", nil, hoje, 0},
		{"mais recente fora de risco", []riskDay{{d(19), ok}, {d(18), risco}}, hoje, 0},
		{"quatro dias contíguos em risco", []riskDay{
			{d(19), risco}, {d(18), risco}, {d(17), risco}, {d(16), risco},
		}, hoje, 4},
		{"lacuna de dia quebra a sequência", []riskDay{
			{d(19), risco}, {d(18), risco}, {d(16), risco}, {d(15), risco},
		}, hoje, 2},
		{"dia fora de risco quebra a sequência", []riskDay{
			{d(19), risco}, {d(18), risco}, {d(17), ok}, {d(16), risco},
		}, hoje, 2},
		{"um único dia em risco (hoje)", []riskDay{{d(19), risco}}, hoje, 1},
		{"mais recente é ontem ainda conta", []riskDay{
			{d(18), risco}, {d(17), risco},
		}, hoje, 2},
		{"streak antigo (anterior a ontem) não é recente", []riskDay{
			{d(10), risco}, {d(9), risco}, {d(8), risco}, {d(7), risco},
		}, hoje, 0},
	}
	for _, tc := range tests {
		t.Run(tc.nome, func(t *testing.T) {
			assert.Equal(t, tc.want, riskStreak(tc.rows, tc.today))
		})
	}
}
