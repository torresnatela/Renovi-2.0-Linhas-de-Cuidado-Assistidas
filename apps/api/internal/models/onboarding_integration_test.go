//go:build integration

// Testes de INTEGRAÇÃO da conclusão do onboarding contra um Postgres real (0016+0017).
// Cobrem a orquestração do fechamento do vínculo (token -> conta -> vinculado +
// accepted_at + token usado), a verificação de CPF por HMAC, o cpf_match e a recusa —
// o que um fake não emula fielmente. A criação da conta é um fake (sem DAV). Rode com
// `make test-integration`.
package models_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/models/cpf"
	"github.com/renovisaude/renovi-care/internal/testsupport"
)

const (
	testPepper = "pepper-de-teste-onboarding"
	// CPF válido (DV conferidos); é o convidado no caminho feliz.
	validCPF = "11144477735"
	// Outro CPF válido, para o caso de não-correspondência.
	otherCPF = "52998224725"
)

// fakeRegistrar faz o papel do AccountStore no teste: cria a conta (uma linha em
// patient_account, sem DAV) e devolve o Account. Um err forçado simula falha do cadastro.
type fakeRegistrar struct {
	pool *pgxpool.Pool
	err  error
}

func (f *fakeRegistrar) Register(ctx context.Context, in models.RegisterInput) (models.Account, error) {
	if f.err != nil {
		return models.Account{}, f.err
	}
	id := uuid.Must(uuid.NewV7())
	_, err := f.pool.Exec(ctx, `
		INSERT INTO patient_account (id, full_name, email, phone, birth_date, password_hash, status)
		VALUES ($1, $2, $3, $4, $5, 'hash', 'PENDING_DAV')`,
		id, in.FullName, in.Email, in.Phone, in.BirthDate)
	if err != nil {
		return models.Account{}, err
	}
	return models.Account{ID: id, FullName: in.FullName, Email: in.Email}, nil
}

