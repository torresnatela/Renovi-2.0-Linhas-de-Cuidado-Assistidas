//go:build integration

package models_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/models/careline"
	"github.com/renovisaude/renovi-care/internal/testsupport"
)

// fakeSpecialties é o SpecialtyLister que o publish consulta. Aceita Psicologia e
// Psiquiatria (as especialidades da linha piloto do teste).
type fakeSpecialties struct {
	specs []agenda.Specialty
	err   error
}

func (f fakeSpecialties) ListSpecialties(context.Context, time.Time) ([]agenda.Specialty, error) {
	return f.specs, f.err
}

func newCareStores(t *testing.T, specs fakeSpecialties) (*models.CareLineStore, *models.EnrollmentStore, *pgxpool.Pool) {
	t.Helper()
	dsn := testsupport.StartPostgres(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return models.NewCareLineStore(pool, specs), models.NewEnrollmentStore(pool), pool
}

func insertPatient(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
		INSERT INTO patient_account (id, full_name, email, phone, birth_date, password_hash, status)
		VALUES ($1, 'Paciente Teste', $2, '11999999999', '1990-01-01', 'x', 'PENDING_DAV')`,
		id, id.String()+"@example.com")
	require.NoError(t, err)
	return id
}

func aceitaAmbas() fakeSpecialties {
	return fakeSpecialties{specs: []agenda.Specialty{
		{ID: "1", Name: "Psicologia"},
		{ID: "2", Name: "Psiquiatria"},
	}}
}

// TestCareLineAdminFlow exercita o fluxo real do slice contra Postgres: monta a
// linha, prende regras, publica, matricula, renova e encerra — e confere os
// eventos gravados na jornada.
func TestCareLineAdminFlow(t *testing.T) {
	ctx := context.Background()
	catalog, enroll, pool := newCareStores(t, aceitaAmbas())

	// --- Monta a linha (draft) -------------------------------------------------
	line, err := catalog.Create(ctx, "saude-mental", "Saúde Mental", "linha piloto")
	require.NoError(t, err)
	require.Equal(t, "draft", line.Status)
	require.Equal(t, 1, line.Version)

	_, err = catalog.AddItem(ctx, line.ID, models.AddItemInput{
		Ref: "aval", SpecialtyCode: "Psicologia", Label: "Avaliação inicial",
	})
	require.NoError(t, err, "kind vazio deve cair no default CONSULTA")

	acomp, err := catalog.AddItem(ctx, line.ID, models.AddItemInput{
		Ref: "acomp", Kind: "CONSULTA", SpecialtyCode: "Psiquiatria", Label: "Acompanhamento",
	})
	require.NoError(t, err)
	require.Equal(t, "CONSULTA", acomp.Kind)

	// Regra com params inválidos é barrada JÁ na inserção (não vira lixo no banco).
	_, err = catalog.AddRule(ctx, line.ID, "acomp", careline.RuleQuota, json.RawMessage(`{"max":0,"period":"month"}`))
	require.ErrorAs(t, err, &models.ErrCareLineInvalid{}, "max<1 deve reprovar na inserção")

	require.NoError(t, add(ctx, catalog, line.ID, "acomp", careline.RuleQuota, `{"max":4,"period":"month"}`))
	require.NoError(t, add(ctx, catalog, line.ID, "acomp", careline.RuleMinInterval, `{"days":7}`))
	require.NoError(t, add(ctx, catalog, line.ID, "acomp", careline.RuleMaxAdvance, `{"days":60}`))
	require.NoError(t, add(ctx, catalog, line.ID, "acomp", careline.RulePrerequisite, `{"item_ref":"aval","status":"realizada","within_days":90}`))

	// --- Publica ---------------------------------------------------------------
	published, err := catalog.Publish(ctx, line.ID, time.Now())
	require.NoError(t, err)
	require.Equal(t, "published", published.Status)
	require.NotNil(t, published.PublishedAt)
	require.Len(t, published.Items, 2)

	// Criar o mesmo code de novo dá a versão 2, em draft.
	v2, err := catalog.Create(ctx, "saude-mental", "Saúde Mental v2", "")
	require.NoError(t, err)
	require.Equal(t, 2, v2.Version)
	require.Equal(t, "draft", v2.Status)

	// Uma linha publicada é imutável.
	_, err = catalog.AddItem(ctx, line.ID, models.AddItemInput{
		Ref: "extra", SpecialtyCode: "Psicologia", Label: "Extra",
	})
	require.ErrorIs(t, err, models.ErrCareLinePublished)

	// --- Matricula -------------------------------------------------------------
	patient := insertPatient(t, pool)
	now1 := time.Now().UTC().Truncate(time.Microsecond)

	e, err := enroll.Enroll(ctx, patient, "saude-mental", 2, now1)
	require.NoError(t, err)
	require.Equal(t, "ativa", e.Status)
	require.Equal(t, 1, e.CareLineVersion, "matrícula congela a versão publicada")
	wantUntil := now1.Add(2 * careline.MonthWindow)
	require.True(t, wantUntil.Equal(e.ValidUntil), "valid_until = now + 2×30d")
	require.Len(t, e.Periods, 1)

	// O evento matricula_criada foi gravado com o payload certo.
	evType, actor, payload := latestEvent(t, pool, e.ID)
	require.Equal(t, "matricula_criada", evType)
	require.Equal(t, "admin", actor)
	require.Equal(t, float64(2), payload["months"])
	require.Equal(t, float64(1), payload["care_line_version"])
	require.NotEmpty(t, payload["period_id"])
	require.NotEmpty(t, payload["care_line_id"])

	// Segunda matrícula viva na mesma linha é barrada.
	_, err = enroll.Enroll(ctx, patient, "saude-mental", 1, now1)
	require.ErrorIs(t, err, models.ErrEnrollmentAlive)

	// --- Renova (contíguo) -----------------------------------------------------
	now2 := now1.Add(10 * 24 * time.Hour)
	r, err := enroll.Renew(ctx, e.ID, 1, now2)
	require.NoError(t, err)
	wantUntil2 := wantUntil.Add(careline.MonthWindow) // avança exatamente 1×30d
	require.True(t, wantUntil2.Equal(r.ValidUntil), "renovação contígua avança 30d do valid_until anterior")
	require.Len(t, r.Periods, 2)
	// O novo período começa onde o anterior terminou (paciente não perde dias).
	novo := r.Periods[len(r.Periods)-1]
	require.True(t, wantUntil.Equal(novo.StartsAt), "período novo começa no valid_until antigo")
	require.True(t, wantUntil2.Equal(novo.EndsAt))

	// --- Encerra ---------------------------------------------------------------
	ended, err := enroll.End(ctx, e.ID, careline.EnrollmentEncerrada, "piloto concluído", now2)
	require.NoError(t, err)
	require.Equal(t, "encerrada", ended.Status)

	evType, _, _ = latestEvent(t, pool, e.ID)
	require.Equal(t, "matricula_encerrada", evType)

	// Encerrar de novo é 409 (desfecho final).
	_, err = enroll.End(ctx, e.ID, careline.EnrollmentConcluida, "x", now2)
	require.ErrorIs(t, err, models.ErrEnrollmentClosed)

	// Renovar uma encerrada também não vale.
	_, err = enroll.Renew(ctx, e.ID, 1, now2)
	require.ErrorIs(t, err, models.ErrEnrollmentClosed)
}

// TestPublish_CicloDePrerequisito_Reprova prova que o publish acumula os erros de
// validação em vez de gravar uma linha inconsistente.
func TestPublish_CicloDePrerequisito_Reprova(t *testing.T) {
	ctx := context.Background()
	catalog, _, _ := newCareStores(t, aceitaAmbas())

	line, err := catalog.Create(ctx, "ciclo", "Linha com ciclo", "")
	require.NoError(t, err)
	_, err = catalog.AddItem(ctx, line.ID, models.AddItemInput{Ref: "a", SpecialtyCode: "Psicologia", Label: "A"})
	require.NoError(t, err)
	_, err = catalog.AddItem(ctx, line.ID, models.AddItemInput{Ref: "b", SpecialtyCode: "Psiquiatria", Label: "B"})
	require.NoError(t, err)
	require.NoError(t, add(ctx, catalog, line.ID, "a", careline.RulePrerequisite, `{"item_ref":"b","status":"realizada","within_days":30}`))
	require.NoError(t, add(ctx, catalog, line.ID, "b", careline.RulePrerequisite, `{"item_ref":"a","status":"realizada","within_days":30}`))

	_, err = catalog.Publish(ctx, line.ID, time.Now())
	var inv models.ErrCareLineInvalid
	require.ErrorAs(t, err, &inv)
	require.NotEmpty(t, inv.Errors, "o publish precisa devolver a lista de problemas")

	// A linha continua draft: um publish reprovado não muda nada.
	after, err := catalog.Get(ctx, line.ID)
	require.NoError(t, err)
	require.Equal(t, "draft", after.Status)
}

// TestPublish_LegadoIndisponivel_Propaga garante que uma falha do legado vira 503
// no controller (o erro sobe intacto).
func TestPublish_LegadoIndisponivel_Propaga(t *testing.T) {
	ctx := context.Background()
	catalog, _, _ := newCareStores(t, fakeSpecialties{err: agenda.ErrUnavailable})

	line, err := catalog.Create(ctx, "sem-legado", "Sem legado", "")
	require.NoError(t, err)
	_, err = catalog.AddItem(ctx, line.ID, models.AddItemInput{Ref: "a", SpecialtyCode: "Psicologia", Label: "A"})
	require.NoError(t, err)

	_, err = catalog.Publish(ctx, line.ID, time.Now())
	require.ErrorIs(t, err, agenda.ErrUnavailable)
}

func add(ctx context.Context, c *models.CareLineStore, lineID uuid.UUID, ref, ruleType, params string) error {
	_, err := c.AddRule(ctx, lineID, ref, ruleType, json.RawMessage(params))
	return err
}

func latestEvent(t *testing.T, pool *pgxpool.Pool, enrollmentID uuid.UUID) (eventType, actor string, payload map[string]any) {
	t.Helper()
	var raw []byte
	err := pool.QueryRow(context.Background(), `
		SELECT event_type, actor, payload FROM journey_event
		WHERE enrollment_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT 1`, enrollmentID).
		Scan(&eventType, &actor, &raw)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &payload))
	return eventType, actor, payload
}
