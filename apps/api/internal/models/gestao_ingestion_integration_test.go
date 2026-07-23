//go:build integration

// Testes de INTEGRAÇÃO da ingestão da Gestão contra um Postgres real (0016).
// Cobrem a orquestração SQL (upsert idempotente, ON CONFLICT, índice parcial de
// convite vivo, corrida) — o que um fake não conseguiria emular fielmente. Rode com
// `make test-integration`.
package models_test

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/adapters/notify"
	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/testsupport"
)

type fakeNotifier struct {
	mu   sync.Mutex
	sent []notify.InviteMessage
}

func (f *fakeNotifier) SendInvite(_ context.Context, msg notify.InviteMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
	return nil
}

func (f *fakeNotifier) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

func newIngestionStore(t *testing.T) (*models.GestaoIngestionStore, *pgxpool.Pool, *fakeNotifier) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testsupport.StartPostgres(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	n := &fakeNotifier{}
	return models.NewGestaoIngestionStore(pool, n, 7*24*time.Hour, "https://app.test"), pool, n
}

func hmac32(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func pushInput(cpfHmac []byte, contractID, companyID, status string) models.ContractPush {
	return models.ContractPush{
		ContractID: contractID, Status: status, StartedAt: time.Now(),
		Employee: models.EmployeePush{
			ID: "E-1", CPFHmac: cpfHmac, Name: "Maria de Teste",
			Email: "maria@example.test", Phone: "11999999999",
		},
		Company: models.CompanyPush{ID: companyID, DisplayName: "ACME"},
	}
}

func liveTokenCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cpfHmac []byte) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM onboarding_token t
		JOIN gestao_employee_link e ON e.id = t.gestao_employee_link_id
		WHERE e.cpf_hmac = $1 AND t.used_at IS NULL AND t.revoked_at IS NULL`, cpfHmac).Scan(&n))
	return n
}

func seedPatientWithHmac(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cpfHmac []byte) {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	_, err := pool.Exec(ctx, `
		INSERT INTO patient_account (id, full_name, email, phone, birth_date, password_hash, status)
		VALUES ($1, 'Paciente', $2, '11999999999', '1990-01-01', 'hash', 'PENDING_DAV')`,
		id, id.String()+"@example.test")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO patient_identity (account_id, cpf, cpf_hmac) VALUES ($1, '12345678901', $2)`,
		id, cpfHmac)
	require.NoError(t, err)
}

// Caso 1–2: pessoa nova recebe convite; o re-push é idempotente e não reemite.
func TestRecordContract_NovoDepoisIdempotente(t *testing.T) {
	ctx := context.Background()
	store, pool, notif := newIngestionStore(t)
	cpf := hmac32(0x01)

	r1, err := store.RecordContract(ctx, pushInput(cpf, "C-1", "CO-1", "ativo"))
	require.NoError(t, err)
	assert.Equal(t, "pendente", r1.PersonStatus)
	assert.Equal(t, "ativo", r1.ContractStatus)
	assert.True(t, r1.InviteSent)
	assert.Contains(t, r1.InviteURL, "https://app.test/onboarding/")
	require.NotNil(t, r1.InviteExpiresAt)
	assert.Equal(t, 1, liveTokenCount(t, ctx, pool, cpf))
	assert.Equal(t, 1, notif.count())

	// Re-push do mesmo contrato: idempotente, sem reemitir o convite.
	r2, err := store.RecordContract(ctx, pushInput(cpf, "C-1", "CO-1", "ativo"))
	require.NoError(t, err)
	assert.False(t, r2.InviteSent)
	assert.Empty(t, r2.InviteURL)
	assert.Equal(t, 1, liveTokenCount(t, ctx, pool, cpf), "não pode cunhar um segundo convite vivo")
	assert.Equal(t, 1, notif.count(), "não pode notificar de novo")
}