func newOnboardingSetup(t *testing.T) (*models.GestaoIngestionStore, *models.OnboardingStore, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testsupport.StartPostgres(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	ing := models.NewGestaoIngestionStore(pool, &fakeNotifier{}, 7*24*time.Hour, "https://app.test")
	onb := models.NewOnboardingStore(pool, &fakeRegistrar{pool: pool}, []byte(testPepper))
	return ing, onb, pool
}

// cpfHmacFor devolve o HMAC do CPF sob o pepper de teste — o que a Gestão mandaria e o
// que o convite guarda.
func cpfHmacFor(t *testing.T, cpfStr string) []byte {
	t.Helper()
	c, err := cpf.Parse(cpfStr)
	require.NoError(t, err)
	h, err := c.HMAC([]byte(testPepper))
	require.NoError(t, err)
	return h
}

// seedPatientCPF insere uma conta com o status dado e com cpf EM CLARO e cpf_hmac
// COERENTES (ambos do mesmo CPF) — diferente do seedPatientWithHmac, que usa um cpf
// dummy com um cpf_hmac arbitrário. É o que a conclusão consulta (FindAccountByCPF).
func seedPatientCPF(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cpfStr, status string) {
	t.Helper()
	c, err := cpf.Parse(cpfStr)
	require.NoError(t, err)
	h, err := c.HMAC([]byte(testPepper))
	require.NoError(t, err)
	id := uuid.Must(uuid.NewV7())
	// Uma conta ACTIVE exige o vínculo DAV (CHECK active_exige_vinculo_dav, 0002); uma
	// PENDING_DAV não o tem. Espelhamos isso para o seed ser um estado que o banco aceita.
	var davPersonID, davOrigin any
	if status == "ACTIVE" {
		davPersonID, davOrigin = id.String(), "CREATED"
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO patient_account (id, full_name, email, phone, birth_date, password_hash, status, dav_person_id, dav_link_origin)
		VALUES ($1, 'Paciente', $2, '11999999999', '1990-01-01', 'hash', $3, $4, $5)`,
		id, id.String()+"@example.test", status, davPersonID, davOrigin)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO patient_identity (account_id, cpf, cpf_hmac) VALUES ($1, $2, $3)`,
		id, c.String(), h)
	require.NoError(t, err)
}

// mintInvite roda a ingestão (que cunha o token) e devolve o token CRU extraído da
// invite_url — o que chegaria na URL /onboarding/<token>.
func mintInvite(t *testing.T, ctx context.Context, ing *models.GestaoIngestionStore, cpfHmac []byte) string {
	t.Helper()
	r, err := ing.RecordContract(ctx, pushInput(cpfHmac, "C-1", "CO-1", "ativo"))
	require.NoError(t, err)
	require.True(t, r.InviteSent)
	raw := r.InviteURL[strings.LastIndex(r.InviteURL, "/")+1:]
	require.NotEmpty(t, raw)
	return raw
}

func onboardingRegisterInput() models.RegisterInput {
	return models.RegisterInput{
		FullName:  "Maria de Teste",
		CPF:       validCPF,
		BirthDate: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
		Email:     "maria.nova@example.test",
		Phone:     "11999999999",
		Password:  "senha-super-secreta",
		Address: models.Address{
			ZipCode: "01311000", Street: "Av Paulista", Number: "1000",
			Neighborhood: "Bela Vista", City: "São Paulo", State: "SP", Country: "BR",
		},
	}
}

// Caminho feliz: o convite fecha o vínculo (vinculado + convite + linked_at), marca o
// consentimento no contrato, consome o token e audita.
func TestOnboardingComplete_Feliz(t *testing.T) {
	ctx := context.Background()
	ing, onb, pool := newOnboardingSetup(t)
	hmacKey := cpfHmacFor(t, validCPF)
	raw := mintInvite(t, ctx, ing, hmacKey)

	acc, err := onb.Complete(ctx, raw, onboardingRegisterInput())
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, acc.ID)

	var status, linkMethod string
	var patientID *string
	var linkedAt *time.Time
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT status, link_method, patient_id::text, linked_at
		FROM gestao_employee_link WHERE cpf_hmac = $1`, hmacKey).
		Scan(&status, &linkMethod, &patientID, &linkedAt))
	assert.Equal(t, "vinculado", status)
	assert.Equal(t, "convite", linkMethod)
	require.NotNil(t, patientID)
	assert.Equal(t, acc.ID.String(), *patientID)
	assert.NotNil(t, linkedAt)

	var acceptedAt *time.Time
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT accepted_at FROM gestao_contract WHERE gestao_contract_id = 'C-1'`).Scan(&acceptedAt))
	assert.NotNil(t, acceptedAt, "o contrato deve receber accepted_at")

	assert.Equal(t, 0, liveTokenCount(t, ctx, pool, hmacKey), "o convite foi consumido")

	var eventType string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT event_type FROM gestao_ingestion_event ORDER BY occurred_at DESC LIMIT 1`).Scan(&eventType))
	assert.Equal(t, "onboarding_concluido", eventType)
}

// CPF digitado diferente do convite: recusa, sem criar conta nem tocar o vínculo.
func TestOnboardingComplete_CPFMismatch(t *testing.T) {
	ctx := context.Background()
	ing, onb, pool := newOnboardingSetup(t)
	hmacKey := cpfHmacFor(t, validCPF)
	raw := mintInvite(t, ctx, ing, hmacKey)

	in := onboardingRegisterInput()
	in.CPF = otherCPF
	_, err := onb.Complete(ctx, raw, in)
	assert.ErrorIs(t, err, models.ErrCPFMismatch)

	var status string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM gestao_employee_link WHERE cpf_hmac = $1`, hmacKey).Scan(&status))
	assert.Equal(t, "pendente", status, "o vínculo não pode mudar")
	assert.Equal(t, 1, liveTokenCount(t, ctx, pool, hmacKey), "o convite continua vivo")

	var accounts int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM patient_account`).Scan(&accounts))
	assert.Equal(t, 0, accounts, "nenhuma conta pode ser criada")
}

// CPF que já tem conta ATIVA: recusa (409) e defere ao aceite logado (fatia futura).
func TestOnboardingComplete_JaTemConta(t *testing.T) {
	ctx := context.Background()
	ing, onb, pool := newOnboardingSetup(t)
	hmacKey := cpfHmacFor(t, validCPF)
	raw := mintInvite(t, ctx, ing, hmacKey)
	seedPatientCPF(t, ctx, pool, validCPF, "ACTIVE")

	_, err := onb.Complete(ctx, raw, onboardingRegisterInput())
	assert.ErrorIs(t, err, models.ErrAlreadyHasAccount)
	assert.Equal(t, 1, liveTokenCount(t, ctx, pool, hmacKey), "o convite não é consumido")
}

// Retentativa após a DAV falhar no meio da conclusão: a tentativa anterior deixou um
// stub PENDING_DAV (o reserve grava conta+identidade ANTES de falar com a DAV, então a
// linha persiste mesmo quando a DAV não confirma). Um PENDING_DAV NÃO é "já tem conta"
// — a conclusão deve seguir e fechar o vínculo, não travar em 409 e trancar o convidado.
func TestOnboardingComplete_StubPendingDAVNaoTranca(t *testing.T) {
	ctx := context.Background()
	ing, onb, pool := newOnboardingSetup(t)
	hmacKey := cpfHmacFor(t, validCPF)
	raw := mintInvite(t, ctx, ing, hmacKey)
	seedPatientCPF(t, ctx, pool, validCPF, "PENDING_DAV") // stub da tentativa que a DAV não confirmou

	acc, err := onb.Complete(ctx, raw, onboardingRegisterInput())
	require.NoError(t, err, "um stub PENDING_DAV não pode trancar a conclusão")
	require.NotEqual(t, uuid.Nil, acc.ID)

	var status string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM gestao_employee_link WHERE cpf_hmac = $1`, hmacKey).Scan(&status))
	assert.Equal(t, "vinculado", status, "o vínculo fecha na retentativa")
	assert.Equal(t, 0, liveTokenCount(t, ctx, pool, hmacKey), "o convite é consumido")
}

