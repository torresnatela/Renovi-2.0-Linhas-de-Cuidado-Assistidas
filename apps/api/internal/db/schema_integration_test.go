//go:build integration

// Testes de INTEGRAÇÃO do schema das linhas de cuidado (migrations 0005–0007).
//
// Não exercitam as queries sqlc: usam SQL cru para provar que as TRAVAS DO BANCO
// (índices únicos parciais e CHECKs) recusam estados inválidos por conta própria,
// independentemente da disciplina do model. Rode com `make test-integration`.
package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/testsupport"
)

// SQLSTATEs que asseramos pelo código, nunca pela mensagem (ver ADRs de robustez).
const (
	sqlstateUniqueViolation = "23505" // unique_violation
	sqlstateCheckViolation  = "23514" // check_violation
)

func mustV7(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)
	return id
}

// connect abre uma conexão pgx ao DSN e a fecha ao fim do teste.
func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err, "conectar ao postgres")
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

// pgCode devolve o SQLSTATE de um erro do pgx (ou falha o teste se não for PgError).
func pgCode(t *testing.T, err error) string {
	t.Helper()
	require.Error(t, err)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr, "esperava um pgconn.PgError, veio: %v", err)
	return pgErr.Code
}

// seedPatient insere um patient_account mínimo (status PENDING_DAV — sem vínculo
// DAV, respeitando o CHECK active_exige_vinculo_dav de 0002) e devolve o id.
func seedPatient(t *testing.T, ctx context.Context, conn *pgx.Conn) uuid.UUID {
	t.Helper()
	id := mustV7(t)
	_, err := conn.Exec(ctx, `
		INSERT INTO patient_account (id, full_name, email, phone, birth_date, password_hash, status)
		VALUES ($1, 'Paciente Teste', $2, '11999999999', '1990-01-01', 'argon2id$hash', 'PENDING_DAV')`,
		id, id.String()+"@example.test")
	require.NoError(t, err, "seed patient_account")
	return id
}

// seedCareLine insere uma care_line publicada e devolve (id, code).
func seedCareLine(t *testing.T, ctx context.Context, conn *pgx.Conn) (uuid.UUID, string) {
	t.Helper()
	id := mustV7(t)
	code := "linha_" + id.String()[:8]
	_, err := conn.Exec(ctx, `
		INSERT INTO care_line (id, code, version, name, status, published_at)
		VALUES ($1, $2, 1, 'Linha Teste', 'published', now())`,
		id, code)
	require.NoError(t, err, "seed care_line")
	return id, code
}

// seedCareLineItem insere um item da linha e devolve o id.
func seedCareLineItem(t *testing.T, ctx context.Context, conn *pgx.Conn, careLineID uuid.UUID) uuid.UUID {
	t.Helper()
	id := mustV7(t)
	_, err := conn.Exec(ctx, `
		INSERT INTO care_line_item (id, care_line_id, ref, kind, specialty_code, label)
		VALUES ($1, $2, 'consulta_geral', 'CONSULTA', 'CLINICA', 'Consulta Geral')`,
		id, careLineID)
	require.NoError(t, err, "seed care_line_item")
	return id
}

// insertEnrollment insere uma matrícula com o status dado.
func insertEnrollment(ctx context.Context, conn *pgx.Conn, id, patientID, careLineID uuid.UUID, code, status string) error {
	now := time.Now()
	_, err := conn.Exec(ctx, `
		INSERT INTO enrollment (id, patient_id, care_line_id, care_line_code, status, valid_from, valid_until)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, patientID, careLineID, code, status, now, now.Add(365*24*time.Hour))
	return err
}

// TestEnrollmentVivaRecusaSegundaAtiva: o índice parcial ux_enrollment_viva impede
// duas matrículas VIVAS (ativa/pausada) do mesmo (paciente, code).
func TestEnrollmentVivaRecusaSegundaAtiva(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, testsupport.StartPostgres(t))

	patientID := seedPatient(t, ctx, conn)
	careLineID, code := seedCareLine(t, ctx, conn)

	require.NoError(t, insertEnrollment(ctx, conn, mustV7(t), patientID, careLineID, code, "ativa"),
		"primeira matrícula ativa deve entrar")

	err := insertEnrollment(ctx, conn, mustV7(t), patientID, careLineID, code, "ativa")
	require.Equal(t, sqlstateUniqueViolation, pgCode(t, err),
		"segunda matrícula viva do mesmo (paciente, code) deve violar ux_enrollment_viva")

	// Mas uma matrícula ENCERRADA não ocupa a linha: uma nova ativa pode nascer.
	require.NoError(t, insertEnrollment(ctx, conn, mustV7(t), patientID, careLineID, code, "encerrada"),
		"matrícula encerrada não conta para a trava")
}

// TestCareAppointmentIdempotencyKeyUnica: ux_care_appt_idem impede duas consultas
// com a mesma (enrollment_id, idempotency_key). Os booking_id são distintos de
// propósito, para que a falha seja da chave de idempotência e não do índice de
// booking.
func TestCareAppointmentIdempotencyKeyUnica(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, testsupport.StartPostgres(t))

	patientID := seedPatient(t, ctx, conn)
	careLineID, code := seedCareLine(t, ctx, conn)
	itemID := seedCareLineItem(t, ctx, conn, careLineID)
	enrollmentID := mustV7(t)
	require.NoError(t, insertEnrollment(ctx, conn, enrollmentID, patientID, careLineID, code, "ativa"))

	insertAppt := func(bookingID uuid.UUID, idemKey string) error {
		_, err := conn.Exec(ctx, `
			INSERT INTO care_appointment
				(id, enrollment_id, care_line_item_id, item_ref, booking_id, scheduled_at, status, idempotency_key)
			VALUES ($1, $2, $3, 'consulta_geral', $4, now(), 'agendada', $5)`,
			mustV7(t), enrollmentID, itemID, bookingID, idemKey)
		return err
	}

	require.NoError(t, insertAppt(mustV7(t), "idem-abc"), "primeira consulta deve entrar")

	err := insertAppt(mustV7(t), "idem-abc")
	require.Equal(t, sqlstateUniqueViolation, pgCode(t, err),
		"mesma idempotency_key na mesma matrícula deve violar ux_care_appt_idem")
}

// TestEnrollmentPeriodChecaJanela: periodo_valido recusa ends_at <= starts_at.
func TestEnrollmentPeriodChecaJanela(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, testsupport.StartPostgres(t))

	patientID := seedPatient(t, ctx, conn)
	careLineID, code := seedCareLine(t, ctx, conn)
	enrollmentID := mustV7(t)
	require.NoError(t, insertEnrollment(ctx, conn, enrollmentID, patientID, careLineID, code, "ativa"))

	now := time.Now()
	_, err := conn.Exec(ctx, `
		INSERT INTO enrollment_period (id, enrollment_id, starts_at, ends_at)
		VALUES ($1, $2, $3, $4)`,
		mustV7(t), enrollmentID, now, now) // ends_at == starts_at: inválido
	require.Equal(t, sqlstateCheckViolation, pgCode(t, err),
		"período com ends_at <= starts_at deve violar periodo_valido")
}
