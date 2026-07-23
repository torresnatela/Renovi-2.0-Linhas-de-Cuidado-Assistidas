//go:build integration

// Testes de INTEGRAÇÃO das travas do schema da ingestão da Gestão (0016).
//
// Não exercitam as queries sqlc: usam SQL cru para provar que os índices parciais,
// os CHECKs e o privilégio append-only recusam estados inválidos por conta própria,
// independentemente do model. Rode com `make test-integration`.
package db_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/testsupport"
)

// hmac32 devolve um cpf_hmac/token_hash fake de 32 bytes (o tamanho que os CHECKs
// octet_length exigem), preenchido com um byte distinto por caso.
func hmac32(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

func seedCompanyLink(t *testing.T, ctx context.Context, conn *pgx.Conn) uuid.UUID {
	t.Helper()
	id := mustV7(t)
	_, err := conn.Exec(ctx, `
		INSERT INTO gestao_company_link (id, gestao_company_id, display_name)
		VALUES ($1, $2, 'ACME')`, id, id.String())
	require.NoError(t, err, "seed gestao_company_link")
	return id
}

func insertEmployeeLink(ctx context.Context, conn *pgx.Conn, id uuid.UUID, cpfHmac []byte, status string) error {
	_, err := conn.Exec(ctx, `
		INSERT INTO gestao_employee_link (id, cpf_hmac, invite_name, status)
		VALUES ($1, $2, 'Fulano de Teste', $3)`, id, cpfHmac, status)
	return err
}

// TestGestaoEmployeeAtivoRecusaSegundaViva: ux_gestao_employee_ativo impede duas
// pessoas VIVAS (não canceladas) com o mesmo cpf_hmac; uma cancelada não conta.
func TestGestaoEmployeeAtivoRecusaSegundaViva(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, testsupport.StartPostgres(t))
	cpf := hmac32(0x11)

	require.NoError(t, insertEmployeeLink(ctx, conn, mustV7(t), cpf, "pendente"),
		"primeira pessoa pendente deve entrar")

	err := insertEmployeeLink(ctx, conn, mustV7(t), cpf, "pendente")
	require.Equal(t, sqlstateUniqueViolation, pgCode(t, err),
		"segunda pessoa viva com o mesmo cpf_hmac deve violar ux_gestao_employee_ativo")

	// Uma pessoa CANCELADA com o mesmo cpf_hmac não ocupa a linha: re-onboarding vale.
	require.NoError(t, insertEmployeeLink(ctx, conn, mustV7(t), hmac32(0x12), "cancelado"),
		"cancelada não conta para a trava")
	require.NoError(t, insertEmployeeLink(ctx, conn, mustV7(t), hmac32(0x12), "pendente"),
		"nova pendente pode nascer quando a anterior foi cancelada")
}

// TestGestaoEmployeeVinculadoCompleto: o CHECK vinculado_completo recusa 'vinculado'
// sem patient_id/link_method/linked_at.
func TestGestaoEmployeeVinculadoCompleto(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, testsupport.StartPostgres(t))

	_, err := conn.Exec(ctx, `
		INSERT INTO gestao_employee_link (id, cpf_hmac, invite_name, status)
		VALUES ($1, $2, 'Fulano', 'vinculado')`, mustV7(t), hmac32(0x21))
	require.Equal(t, sqlstateCheckViolation, pgCode(t, err),
		"'vinculado' sem paciente/método/data deve violar vinculado_completo")
}

// TestGestaoEmployeeCPFHmacTamanho: o CHECK octet_length recusa cpf_hmac != 32 bytes.
func TestGestaoEmployeeCPFHmacTamanho(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, testsupport.StartPostgres(t))

	err := insertEmployeeLink(ctx, conn, mustV7(t), []byte("curto"), "pendente")
	require.Equal(t, sqlstateCheckViolation, pgCode(t, err),
		"cpf_hmac com menos de 32 bytes deve violar o CHECK octet_length")
}

