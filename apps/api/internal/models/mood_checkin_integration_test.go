//go:build integration

package models_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/models/careline"
	"github.com/renovisaude/renovi-care/internal/models/mood/scoring"
)

// TestMoodCheckinStore_Fluxo exercita o anel diário ponta a ponta contra Postgres:
// pré-condições (consentimento + matrícula), derivação de quadrante, emissão do
// fato na jornada e upsert do dia (mesmo dia atualiza, não duplica).
func TestMoodCheckinStore_Fluxo(t *testing.T) {
	ctx := context.Background()
	catalog, enroll, pool := newCareStores(t, aceitaAmbas())
	consents := models.NewConsentStore(pool)
	mood := models.NewMoodCheckinStore(pool)
	// `now` fixo ao meio-dia UTC (09h em São Paulo): o teste depende de `now` e
	// `now+1h` caírem no MESMO dia local (upsert do dia). Com time.Now() real, rodar
	// perto da meia-noite de Brasília jogaria os dois em dias diferentes (2 linhas).
	now := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)

	// Linha publicada com o item de atividade checkin-humor-diario.
	line, err := catalog.Create(ctx, "bem-estar", "Bem-estar", "")
	require.NoError(t, err)
	_, err = catalog.AddItem(ctx, line.ID, models.AddItemInput{
		Ref: models.CheckinHumorDiarioRef, Kind: careline.KindAtividade, Label: "Check-in de humor",
	})
	require.NoError(t, err)
	_, err = catalog.Publish(ctx, line.ID, now)
	require.NoError(t, err)

	patient := insertPatient(t, pool)
	entrada := models.MoodCheckinInput{Valencia: 20, Energia: 20}

	// Sem consentimento: barra (pré-condição LGPD).
	_, err = mood.Record(ctx, patient, entrada, now)
	require.ErrorIs(t, err, models.ErrNoActiveConsent)

	today, err := mood.Today(ctx, patient, now)
	require.NoError(t, err)
	require.False(t, today.CanCheckin)
	require.Equal(t, models.ReasonConsentRequired, today.Reason)

	// Concede consentimento.
	_, err = consents.Grant(ctx, patient, models.ConsentCheckinHumor, "v1", nil, now)
	require.NoError(t, err)

	// Com consentimento mas sem matrícula: barra.
	_, err = mood.Record(ctx, patient, entrada, now)
	require.ErrorIs(t, err, models.ErrNotEnrolledInActivity)

	today, err = mood.Today(ctx, patient, now)
	require.NoError(t, err)
	require.Equal(t, models.ReasonNotEnrolled, today.Reason)

	// Matricula na linha.
	_, err = enroll.Enroll(ctx, patient, "bem-estar", 2, now)
	require.NoError(t, err)

	// Registra: (20,20) => desagradavel_calmo, com tag de contexto.
	c, err := mood.Record(ctx, patient, models.MoodCheckinInput{Valencia: 20, Energia: 20, ContextTags: []string{"sono"}}, now)
	require.NoError(t, err)
	require.Equal(t, scoring.QuadranteDesagradavelCalmo, c.Quadrante)
	require.Equal(t, []string{"sono"}, c.ContextTags)

	// Emitiu o fato na jornada (append-only).
	var evCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM journey_event WHERE patient_id=$1 AND event_type='checkin_humor_registrado'`,
		patient).Scan(&evCount))
	require.Equal(t, 1, evCount)

	// Mesmo dia atualiza (não duplica): (80,80) => agradavel_ativado.
	c2, err := mood.Record(ctx, patient, models.MoodCheckinInput{Valencia: 80, Energia: 80}, now.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, scoring.QuadranteAgradavelAtivado, c2.Quadrante)

	var rowCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM mood_checkin WHERE patient_id=$1`, patient).Scan(&rowCount))
	require.Equal(t, 1, rowCount, "mesmo dia local = 1 linha (upsert)")

	// Today reflete a atualização.
	today, err = mood.Today(ctx, patient, now.Add(time.Hour))
	require.NoError(t, err)
	require.True(t, today.CanCheckin)
	require.NotNil(t, today.Checkin)
	require.Equal(t, 80, today.Checkin.Valencia)
	require.Empty(t, today.Checkin.ContextTags, "segundo record sem tags sobrescreveu")

	// History tem 1.
	hist, err := mood.History(ctx, patient, 30)
	require.NoError(t, err)
	require.Len(t, hist, 1)
}
