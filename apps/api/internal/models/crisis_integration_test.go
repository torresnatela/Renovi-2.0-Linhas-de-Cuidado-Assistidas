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
)

// TestCrisisRouting_HelpNowEEscalonamento cobre o Módulo 6 contra Postgres: a
// afordância de ajuda registra pedido_ajuda e devolve o canal; um rastreio
// positivo emite escalonamento_clinico (actor=sistema); e a muralha — todo evento
// é escopado ao paciente, sem superfície agregada/gestor no schema.
func TestCrisisRouting_HelpNowEEscalonamento(t *testing.T) {
	ctx := context.Background()
	catalog, enroll, pool := newCareStores(t, aceitaAmbas())
	consents := models.NewConsentStore(pool)
	mood := models.NewMoodCheckinStore(pool)
	assess := models.NewAssessmentStore(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	line, err := catalog.Create(ctx, "bem-estar", "Bem-estar", "")
	require.NoError(t, err)
	for _, ref := range []string{models.CheckinHumorDiarioRef, models.Who5ItemRef} {
		_, err = catalog.AddItem(ctx, line.ID, models.AddItemInput{Ref: ref, Kind: careline.KindAtividade, Label: ref})
		require.NoError(t, err)
	}
	_, err = catalog.AddRule(ctx, line.ID, models.Who5ItemRef, careline.RuleMinInterval, json.RawMessage(`{"days":7}`))
	require.NoError(t, err)
	_, err = catalog.Publish(ctx, line.ID, now)
	require.NoError(t, err)

	patient := insertPatient(t, pool)
	_, err = consents.Grant(ctx, patient, models.ConsentCheckinHumor, "v1", nil, now)
	require.NoError(t, err)
	_, err = enroll.Enroll(ctx, patient, "bem-estar", 2, now)
	require.NoError(t, err)

	// "Preciso de ajuda agora": devolve o canal e registra o pedido na jornada.
	ch, err := mood.HelpNow(ctx, patient, now)
	require.NoError(t, err)
	require.Equal(t, "care_navigation", ch.Type)
	require.NotEmpty(t, ch.Message)

	var ajuda int
	var ajudaActor string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM journey_event WHERE patient_id=$1 AND event_type='pedido_ajuda'`, patient).Scan(&ajuda))
	require.Equal(t, 1, ajuda)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT actor FROM journey_event WHERE patient_id=$1 AND event_type='pedido_ajuda'`, patient).Scan(&ajudaActor))
	require.Equal(t, "paciente", ajudaActor)

	// WHO-5 no piso => rastreio positivo => escalonamento_clinico (actor=sistema).
	res, err := assess.Submit(ctx, patient, models.Who5Codigo, []int{0, 0, 0, 0, 0}, now)
	require.NoError(t, err)
	require.True(t, res.FlagEncaminhar)

	var esc int
	var escActor string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM journey_event WHERE patient_id=$1 AND event_type='escalonamento_clinico'`, patient).Scan(&esc))
	require.Equal(t, 1, esc)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT actor FROM journey_event WHERE patient_id=$1 AND event_type='escalonamento_clinico'`, patient).Scan(&escActor))
	require.Equal(t, "sistema", escActor)

	// Muralha: nenhum evento sem paciente — não há superfície agregada/gestor no
	// schema (a camada agregada/anonimizada é outro documento, C.8).
	var semPaciente int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM journey_event WHERE patient_id IS NULL`).Scan(&semPaciente))
	require.Equal(t, 0, semPaciente)
}
