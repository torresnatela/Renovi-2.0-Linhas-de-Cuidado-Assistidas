//go:build integration

// Testes de INTEGRAÇÃO das travas de schema abertas pela 0017 (conclusão do
// onboarding): o novo status 'recusado' da pessoa e os novos tipos de evento
// 'onboarding_concluido'/'onboarding_recusado'. SQL cru, como o trap da 0016.
// Rode com `make test-integration`.
package db_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/testsupport"
)

// TestGestaoEmployeeRecusadoAceito: a 0017 admite status='recusado' (a pessoa abriu
// o convite e disse que não faz parte da empresa); um status fora do conjunto ainda
// é recusado pelo CHECK.
func TestGestaoEmployeeRecusadoAceito(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, testsupport.StartPostgres(t))

	require.NoError(t, insertEmployeeLink(ctx, conn, mustV7(t), hmac32(0x61), "recusado"),
		"status 'recusado' deve ser aceito após a 0017")

	// 'recusado' <> 'cancelado': continua ocupando ux_gestao_employee_ativo.
	err := insertEmployeeLink(ctx, conn, mustV7(t), hmac32(0x61), "pendente")
	require.Equal(t, sqlstateUniqueViolation, pgCode(t, err),
		"uma pessoa recusada ainda conta para a trava (não libera novo convite)")

	err = insertEmployeeLink(ctx, conn, mustV7(t), hmac32(0x62), "sei-la")
	require.Equal(t, sqlstateCheckViolation, pgCode(t, err),
		"status fora do conjunto deve violar o CHECK")
}

// TestGestaoIngestionEventOnboardingTipos: a 0017 admite os eventos de conclusão e
// recusa do onboarding; um tipo desconhecido continua recusado.
func TestGestaoIngestionEventOnboardingTipos(t *testing.T) {
	ctx := context.Background()
	conn := connect(t, testsupport.StartPostgres(t))

	for _, tipo := range []string{"onboarding_concluido", "onboarding_recusado"} {
		_, err := conn.Exec(ctx, `
			INSERT INTO gestao_ingestion_event (id, event_type, cpf_hmac)
			VALUES ($1, $2, $3)`, mustV7(t), tipo, hmac32(0x63))
		require.NoErrorf(t, err, "event_type %q deve ser aceito após a 0017", tipo)
	}

	_, err := conn.Exec(ctx, `
		INSERT INTO gestao_ingestion_event (id, event_type, cpf_hmac)
		VALUES ($1, 'tipo_inexistente', $2)`, mustV7(t), hmac32(0x64))
	require.Equal(t, sqlstateCheckViolation, pgCode(t, err),
		"event_type desconhecido deve violar o CHECK")
}
