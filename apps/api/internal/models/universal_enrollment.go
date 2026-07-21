package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/renovisaude/renovi-care/internal/db/gen"
	"github.com/renovisaude/renovi-care/internal/models/careline"
)

// UniversalMentalHealthCode é o code da linha de cuidado ABERTA — a que carrega o
// Verificador de Humor (GRID/WHO-5/PHQ-4) para TODO colaborador, com ou sem plano
// (Degrau 1, ADR-040). Semeada em 0015_universal_mental_health e excluída da listagem
// da jornada (care_journey_repo.go) para NÃO contar como "plano" no perfil.
const UniversalMentalHealthCode = "saude-mental-aberta"

// universalValidUntil é a vigência (perpétua) da matrícula universal. Data-sentinela
// distante — não 'infinity' (pgx v5 não faz scan de infinity em time.Time, e o
// valid_until entra no motor puro). Com ela o motor `vigenciaBlock` nunca bloqueia e a
// expiração lazy nunca dispara.
var universalValidUntil = time.Date(2999, 12, 31, 0, 0, 0, 0, time.UTC)

// insertUniversalEnrollment matricula o paciente na linha universal, se ainda não
// estiver, DENTRO da transação do chamador (recebe o querier `q`). É idempotente e
// FAIL-OPEN: se a linha universal ainda não foi semeada, não faz nada (nunca derruba
// o cadastro por causa de um seed ausente — o backfill/uma ativação futura cobre).
//
// Função de pacote (não método de EnrollmentStore) de propósito: precisa ser chamável
// da TX de ativação do AccountStore sem acoplar os dois stores. Reusa as queries
// geradas já existentes — a checagem de vivacidade usa ListEnrollmentsByPatient em vez
// de uma query nova, então uma unique_violation nunca chega a envenenar a TX.
func insertUniversalEnrollment(ctx context.Context, q *gen.Queries, patientID uuid.UUID, now time.Time) error {
	line, err := q.GetLatestPublishedCareLine(ctx, UniversalMentalHealthCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // fail-open: linha universal ainda não semeada
		}
		return fmt.Errorf("carregar linha universal: %w", err)
	}

	// Idempotência: se já há matrícula viva na linha universal, não cria outra (a trava
	// ux_enrollment_viva também impede, mas checar antes evita a unique_violation).
	existing, err := q.ListEnrollmentsByPatient(ctx, patientID)
	if err != nil {
		return fmt.Errorf("listar matrículas: %w", err)
	}
	for _, e := range existing {
		if e.CareLineCode == UniversalMentalHealthCode &&
			(e.Status == careline.EnrollmentAtiva || e.Status == careline.EnrollmentPausada) {
			return nil
		}
	}

	enrollmentID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("gerar uuid v7 da matrícula: %w", err)
	}
	if _, err := q.InsertEnrollment(ctx, gen.InsertEnrollmentParams{
		ID: enrollmentID, PatientID: patientID, CareLineID: line.ID,
		CareLineCode: UniversalMentalHealthCode, Status: careline.EnrollmentAtiva,
		ValidFrom: now, ValidUntil: universalValidUntil, GestaoContractID: pgtype.Text{},
	}); err != nil {
		return fmt.Errorf("inserir matrícula universal: %w", err)
	}

	periodID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("gerar uuid v7 do período: %w", err)
	}
	if _, err := q.InsertEnrollmentPeriod(ctx, gen.InsertEnrollmentPeriodParams{
		ID: periodID, EnrollmentID: enrollmentID, StartsAt: now, EndsAt: universalValidUntil,
	}); err != nil {
		return fmt.Errorf("inserir período universal: %w", err)
	}

	payload, err := json.Marshal(enrollCreatedPayload{
		Months: 0, ValidFrom: now, ValidUntil: universalValidUntil,
		CareLineID: line.ID, CareLineVersion: line.Version, PeriodID: periodID,
	})
	if err != nil {
		return fmt.Errorf("montar payload: %w", err)
	}
	eventID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("gerar uuid v7 do evento: %w", err)
	}
	if _, err := q.InsertJourneyEvent(ctx, gen.InsertJourneyEventParams{
		ID: eventID, EnrollmentID: enrollmentID, PatientID: patientID,
		EventType: "matricula_criada", Actor: actorSistema,
		RefTable: pgtype.Text{String: refEnrollment, Valid: true},
		RefID:    pgUUID(enrollmentID), Payload: payload,
	}); err != nil {
		return fmt.Errorf("gravar evento matricula_criada: %w", err)
	}
	return nil
}