// TestGestaoContractUnico: gestao_contract_id é único (idempotência do push).
func TestGestaoContractUnico(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, testsupport.StartPostgres(t))

	companyID := seedCompanyLink(t, ctx, conn)
	empID := mustV7(t)
	require.NoError(t, insertEmployeeLink(ctx, conn, empID, hmac32(0x31), "pendente"))

	insertContract := func(id uuid.UUID, gestaoContractID, status string, ended *time.Time) error {
		_, err := conn.Exec(ctx, `
			INSERT INTO gestao_contract
				(id, gestao_contract_id, gestao_employee_id, gestao_employee_link_id,
				 gestao_company_link_id, status, started_at, ended_at)
			VALUES ($1, $2, 'E-1', $3, $4, $5, now(), $6)`,
			id, gestaoContractID, empID, companyID, status, ended)
		return err
	}

	require.NoError(t, insertContract(mustV7(t), "C-1", "ativo", nil), "primeiro contrato deve entrar")
	err := insertContract(mustV7(t), "C-1", "ativo", nil)
	require.Equal(t, sqlstateUniqueViolation, pgCode(t, err),
		"gestao_contract_id repetido deve violar a unicidade")

	// desligado_exige_data: desligar sem ended_at é estado incoerente e o banco recusa.
	err = insertContract(mustV7(t), "C-2", "desligado", nil)
	require.Equal(t, sqlstateCheckViolation, pgCode(t, err),
		"'desligado' sem ended_at deve violar desligado_exige_data")

	now := time.Now()
	require.NoError(t, insertContract(mustV7(t), "C-3", "desligado", &now),
		"'desligado' com ended_at deve entrar")
}

// TestOnboardingTokenVivoUnico: ux_token_vivo impede dois convites vivos por pessoa;
// um revogado não conta.
func TestOnboardingTokenVivoUnico(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, testsupport.StartPostgres(t))

	empID := mustV7(t)
	require.NoError(t, insertEmployeeLink(ctx, conn, empID, hmac32(0x41), "pendente"))

	insertToken := func(id uuid.UUID, tokenHash []byte, revoked *time.Time) error {
		_, err := conn.Exec(ctx, `
			INSERT INTO onboarding_token (id, gestao_employee_link_id, token_hash, expires_at, revoked_at)
			VALUES ($1, $2, $3, now() + interval '7 days', $4)`,
			id, empID, tokenHash, revoked)
		return err
	}

	require.NoError(t, insertToken(mustV7(t), hmac32(0xA1), nil), "primeiro convite vivo deve entrar")

	err := insertToken(mustV7(t), hmac32(0xA2), nil)
	require.Equal(t, sqlstateUniqueViolation, pgCode(t, err),
		"segundo convite vivo da mesma pessoa deve violar ux_token_vivo")

	// Revogar o vivo libera a cunhagem de um novo (fluxo do reenvio).
	_, err = conn.Exec(ctx, `UPDATE onboarding_token SET revoked_at = now() WHERE gestao_employee_link_id = $1`, empID)
	require.NoError(t, err)
	require.NoError(t, insertToken(mustV7(t), hmac32(0xA3), nil),
		"com o anterior revogado, um novo convite vivo pode nascer")
}

// TestGestaoIngestionEventAppendOnly: como renovi_app, INSERT/SELECT funcionam;
// UPDATE/DELETE são recusados pelo privilégio de banco (append-only, ADR-024).
func TestGestaoIngestionEventAppendOnly(t *testing.T) {
	ctx := context.Background()
	superDSN, appDSN := testsupport.StartPostgresDSNs(t)
	_ = connect(t, superDSN) // aplica migrations como owner (efeito colateral do Start)

	app := connect(t, appDSN)
	eventID := mustV7(t)
	_, err := app.Exec(ctx, `
		INSERT INTO gestao_ingestion_event (id, event_type, cpf_hmac)
		VALUES ($1, 'contrato_recebido', $2)`, eventID, hmac32(0x51))
	require.NoError(t, err, "renovi_app deve poder INSERIR no log de ingestão")

	var count int
	require.NoError(t,
		app.QueryRow(ctx, `SELECT count(*) FROM gestao_ingestion_event WHERE id = $1`, eventID).Scan(&count))
	require.Equal(t, 1, count)

	_, err = app.Exec(ctx, `UPDATE gestao_ingestion_event SET event_type = 'convite_emitido' WHERE id = $1`, eventID)
	require.Equal(t, sqlstateInsufficientPrivilege, pgCode(t, err),
		"UPDATE no log de ingestão deve ser recusado (append-only)")

	_, err = app.Exec(ctx, `DELETE FROM gestao_ingestion_event WHERE id = $1`, eventID)
	require.Equal(t, sqlstateInsufficientPrivilege, pgCode(t, err),
		"DELETE no log de ingestão deve ser recusado (append-only)")
}
