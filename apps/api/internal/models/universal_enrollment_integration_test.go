//go:build integration

package models_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/models/mood/scoring"
)

// TestUniversalMentalHealth_SeedPublicada prova que a migration 0015 semeia a linha
// universal já PUBLICADA, com os 3 itens ATIVIDADE e as cadências dos anéis periódicos
// (o anel diário fica sem regra — o "1 por dia" é do upsert, não do motor).
func TestUniversalMentalHealth_SeedPublicada(t *testing.T) {
	_, _, pool := newCareStores(t, aceitaAmbas())
	ctx := context.Background()

	var status string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM care_line WHERE code=$1`, models.UniversalMentalHealthCode).Scan(&status))
	require.Equal(t, "published", status)

	var itemCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM care_line_item i
		JOIN care_line c ON c.id = i.care_line_id
		WHERE c.code=$1 AND i.kind='ATIVIDADE' AND i.specialty_code IS NULL`,
		models.UniversalMentalHealthCode).Scan(&itemCount))
	require.Equal(t, 3, itemCount, "GRID diário + WHO-5 + PHQ-4")

	var ruleCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM care_line_rule r
		JOIN care_line_item i ON i.id = r.care_line_item_id
		JOIN care_line c ON c.id = i.care_line_id
		WHERE c.code=$1 AND r.rule_type='MIN_INTERVAL'`,
		models.UniversalMentalHealthCode).Scan(&ruleCount))
	require.Equal(t, 2, ruleCount, "MIN_INTERVAL só em WHO-5 (7d) e PHQ-4 (14d)")
}

// TestUniversalMentalHealth_RegisterAutoMatricula prova que ativar a conta (Register →
// commitLink) matricula automaticamente na linha universal, com vigência perpétua, e
// que essa matrícula NÃO aparece na jornada (não é "plano" no perfil).
func TestUniversalMentalHealth_RegisterAutoMatricula(t *testing.T) {
	ctx := context.Background()
	store, pool := newStore(t, &fakeDAV{}) // fakeDAV/validInput vêm de account_integration_test.go

	acc, err := store.Register(ctx, validInput())
	require.NoError(t, err)

	// Exatamente uma matrícula ATIVA na linha universal, vigência lá em 2999.
	var count int
	var validUntil time.Time
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*), min(valid_until) FROM enrollment
		WHERE patient_id=$1 AND care_line_code=$2 AND status='ativa'`,
		acc.ID, models.UniversalMentalHealthCode).Scan(&count, &validUntil))
	require.Equal(t, 1, count)
	require.Equal(t, 2999, validUntil.Year(), "vigência perpétua (sentinela distante)")

	// Emitiu o fato matricula_criada (actor=sistema).
	var evCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM journey_event je
		JOIN enrollment e ON e.id = je.enrollment_id
		WHERE e.patient_id=$1 AND e.care_line_code=$2
		  AND je.event_type='matricula_criada' AND je.actor='sistema'`,
		acc.ID, models.UniversalMentalHealthCode).Scan(&evCount))
	require.Equal(t, 1, evCount)

	// A jornada (repo) NÃO enxerga a linha universal → perfil segue "Sem plano ativo".
	repo := models.NewJourneyRepo(pool)
	snaps, err := repo.ListEnrollmentsByPatient(ctx, acc.ID)
	require.NoError(t, err)
	require.Empty(t, snaps, "linha universal fica fora da listagem de planos da jornada")
}

// TestUniversalMentalHealth_SemPlanoFazHumorEAssessment é o coração do Degrau 1: uma
// conta SEM matrícula em linha real, apenas com a matrícula universal automática,
// consegue fazer o check-in diário e o WHO-5 (após consentimento), com a cadência do
// motor ainda valendo — tudo sem nenhum Enroll explícito.
func TestUniversalMentalHealth_SemPlanoFazHumorEAssessment(t *testing.T) {
	ctx := context.Background()
	store, pool := newStore(t, &fakeDAV{})
	consents := models.NewConsentStore(pool)
	mood := models.NewMoodCheckinStore(pool)
	assess := models.NewAssessmentStore(pool)

	acc, err := store.Register(ctx, validInput())
	require.NoError(t, err)
	patient := acc.ID

	// `now` seguro depois do valid_from (fixado no time.Now() do commitLink).
	now := time.Now().UTC().Truncate(time.Microsecond).Add(time.Minute)

	// Sem consentimento: barra (LGPD permanece).
	_, err = mood.Record(ctx, patient, models.MoodCheckinInput{Valencia: 20, Energia: 20}, now)
	require.ErrorIs(t, err, models.ErrNoActiveConsent)

	// Concede consentimento — e agora o check-in funciona SEM plano.
	_, err = consents.Grant(ctx, patient, models.ConsentCheckinHumor, "v1", nil, now)
	require.NoError(t, err)

	today, err := mood.Today(ctx, patient, now)
	require.NoError(t, err)
	require.True(t, today.CanCheckin, "sem plano, mas com a linha universal, o check-in é liberado")
	require.Empty(t, today.Reason)

	c, err := mood.Record(ctx, patient, models.MoodCheckinInput{Valencia: 20, Energia: 20}, now)
	require.NoError(t, err)
	require.Equal(t, scoring.QuadranteDesagradavelCalmo, c.Quadrante)

	// O check-in gravou apontando para a matrícula universal.
	var enrolledCode string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT e.care_line_code FROM mood_checkin m
		JOIN enrollment e ON e.id = m.enrollment_id WHERE m.patient_id=$1`,
		patient).Scan(&enrolledCode))
	require.Equal(t, models.UniversalMentalHealthCode, enrolledCode)

	// WHO-5: disponível, responde, e a cadência (MIN_INTERVAL 7d da linha universal)
	// bloqueia a 2ª resposta 2 dias depois — prova de que o motor age sobre a universal.
	av, err := assess.Availability(ctx, patient, models.Who5Codigo, now)
	require.NoError(t, err)
	require.True(t, av.Eligibility.Allowed)

	_, err = assess.Submit(ctx, patient, models.Who5Codigo, []int{2, 2, 2, 2, 2}, now)
	require.NoError(t, err)

	_, err = assess.Submit(ctx, patient, models.Who5Codigo, []int{3, 3, 3, 3, 3}, now.Add(2*24*time.Hour))
	var blocked models.ErrAssessmentBlocked
	require.ErrorAs(t, err, &blocked, "cadência da linha universal ainda barra respostas próximas")

	// 7 dias depois: liberado de novo.
	av, err = assess.Availability(ctx, patient, models.Who5Codigo, now.Add(7*24*time.Hour))
	require.NoError(t, err)
	require.True(t, av.Eligibility.Allowed)
}