func TestOnboardingComplete_TokenExpirado(t *testing.T) {
	ctx := context.Background()
	ing, onb, pool := newOnboardingSetup(t)
	hmacKey := cpfHmacFor(t, validCPF)
	raw := mintInvite(t, ctx, ing, hmacKey)

	_, err := pool.Exec(ctx, `UPDATE onboarding_token SET expires_at = now() - interval '1 hour'`)
	require.NoError(t, err)

	_, err = onb.Complete(ctx, raw, onboardingRegisterInput())
	assert.ErrorIs(t, err, models.ErrTokenExpired)
}

// Dupla conclusão: a segunda vê o token já consumido.
func TestOnboardingComplete_TokenUsadoNaSegunda(t *testing.T) {
	ctx := context.Background()
	ing, onb, _ := newOnboardingSetup(t)
	raw := mintInvite(t, ctx, ing, cpfHmacFor(t, validCPF))

	_, err := onb.Complete(ctx, raw, onboardingRegisterInput())
	require.NoError(t, err)

	_, err = onb.Complete(ctx, raw, onboardingRegisterInput())
	assert.ErrorIs(t, err, models.ErrTokenUsed)
}

func TestOnboardingInfo_Feliz(t *testing.T) {
	ctx := context.Background()
	ing, onb, _ := newOnboardingSetup(t)
	raw := mintInvite(t, ctx, ing, cpfHmacFor(t, validCPF))

	info, err := onb.Info(ctx, raw)
	require.NoError(t, err)
	assert.Equal(t, "Maria de Teste", info.InviteName)
	assert.Equal(t, "maria@example.test", info.InviteEmail)
	assert.Equal(t, "11999999999", info.InvitePhone)
	assert.Equal(t, []string{"ACME"}, info.Companies)
}

func TestOnboardingInfo_TokenInexistente(t *testing.T) {
	ctx := context.Background()
	_, onb, _ := newOnboardingSetup(t)
	_, err := onb.Info(ctx, "token-que-nao-existe")
	assert.ErrorIs(t, err, models.ErrTokenNotFound)
}

// Recusa: marca 'recusado', revoga o convite, audita; depois o convite não vale mais.
func TestOnboardingDecline(t *testing.T) {
	ctx := context.Background()
	ing, onb, pool := newOnboardingSetup(t)
	hmacKey := cpfHmacFor(t, validCPF)
	raw := mintInvite(t, ctx, ing, hmacKey)

	require.NoError(t, onb.Decline(ctx, raw))

	var status string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT status FROM gestao_employee_link WHERE cpf_hmac = $1`, hmacKey).Scan(&status))
	assert.Equal(t, "recusado", status)
	assert.Equal(t, 0, liveTokenCount(t, ctx, pool, hmacKey), "o convite foi revogado")

	var eventType string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT event_type FROM gestao_ingestion_event ORDER BY occurred_at DESC LIMIT 1`).Scan(&eventType))
	assert.Equal(t, "onboarding_recusado", eventType)

	// O convite não pode mais ser usado nem visto.
	_, err := onb.Info(ctx, raw)
	assert.ErrorIs(t, err, models.ErrTokenRevoked)
}
