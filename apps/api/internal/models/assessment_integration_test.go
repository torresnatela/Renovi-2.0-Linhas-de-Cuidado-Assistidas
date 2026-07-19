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

// TestAssessmentStore_WHO5_Cadencia prova que o anel semanal reusa o MOTOR de
// linhas de cuidado para a cadência (MIN_INTERVAL 7d), pontua com os cortes BR e
// persiste (assessment + respostas + fato na jornada).
func TestAssessmentStore_WHO5_Cadencia(t *testing.T) {
	ctx := context.Background()
	catalog, enroll, pool := newCareStores(t, aceitaAmbas())
	consents := models.NewConsentStore(pool)
	assess := models.NewAssessmentStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Linha com o WHO-5 como ATIVIDADE + cadência mínima de 7 dias.
	line, err := catalog.Create(ctx, "bem-estar", "Bem-estar", "")
	require.NoError(t, err)
	_, err = catalog.AddItem(ctx, line.ID, models.AddItemInput{
		Ref: models.Who5ItemRef, Kind: careline.KindAtividade, Label: "WHO-5",
	})
	require.NoError(t, err)
	_, err = catalog.AddRule(ctx, line.ID, models.Who5ItemRef, careline.RuleMinInterval, json.RawMessage(`{"days":7}`))
	require.NoError(t, err)
	_, err = catalog.Publish(ctx, line.ID, now)
	require.NoError(t, err)

	patient := insertPatient(t, pool)

	// Sem consentimento: barra.
	_, err = assess.Submit(ctx, patient, models.Who5Codigo, []int{0, 0, 0, 0, 0}, now)
	require.ErrorIs(t, err, models.ErrNoActiveConsent)

	_, err = consents.Grant(ctx, patient, models.ConsentCheckinHumor, "v1", nil, now)
	require.NoError(t, err)
	_, err = enroll.Enroll(ctx, patient, "bem-estar", 2, now)
	require.NoError(t, err)

	// Disponível (sem aplicação anterior).
	av, err := assess.Availability(ctx, patient, models.Who5Codigo, now)
	require.NoError(t, err)
	require.True(t, av.Eligibility.Allowed)
	require.Equal(t, 5, av.ItemCount)

	// WHO-5 no piso (0×5): índice 0 (<28) => encaminha, flag positiva.
	res, err := assess.Submit(ctx, patient, models.Who5Codigo, []int{0, 0, 0, 0, 0}, now)
	require.NoError(t, err)
	require.Equal(t, scoring.FaixaEncaminha, res.Faixa)
	require.True(t, res.FlagEncaminhar)
	require.NotNil(t, res.IndexScore)
	require.Equal(t, 0.0, *res.IndexScore)

	// Emitiu o fato + gravou as 5 respostas.
	var evCount, respCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM journey_event WHERE patient_id=$1 AND event_type='assessment_respondido'`,
		patient).Scan(&evCount))
	require.Equal(t, 1, evCount)
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM assessment_item_response`).Scan(&respCount))
	require.Equal(t, 5, respCount)

	// 2 dias depois: bloqueado por MIN_INTERVAL (derivado sob demanda).
	av, err = assess.Availability(ctx, patient, models.Who5Codigo, now.Add(2*24*time.Hour))
	require.NoError(t, err)
	require.False(t, av.Eligibility.Allowed)
	require.Len(t, av.Eligibility.Blocks, 1)
	require.Equal(t, careline.RuleMinInterval, av.Eligibility.Blocks[0].RuleType)
	require.NotNil(t, av.Eligibility.Blocks[0].AvailableFrom)

	// Submit precoce é barrado (ErrAssessmentBlocked carrega os blocks).
	_, err = assess.Submit(ctx, patient, models.Who5Codigo, []int{3, 3, 3, 3, 3}, now.Add(2*24*time.Hour))
	var blocked models.ErrAssessmentBlocked
	require.ErrorAs(t, err, &blocked)
	require.NotEmpty(t, blocked.Blocks)

	// Exatamente 7 dias depois: liberado de novo (distância == D é permitida).
	later := now.Add(7 * 24 * time.Hour)
	av, err = assess.Availability(ctx, patient, models.Who5Codigo, later)
	require.NoError(t, err)
	require.True(t, av.Eligibility.Allowed)

	res2, err := assess.Submit(ctx, patient, models.Who5Codigo, []int{5, 5, 5, 5, 5}, later)
	require.NoError(t, err)
	require.Equal(t, scoring.FaixaNormal, res2.Faixa)
	require.False(t, res2.FlagEncaminhar)
	require.Equal(t, 100.0, *res2.IndexScore)
}