// A transição ativo -> afastado -> desligado é idempotente e grava ended_at só no fim.
func TestRecordContract_TransicaoDeStatus(t *testing.T) {
	ctx := context.Background()
	store, pool, _ := newIngestionStore(t)
	cpf := hmac32(0x02)

	_, err := store.RecordContract(ctx, pushInput(cpf, "C-1", "CO-1", "ativo"))
	require.NoError(t, err)

	r, err := store.RecordContract(ctx, pushInput(cpf, "C-1", "CO-1", "afastado"))
	require.NoError(t, err)
	assert.Equal(t, "afastado", r.ContractStatus)

	r, err = store.RecordContract(ctx, pushInput(cpf, "C-1", "CO-1", "desligado"))
	require.NoError(t, err)
	assert.Equal(t, "desligado", r.ContractStatus)

	var endedAt *time.Time
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT ended_at FROM gestao_contract WHERE gestao_contract_id = 'C-1'`).Scan(&endedAt))
	assert.NotNil(t, endedAt, "desligado deve gravar ended_at")
}

// cpf_match: quando já existe patient_account para o cpf_hmac, detectamos e NÃO
// mandamos convite de onboarding (defere ao consentimento, fatia futura).
func TestRecordContract_CPFMatchSuprimeConvite(t *testing.T) {
	ctx := context.Background()
	store, pool, notif := newIngestionStore(t)
	cpf := hmac32(0x03)
	seedPatientWithHmac(t, ctx, pool, cpf)

	r, err := store.RecordContract(ctx, pushInput(cpf, "C-1", "CO-1", "ativo"))
	require.NoError(t, err)
	assert.Equal(t, "pendente", r.PersonStatus)
	assert.False(t, r.InviteSent, "quem já tem conta não recebe convite de onboarding")
	assert.Equal(t, 0, liveTokenCount(t, ctx, pool, cpf))
	assert.Equal(t, 0, notif.count())

	var eventType string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT event_type FROM gestao_ingestion_event ORDER BY occurred_at DESC LIMIT 1`).Scan(&eventType))
	assert.Equal(t, "cpf_match_pendente", eventType)
}

// Reenvio: revoga o convite anterior e cunha outro; exatamente um vivo ao fim.
func TestResendInvite_RevogaECunhaNovo(t *testing.T) {
	ctx := context.Background()
	store, pool, _ := newIngestionStore(t)
	cpf := hmac32(0x04)

	_, err := store.RecordContract(ctx, pushInput(cpf, "C-1", "CO-1", "ativo"))
	require.NoError(t, err)

	res, err := store.ResendInvite(ctx, cpf)
	require.NoError(t, err)
	assert.Contains(t, res.InviteURL, "https://app.test/onboarding/")
	assert.False(t, res.ExpiresAt.IsZero())
	assert.Equal(t, 1, liveTokenCount(t, ctx, pool, cpf), "só um convite vivo após o reenvio")

	var total int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT count(*) FROM onboarding_token t
		JOIN gestao_employee_link e ON e.id = t.gestao_employee_link_id
		WHERE e.cpf_hmac = $1`, cpf).Scan(&total))
	assert.Equal(t, 2, total, "o convite anterior foi revogado, não apagado")
}

func TestResendInvite_Desconhecido(t *testing.T) {
	ctx := context.Background()
	store, _, _ := newIngestionStore(t)
	_, err := store.ResendInvite(ctx, hmac32(0x05))
	assert.ErrorIs(t, err, models.ErrEmployeeUnknown)
}

func TestResendInvite_JaTemConta(t *testing.T) {
	ctx := context.Background()
	store, pool, _ := newIngestionStore(t)
	cpf := hmac32(0x06)
	// Primeiro cria a pessoa (pendente), depois "aparece" uma conta com o mesmo CPF.
	_, err := store.RecordContract(ctx, pushInput(cpf, "C-1", "CO-1", "ativo"))
	require.NoError(t, err)
	seedPatientWithHmac(t, ctx, pool, cpf)

	_, err = store.ResendInvite(ctx, cpf)
	assert.ErrorIs(t, err, models.ErrAlreadyHasAccount)
}

// Pushes concorrentes do mesmo contrato: a corrida no ux_token_vivo é absorvida e
// resta exatamente UM convite vivo.
func TestRecordContract_PushConcorrente(t *testing.T) {
	ctx := context.Background()
	store, pool, _ := newIngestionStore(t)
	cpf := hmac32(0x07)

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = store.RecordContract(ctx, pushInput(cpf, "C-1", "CO-1", "ativo"))
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "push concorrente %d falhou", i)
	}
	assert.Equal(t, 1, liveTokenCount(t, ctx, pool, cpf), "no máximo um convite vivo após a corrida")
}
