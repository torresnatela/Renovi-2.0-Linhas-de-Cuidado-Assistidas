package scoring_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/models/mood/scoring"
)

// Esta tabela É a especificação da pontuação determinística (Anexo C.4/C.5.2):
// grade valência×energia, WHO-5 (índice = bruto×4) e PHQ-4 (subescalas PHQ-2/GAD-2).
// Cortes entram por parâmetro (vêm de instrument_cutoff, validação BR).

func TestQuadrant(t *testing.T) {
	tests := []struct {
		nome              string
		valencia, energia int
		want              string
		risco             bool
	}{
		{"agradável e ativado", 80, 80, scoring.QuadranteAgradavelAtivado, false},
		{"agradável e calmo", 80, 20, scoring.QuadranteAgradavelCalmo, false},
		{"desagradável e ativado (estresse/ansiedade)", 20, 80, scoring.QuadranteDesagradavelAtivado, true},
		{"desagradável e calmo (depressão/esgotamento)", 20, 20, scoring.QuadranteDesagradavelCalmo, true},
		// Fronteira: 50 é o lado ALTO (agradável / ativado), determinístico.
		{"centro exato cai no lado alto", 50, 50, scoring.QuadranteAgradavelAtivado, false},
		{"valência 49 já é desagradável", 49, 50, scoring.QuadranteDesagradavelAtivado, true},
		{"energia 49 já é calmo", 50, 49, scoring.QuadranteAgradavelCalmo, false},
		{"extremos baixos", 0, 0, scoring.QuadranteDesagradavelCalmo, true},
		{"extremos altos", 100, 100, scoring.QuadranteAgradavelAtivado, false},
	}
	for _, tc := range tests {
		t.Run(tc.nome, func(t *testing.T) {
			got := scoring.Quadrant(tc.valencia, tc.energia)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.risco, scoring.IsQuadranteRisco(got), "risco = baixa valência")
		})
	}
}

func TestScoreWHO5(t *testing.T) {
	cortes := scoring.WHO5Cutoffs{Sinaliza: 50, Encaminha: 28}

	tests := []struct {
		nome           string
		items          []int
		wantIndex      int
		wantFaixa      string
		wantEncaminhar bool
	}{
		{"tudo zero: rastreio positivo (encaminha)", []int{0, 0, 0, 0, 0}, 0, scoring.FaixaEncaminha, true},
		{"tudo máximo: bem-estar pleno", []int{5, 5, 5, 5, 5}, 100, scoring.FaixaNormal, false},
		// bruto 7 -> índice 28: NÃO é < 28, então não encaminha; 28 < 50 => sinaliza.
		{"fronteira do encaminhar (índice 28)", []int{2, 2, 1, 1, 1}, 28, scoring.FaixaSinaliza, false},
		// bruto 6 -> índice 24 < 28 => encaminha.
		{"abaixo do corte de encaminhar (índice 24)", []int{2, 1, 1, 1, 1}, 24, scoring.FaixaEncaminha, true},
		// bruto 12 -> índice 48 < 50 => sinaliza.
		{"baixo bem-estar (índice 48)", []int{3, 3, 2, 2, 2}, 48, scoring.FaixaSinaliza, false},
		// bruto 13 -> índice 52 >= 50 => normal (fronteira do sinaliza).
		{"fronteira do sinaliza (índice 52)", []int{3, 3, 3, 2, 2}, 52, scoring.FaixaNormal, false},
	}
	for _, tc := range tests {
		t.Run(tc.nome, func(t *testing.T) {
			got, err := scoring.ScoreWHO5(tc.items, cortes)
			require.NoError(t, err)
			assert.Equal(t, tc.wantIndex, got.Index)
			assert.Equal(t, tc.wantFaixa, got.Faixa)
			assert.Equal(t, tc.wantEncaminhar, got.FlagEncaminhar)
		})
	}

	t.Run("valida quantidade de itens", func(t *testing.T) {
		_, err := scoring.ScoreWHO5([]int{0, 0, 0, 0}, cortes)
		require.Error(t, err)
	})
	t.Run("valida faixa do item", func(t *testing.T) {
		_, err := scoring.ScoreWHO5([]int{0, 0, 0, 0, 6}, cortes)
		require.Error(t, err)
	})
}

func TestScorePHQ4(t *testing.T) {
	cortes := scoring.PHQ4Cutoffs{SubescalaPositiva: 3, TotalModerado: 6}

	tests := []struct {
		nome           string
		items          []int // [phq1, phq2, gad1, gad2]
		wantPHQ2       int
		wantGAD2       int
		wantTotal      int
		wantFaixa      string
		wantEncaminhar bool
	}{
		{"tudo zero", []int{0, 0, 0, 0}, 0, 0, 0, scoring.FaixaNormal, false},
		// PHQ-2 = 3 (>=3): rastreio positivo de depressão -> encaminha; total 3 < 6 => normal.
		{"subescala depressão positiva", []int{2, 1, 0, 0}, 3, 0, 3, scoring.FaixaNormal, true},
		// GAD-2 = 3 (>=3): rastreio positivo de ansiedade -> encaminha.
		{"subescala ansiedade positiva", []int{0, 0, 2, 1}, 0, 3, 3, scoring.FaixaNormal, true},
		// total = 6 (>=6): moderado; subescalas 3 e 3 também encaminham.
		{"sofrimento moderado + subescalas positivas", []int{2, 1, 2, 1}, 3, 3, 6, scoring.FaixaModerado, true},
		// subescalas 2 e 2 (<3): não encaminha; total 4 < 6 => normal.
		{"sem rastreio positivo", []int{1, 1, 1, 1}, 2, 2, 4, scoring.FaixaNormal, false},
		{"tudo máximo", []int{3, 3, 3, 3}, 6, 6, 12, scoring.FaixaModerado, true},
	}
	for _, tc := range tests {
		t.Run(tc.nome, func(t *testing.T) {
			got, err := scoring.ScorePHQ4(tc.items, cortes)
			require.NoError(t, err)
			assert.Equal(t, tc.wantPHQ2, got.PHQ2)
			assert.Equal(t, tc.wantGAD2, got.GAD2)
			assert.Equal(t, tc.wantTotal, got.Total)
			assert.Equal(t, tc.wantFaixa, got.Faixa)
			assert.Equal(t, tc.wantEncaminhar, got.FlagEncaminhar)
		})
	}

	t.Run("valida quantidade de itens", func(t *testing.T) {
		_, err := scoring.ScorePHQ4([]int{0, 0, 0}, cortes)
		require.Error(t, err)
	})
	t.Run("valida faixa do item", func(t *testing.T) {
		_, err := scoring.ScorePHQ4([]int{0, 0, 0, 4}, cortes)
		require.Error(t, err)
	})
}
