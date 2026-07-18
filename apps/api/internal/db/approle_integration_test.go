//go:build integration

// Teste de INTEGRAÇÃO do role restrito renovi_app (migration 0008).
//
// Prova que o append-only de journey_event é imposto pelo BANCO, não por
// disciplina de código: conectado como renovi_app, um UPDATE ou DELETE na tabela é
// recusado com SQLSTATE 42501 (insufficient_privilege). Asseramos o CÓDIGO, não a
// mensagem. Rode com `make test-integration`.
package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/testsupport"
)

const sqlstateInsufficientPrivilege = "42501"

// TestAppRoleJourneyEventAppendOnly: como renovi_app, INSERT e SELECT em
// journey_event funcionam; UPDATE e DELETE são recusados pelo privilégio de banco.
func TestAppRoleJourneyEventAppendOnly(t *testing.T) {
	ctx := context.Background()
	superDSN, appDSN := testsupport.StartPostgresDSNs(t)

	// O seed roda como owner (super): cria as linhas de que o FK precisa.
	super := connect(t, superDSN)
	patientID := seedPatient(t, ctx, super)
	careLineID, code := seedCareLine(t, ctx, super)
	enrollmentID := mustV7(t)
	require.NoError(t, insertEnrollment(ctx, super, enrollmentID, patientID, careLineID, code, "ativa"))

	// A partir daqui, tudo como renovi_app.
	app := connect(t, appDSN)

	// INSERT: permitido.
	eventID := mustV7(t)
	_, err := app.Exec(ctx, `
		INSERT INTO journey_event (id, enrollment_id, patient_id, event_type, actor)
		VALUES ($1, $2, $3, 'matricula_criada', 'sistema')`,
		eventID, enrollmentID, patientID)
	require.NoError(t, err, "renovi_app deve poder INSERIR em journey_event")

	// SELECT: permitido (append-only = insert + select).
	var count int
	require.NoError(t,
		app.QueryRow(ctx, `SELECT count(*) FROM journey_event WHERE id = $1`, eventID).Scan(&count),
		"renovi_app deve poder LER journey_event")
	require.Equal(t, 1, count)

	// UPDATE: recusado no banco.
	_, err = app.Exec(ctx, `UPDATE journey_event SET actor = 'admin' WHERE id = $1`, eventID)
	require.Equal(t, sqlstateInsufficientPrivilege, pgCode(t, err),
		"UPDATE em journey_event deve ser recusado para renovi_app (append-only)")

	// DELETE: recusado no banco.
	_, err = app.Exec(ctx, `DELETE FROM journey_event WHERE id = $1`, eventID)
	require.Equal(t, sqlstateInsufficientPrivilege, pgCode(t, err),
		"DELETE em journey_event deve ser recusado para renovi_app (append-only)")

	// SELECT em care_line: permitido (privilégio herdado do GRANT em todas as tabelas).
	require.NoError(t,
		app.QueryRow(ctx, `SELECT count(*) FROM care_line WHERE id = $1`, careLineID).Scan(&count),
		"renovi_app deve poder LER care_line")
	require.Equal(t, 1, count)
}
