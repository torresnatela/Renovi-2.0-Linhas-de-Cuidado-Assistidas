package careline_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/models/careline"
)

// hasErrContaining: alguma mensagem da lista contém o trecho.
func hasErrContaining(errs []string, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e, sub) {
			return true
		}
	}
	return false
}

func TestValidatePublish(t *testing.T) {
	legado := []string{"Psicologia", "Psiquiatria", "Nutrição"}
	// item com código já normalizado casando nome acentuado do legado
	nutricao := careline.Item{Ref: "nutricao", Kind: "consulta", SpecialtyCode: "NUTRICAO", Label: "Nutrição"}

	t.Run("linha_sem_itens", func(t *testing.T) {
		errs := careline.ValidatePublish(nil, nil, legado)
		require.NotEmpty(t, errs)
		assert.True(t, hasErrContaining(errs, "pelo menos um item"), "errs: %v", errs)
	})

	t.Run("regra_com_params_invalidos", func(t *testing.T) {
		rules := map[string][]careline.Rule{
			"psicologia": {{Type: careline.RuleQuota, Params: json.RawMessage(`{"max":0,"period":"month"}`)}},
		}
		errs := careline.ValidatePublish([]careline.Item{psicologia}, rules, legado)
		require.NotEmpty(t, errs)
		assert.True(t, hasErrContaining(errs, "max"), "errs: %v", errs)
	})

	t.Run("window_calendar_rejeitada", func(t *testing.T) {
		rules := map[string][]careline.Rule{
			"psicologia": {{Type: careline.RuleQuota, Params: json.RawMessage(`{"max":4,"period":"month","window":"calendar"}`)}},
		}
		errs := careline.ValidatePublish([]careline.Item{psicologia}, rules, legado)
		require.NotEmpty(t, errs)
		assert.True(t, hasErrContaining(errs, "calendar não é suportada"), "errs: %v", errs)
	})

	t.Run("prerequisite_item_ref_inexistente", func(t *testing.T) {
		rules := map[string][]careline.Rule{
			"psicologia": {prerequisite("fantasma", careline.StatusRealizada, 90)},
		}
		errs := careline.ValidatePublish([]careline.Item{psicologia}, rules, legado)
		require.NotEmpty(t, errs)
		assert.True(t, hasErrContaining(errs, "fantasma"), "errs: %v", errs)
	})

	t.Run("ciclo_de_prerequisites", func(t *testing.T) {
		rules := map[string][]careline.Rule{
			"psicologia":  {prerequisite("psiquiatria", careline.StatusRealizada, 90)},
			"psiquiatria": {prerequisite("psicologia", careline.StatusRealizada, 90)},
		}
		errs := careline.ValidatePublish([]careline.Item{psicologia, psiquiatria}, rules, legado)
		require.NotEmpty(t, errs)
		assert.True(t, hasErrContaining(errs, "ciclo"), "errs: %v", errs)
	})

	t.Run("especialidade_inexistente_no_legado", func(t *testing.T) {
		acupuntura := careline.Item{Ref: "acupuntura", Kind: "consulta", SpecialtyCode: "ACUPUNTURA", Label: "Acupuntura"}
		errs := careline.ValidatePublish([]careline.Item{acupuntura}, nil, legado)
		require.NotEmpty(t, errs)
		assert.True(t, hasErrContaining(errs, "especialidade"), "errs: %v", errs)
		assert.True(t, hasErrContaining(errs, "ACUPUNTURA"), "errs: %v", errs)
	})

	t.Run("caso_feliz_com_normalizacao_de_acentos", func(t *testing.T) {
		rules := map[string][]careline.Rule{
			"psicologia": {quotaMonthly(4), minInterval(7),
				prerequisite("psiquiatria", careline.StatusRealizada, 90)},
			"psiquiatria": {quotaTotal(2)},
		}
		errs := careline.ValidatePublish([]careline.Item{psicologia, psiquiatria, nutricao}, rules, legado)
		assert.Empty(t, errs, "linha válida deveria publicar; errs: %v", errs)
	})

	t.Run("multiplos_erros_acumulados", func(t *testing.T) {
		// um item com: especialidade desconhecida + quota calendar + prereq fantasma
		item := careline.Item{Ref: "a", Kind: "consulta", SpecialtyCode: "XPTO", Label: "A"}
		rules := map[string][]careline.Rule{
			"a": {
				{Type: careline.RuleQuota, Params: json.RawMessage(`{"max":4,"period":"month","window":"calendar"}`)},
				prerequisite("z", careline.StatusRealizada, 30),
			},
		}
		errs := careline.ValidatePublish([]careline.Item{item}, rules, legado)
		assert.GreaterOrEqual(t, len(errs), 3, "não pode parar no primeiro erro; errs: %v", errs)
	})
}
