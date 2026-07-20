//go:build integration

package models_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/models/careline"
	"github.com/renovisaude/renovi-care/internal/models/mood/scoring"
)

// TestTrigger_CaminhoCompleto exercita o gatilho de aprofundamento ponta a ponta
// contra Postgres: NORMAL → (N dias em risco) OFERECER_WHO5 → (WHO-5 sinaliza)
// OFERECER_PHQ4 → (PHQ-4 positivo) ESCALAR_CLINICO — tudo derivado sob demanda.
func TestTrigger_CaminhoCompleto(t *testing.T) {
	ctx := context.Background()
	catalog, enroll, pool := newCareStores(t, aceitaAmbas())
	consents := models.NewConsentStore(pool)
	mood := models.NewMoodCheckinStore(pool)
	assess := models.NewAssessmentStore(pool)

	// Linha com os 3 anéis. WHO-5 7d, PHQ-4 14d.
	line, err := catalog.Create(ctx, "bem-estar", "Bem-estar", "")
	require.NoError(t, err)
	for _, it := range []struct{ ref, label string }{
		{models.CheckinHumorDiarioRef, "Check-in"},
		{models.Who5ItemRef, "WHO-5"},
		{models.Phq4ItemRef, "PHQ-4"},
	} {
		_, err = catalog.AddItem(ctx, line.ID, models.AddItemInput{Ref: it.ref, Kind: careline.KindAtividade, Label: it.label})
		require.NoError(t, err)
	}
	_, err = catalog.AddRule(ctx, line.ID, models.Who5ItemRef, careline.RuleMinInterval, json.RawMessage(`{"days":7}`))
	require.NoError(t, err)
	_, err = catalog.AddRule(ctx, line.ID, models.Phq4ItemRef, careline.RuleMinInterval, json.RawMessage(`{"days":14}`))
	require.NoError(t, err)
	_, err = catalog.Publish(ctx, line.ID, time.Now().UTC())
	require.NoError(t, err)

	patient := insertPatient(t, pool)
	base := time.Now().UTC().Truncate(time.Microsecond).Add(-10 * 24 * time.Hour)
	_, err = consents.Grant(ctx, patient, models.ConsentCheckinHumor, "v1", nil, base)
	require.NoError(t, err)
	_, err = enroll.Enroll(ctx, patient, "bem-estar", 3, base)
	require.NoError(t, err)

	// Sem histórico: nenhuma oferta.
	today, err := mood.Today(ctx, patient, base)
	require.NoError(t, err)
	require.Equal(t, "", today.Offer)
	require.False(t, today.Escalate)

	// 4 dias consecutivos em quadrante de risco (20,20 = desagradavel_calmo).
	var dia4 time.Time
	for d := 0; d < 4; d++ {
		dia4 = base.Add(time.Duration(d) * 24 * time.Hour)
		_, err = mood.Record(ctx, patient, models.MoodCheckinInput{Valencia: 20, Energia: 20}, dia4)
		require.NoError(t, err)
	}

	// Deterioração sustentada => oferece WHO-5.
	today, err = mood.Today(ctx, patient, dia4)
	require.NoError(t, err)
	require.Equal(t, "WHO5", today.Offer)

	// Responde WHO-5 no piso (índice 0 < 28 => sinaliza).
	who5res, err := assess.Submit(ctx, patient, models.Who5Codigo, []int{0, 0, 0, 0, 0}, dia4)
	require.NoError(t, err)
	require.Equal(t, scoring.FaixaEncaminha, who5res.Faixa)

	// WHO-5 sinalizando => oferece PHQ-4.
	today, err = mood.Today(ctx, patient, dia4)
	require.NoError(t, err)
	require.Equal(t, "PHQ4", today.Offer)

	// Responde PHQ-4 com depressão positiva ([2,1,0,0] => PHQ-2 = 3 >= 3).
	phq4res, err := assess.Submit(ctx, patient, models.Phq4Codigo, []int{2, 1, 0, 0}, dia4)
	require.NoError(t, err)
	require.True(t, phq4res.FlagEncaminhar)
	require.Equal(t, 3, phq4res.Subscores["phq2"])
	require.Equal(t, 0, phq4res.Subscores["gad2"])
	require.Nil(t, phq4res.IndexScore, "PHQ-4 não tem índice 0–100")

	// PHQ-4 positivo => escalonamento clínico, sem nova oferta.
	today, err = mood.Today(ctx, patient, dia4)
	require.NoError(t, err)
	require.True(t, today.Escalate)
	require.Equal(t, "", today.Offer)
}
