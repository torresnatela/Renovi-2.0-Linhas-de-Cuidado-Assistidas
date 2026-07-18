package models

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/db/gen"
)

// Finalidades de consentimento conhecidas (LGPD). Só finalidades desta allowlist
// são aceitas — não se grava consentimento para propósito arbitrário.
const ConsentCheckinHumor = "checkin_humor"

var (
	// ErrConsentInvalid: finalidade desconhecida ou versão de termo vazia.
	ErrConsentInvalid = errors.New("consent: dados inválidos")
	// ErrNoActiveConsent: não há consentimento ativo do paciente para a finalidade.
	// É a pré-condição de gravação do check-in (Anexo C, C.5.1).
	ErrNoActiveConsent = errors.New("consent: sem consentimento ativo")
)

// Consent é o consentimento de um paciente para uma finalidade, versionado.
type Consent struct {
	ID          uuid.UUID
	PatientID   uuid.UUID
	Finalidade  string
	VersaoTermo string
	Status      string
	ConcedidoEm time.Time
	RevogadoEm  *time.Time
}

// ConsentStore é a camada de dados + regra do consentimento.
type ConsentStore struct {
	pool *pgxpool.Pool
	q    *gen.Queries
}

func NewConsentStore(pool *pgxpool.Pool) *ConsentStore {
	return &ConsentStore{pool: pool, q: gen.New(pool)}
}

func knownFinalidade(f string) bool {
	return f == ConsentCheckinHumor
}

// Active devolve o consentimento ativo do paciente para a finalidade, ou
// ErrNoActiveConsent se não houver. É o que as capturas consultam antes de gravar.
func (s *ConsentStore) Active(ctx context.Context, patientID uuid.UUID, finalidade string) (Consent, error) {
	finalidade = strings.TrimSpace(finalidade)
	row, err := s.q.GetActiveConsent(ctx, gen.GetActiveConsentParams{PatientID: patientID, Finalidade: finalidade})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Consent{}, ErrNoActiveConsent
		}
		return Consent{}, fmt.Errorf("consultar consentimento: %w", err)
	}
	return toConsent(row), nil
}

// Grant concede consentimento para a finalidade na versão de termo informada.
// Idempotente para o MESMO termo: havendo um ativo com a mesma versão, devolve-o
// inalterado. Havendo ativo de OUTRA versão, revoga-o e cria um novo, numa
// transação — o índice ux_consent_ativo garante um só ativo por (paciente, finalidade).
func (s *ConsentStore) Grant(ctx context.Context, patientID uuid.UUID, finalidade, versaoTermo string, gestaoContractID *string, now time.Time) (Consent, error) {
	finalidade = strings.TrimSpace(finalidade)
	versaoTermo = strings.TrimSpace(versaoTermo)
	if !knownFinalidade(finalidade) || versaoTermo == "" {
		return Consent{}, ErrConsentInvalid
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Consent{}, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	cur, err := q.GetActiveConsent(ctx, gen.GetActiveConsentParams{PatientID: patientID, Finalidade: finalidade})
	switch {
	case err == nil && cur.VersaoTermo == versaoTermo:
		// Já ativo no mesmo termo: idempotente, não recria nem move concedido_em.
		if err := tx.Commit(ctx); err != nil {
			return Consent{}, fmt.Errorf("commit: %w", err)
		}
		return toConsent(cur), nil
	case err == nil:
		// Ativo em outra versão: revoga para reconceder na nova.
		if _, rerr := q.RevokeActiveConsent(ctx, gen.RevokeActiveConsentParams{
			PatientID: patientID, Finalidade: finalidade,
			RevogadoEm: pgtype.Timestamptz{Time: now, Valid: true},
		}); rerr != nil {
			return Consent{}, fmt.Errorf("revogar anterior: %w", rerr)
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return Consent{}, fmt.Errorf("consultar consentimento: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Consent{}, fmt.Errorf("gerar uuid v7: %w", err)
	}
	row, err := q.InsertConsent(ctx, gen.InsertConsentParams{
		ID: id, PatientID: patientID, GestaoContractID: textPtr(gestaoContractID),
		Finalidade: finalidade, VersaoTermo: versaoTermo,
	})
	if err != nil {
		return Consent{}, fmt.Errorf("inserir consentimento: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Consent{}, fmt.Errorf("commit: %w", err)
	}
	return toConsent(row), nil
}

// Revoke revoga o consentimento ativo da finalidade. Idempotente: sem ativo é
// no-op (o paciente fica sem consentimento do mesmo jeito).
func (s *ConsentStore) Revoke(ctx context.Context, patientID uuid.UUID, finalidade string, now time.Time) error {
	finalidade = strings.TrimSpace(finalidade)
	if !knownFinalidade(finalidade) {
		return ErrConsentInvalid
	}
	if _, err := s.q.RevokeActiveConsent(ctx, gen.RevokeActiveConsentParams{
		PatientID: patientID, Finalidade: finalidade,
		RevogadoEm: pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return fmt.Errorf("revogar consentimento: %w", err)
	}
	return nil
}

func toConsent(row gen.Consent) Consent {
	return Consent{
		ID:          row.ID,
		PatientID:   row.PatientID,
		Finalidade:  row.Finalidade,
		VersaoTermo: row.VersaoTermo,
		Status:      row.Status,
		ConcedidoEm: row.ConcedidoEm,
		RevogadoEm:  timestamptzPtr(row.RevogadoEm),
	}
}
