package models

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/adapters/notify"
	"github.com/renovisaude/renovi-care/internal/db/gen"
)

// Erros da ingestão da Gestão. Use errors.Is.
var (
	// ErrInvalidContractPush: o push do contrato não passou na validação (cpf_hmac
	// com tamanho errado, status fora do conjunto, id/nome vazios).
	ErrInvalidContractPush = errors.New("models: contrato da Gestão inválido")
	// ErrEmployeeUnknown: reenvio para um cpf_hmac que não temos (→ 404).
	ErrEmployeeUnknown = errors.New("models: colaborador desconhecido")
	// ErrAlreadyHasAccount: reenvio para quem já tem patient_account. O convite de
	// onboarding cria conta; quem já tem entra pelo fluxo de consentimento (cpf_match,
	// fatia futura), não por um novo convite (→ 409).
	ErrAlreadyHasAccount = errors.New("models: colaborador já tem conta")
)

const (
	statusPendente  = "pendente"
	statusDesligado = "desligado"
)

// Notifier entrega o convite de onboarding. A interface vive aqui, no consumidor
// (ADR-012); a implementação (stub log ou e-mail real) vive em adapters/notify.
type Notifier interface {
	SendInvite(ctx context.Context, msg notify.InviteMessage) error
}

// CompanyPush, EmployeePush e ContractPush são o corpo do push da Gestão já
// decodificado pelo controller (cpf_hmac já em bytes).
type CompanyPush struct {
	ID          string
	DisplayName string
}

type EmployeePush struct {
	ID      string
	CPFHmac []byte
	Name    string
	Email   string
	Phone   string
}

type ContractPush struct {
	ContractID string
	Status     string // ativo | afastado | desligado
	StartedAt  time.Time
	EndedAt    *time.Time
	Employee   EmployeePush
	Company    CompanyPush
}

// RecordResult é a resposta do push: o estado da pessoa e do contrato, e — quando
// um convite foi cunhado — a URL para o gestor repassar (WhatsApp) e sua validade.
type RecordResult struct {
	PersonStatus    string
	ContractStatus  string
	InviteSent      bool
	InviteURL       string
	InviteExpiresAt *time.Time
}

// ResendResult é a resposta do reenvio de convite.
type ResendResult struct {
	InviteURL string
	ExpiresAt time.Time
}

// GestaoIngestionStore persiste os contratos vindos da Gestão e cunha os convites.
type GestaoIngestionStore struct {
	pool       *pgxpool.Pool
	q          *gen.Queries
	notifier   Notifier
	inviteTTL  time.Duration
	webBaseURL string
}

// NewGestaoIngestionStore monta o store.
func NewGestaoIngestionStore(pool *pgxpool.Pool, notifier Notifier, inviteTTL time.Duration, webBaseURL string) *GestaoIngestionStore {
	return &GestaoIngestionStore{
		pool: pool, q: gen.New(pool), notifier: notifier,
		inviteTTL: inviteTTL, webBaseURL: webBaseURL,
	}
}

// RecordContract recebe um contrato (push idempotente por contract_id): upsert de
// empresa/pessoa/contrato, decide o convite e o cunha, tudo numa TX curta. A
// entrega (Notifier) roda FORA da TX, como a DAV no cadastro.
func (s *GestaoIngestionStore) RecordContract(ctx context.Context, in ContractPush) (RecordResult, error) {
	if err := validateContractPush(in); err != nil {
		return RecordResult{}, err
	}
	now := time.Now().UTC().Truncate(time.Microsecond)

	var result RecordResult
	var minted *mintedInvite

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RecordResult{}, fmt.Errorf("abrir transação da ingestão: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	// 1. Empresa (idempotente por gestao_company_id).
	companyID, err := uuid.NewV7()
	if err != nil {
		return RecordResult{}, fmt.Errorf("gerar uuid v7: %w", err)
	}
	company, err := q.UpsertGestaoCompany(ctx, gen.UpsertGestaoCompanyParams{
		ID: companyID, GestaoCompanyID: in.Company.ID, DisplayName: in.Company.DisplayName,
	})
	if err != nil {
		return RecordResult{}, fmt.Errorf("upsert empresa: %w", err)
	}

	// 2. Detecta paciente existente (só detecção — NÃO vincula nesta fatia).
	patientExists, err := s.patientExists(ctx, q, in.Employee.CPFHmac)
	if err != nil {
		return RecordResult{}, err
	}

	// 3. Pessoa (idempotente por cpf_hmac; o DO UPDATE nunca rebaixa o status).
	empID, err := uuid.NewV7()
	if err != nil {
		return RecordResult{}, fmt.Errorf("gerar uuid v7: %w", err)
	}
	emp, err := q.UpsertGestaoEmployeeLink(ctx, gen.UpsertGestaoEmployeeLinkParams{
		ID: empID, CpfHmac: in.Employee.CPFHmac, InviteName: in.Employee.Name,
		InviteEmail: text(in.Employee.Email), InvitePhone: text(in.Employee.Phone),
	})
	if err != nil {
		return RecordResult{}, fmt.Errorf("upsert pessoa: %w", err)
	}

	// 4. Contrato (idempotente por gestao_contract_id).
	contractID, err := uuid.NewV7()
	if err != nil {
		return RecordResult{}, fmt.Errorf("gerar uuid v7: %w", err)
	}
	startedAt := in.StartedAt
	if startedAt.IsZero() {
		startedAt = now
	}
	contract, err := q.UpsertGestaoContract(ctx, gen.UpsertGestaoContractParams{
		ID: contractID, GestaoContractID: in.ContractID, GestaoEmployeeID: in.Employee.ID,
		GestaoEmployeeLinkID: emp.ID, GestaoCompanyLinkID: company.ID,
		Status: in.Status, StartedAt: startedAt, EndedAt: contractEndedAt(in, now),
	})
	if err != nil {
		return RecordResult{}, fmt.Errorf("upsert contrato: %w", err)
	}

	result.PersonStatus = emp.Status
	result.ContractStatus = contract.Status

	// 5. Decide o convite. Só consulta o token vivo quando ele pode importar.
	liveTokenExists := false
	if emp.Status == statusPendente && !patientExists {
		if _, ferr := q.FindLiveTokenByLink(ctx, emp.ID); ferr == nil {
			liveTokenExists = true
		} else if !errors.Is(ferr, pgx.ErrNoRows) {
			return RecordResult{}, fmt.Errorf("procurar convite vivo: %w", ferr)
		}
	}
	action := decideInvite(emp.Status, patientExists, liveTokenExists)

	if action == actionMint {
		m, raced, mErr := s.mintToken(ctx, tx, emp.ID, now)
		if mErr != nil {
			return RecordResult{}, mErr
		}
		if raced {
			// Corrida perdida no ux_token_vivo: outro push cunhou primeiro. Trata como
			// "já havia convite vivo" — não reemite.
			action = actionSuppressLiveToken
		} else {
			minted = m
		}
	}
	result.InviteSent = minted != nil

	// 6. Trilha append-only.
	if err := insertIngestionEvent(ctx, q, action.eventType(), text(in.ContractID), emp.ID, in.Employee.CPFHmac); err != nil {
		return RecordResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return RecordResult{}, fmt.Errorf("commit da ingestão: %w", err)
	}

	// Fora da TX: entrega best-effort (a URL também volta na resposta).
	if minted != nil {
		result.InviteURL = s.inviteURL(minted.raw)
		exp := minted.expiresAt
		result.InviteExpiresAt = &exp
		_ = s.notifier.SendInvite(ctx, notify.InviteMessage{
			Name: in.Employee.Name, Email: in.Employee.Email, Phone: in.Employee.Phone,
			InviteURL: result.InviteURL, ExpiresAt: exp,
		})
	}
	return result, nil
}

// ResendInvite revoga o convite vivo e cunha outro para um colaborador pendente.
// Recusa quem não conhecemos (404) e quem já tem conta (409, defere ao consentimento).
func (s *GestaoIngestionStore) ResendInvite(ctx context.Context, cpfHmac []byte) (ResendResult, error) {
	if len(cpfHmac) != 32 {
		return ResendResult{}, fmt.Errorf("%w: cpf_hmac deve ter 32 bytes", ErrInvalidContractPush)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ResendResult{}, fmt.Errorf("abrir transação do reenvio: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	// FOR UPDATE serializa reenvios concorrentes: o segundo espera, vê o token do
	// primeiro já revogado e cunha limpo, sem colidir no ux_token_vivo.
	emp, err := q.GetLiveEmployeeLinkByCPFHmacForUpdate(ctx, cpfHmac)
	if errors.Is(err, pgx.ErrNoRows) {
		return ResendResult{}, ErrEmployeeUnknown
	}
	if err != nil {
		return ResendResult{}, fmt.Errorf("buscar colaborador: %w", err)
	}

	exists, err := s.patientExists(ctx, q, cpfHmac)
	if err != nil {
		return ResendResult{}, err
	}
	if exists {
		return ResendResult{}, ErrAlreadyHasAccount
	}

	if err := q.RevokeLiveTokensByLink(ctx, emp.ID); err != nil {
		return ResendResult{}, fmt.Errorf("revogar convite anterior: %w", err)
	}
	m, raced, err := s.mintToken(ctx, tx, emp.ID, now)
	if err != nil {
		return ResendResult{}, err
	}
	if raced {
		// Não deveria ocorrer: revogamos o vivo antes e o FOR UPDATE serializa os
		// reenvios. Se ocorrer, falhar é melhor que devolver uma URL vazia.
		return ResendResult{}, fmt.Errorf("corrida inesperada ao reenviar convite")
	}
	if err := insertIngestionEvent(ctx, q, "convite_reenviado", pgtype.Text{}, emp.ID, cpfHmac); err != nil {
		return ResendResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ResendResult{}, fmt.Errorf("commit do reenvio: %w", err)
	}

	url := s.inviteURL(m.raw)
	_ = s.notifier.SendInvite(ctx, notify.InviteMessage{
		Name: emp.InviteName, Email: emp.InviteEmail.String, Phone: emp.InvitePhone.String,
		InviteURL: url, ExpiresAt: m.expiresAt,
	})
	return ResendResult{InviteURL: url, ExpiresAt: m.expiresAt}, nil
}

// --------------------------------------------------------------------------
// Decisão do convite (pura)
// --------------------------------------------------------------------------

type inviteAction int

const (
	actionMint inviteAction = iota
	actionSuppressLinked
	actionSuppressCPFMatch
	actionSuppressLiveToken
)

func (a inviteAction) inviteSent() bool { return a == actionMint }

func (a inviteAction) eventType() string {
	switch a {
	case actionMint:
		return "convite_emitido"
	case actionSuppressCPFMatch:
		return "cpf_match_pendente"
	default:
		return "contrato_recebido"
	}
}

// decideInvite resolve o que fazer com o convite a partir do estado observado.
// Prioridade: já vinculado > já tem conta (cpf_match) > já tem convite vivo > cunha.
func decideInvite(personStatus string, patientExists, liveTokenExists bool) inviteAction {
	if personStatus != statusPendente {
		return actionSuppressLinked
	}
	if patientExists {
		return actionSuppressCPFMatch
	}
	if liveTokenExists {
		return actionSuppressLiveToken
	}
	return actionMint
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

type mintedInvite struct {
	raw       string
	expiresAt time.Time
}

func (s *GestaoIngestionStore) patientExists(ctx context.Context, q *gen.Queries, cpfHmac []byte) (bool, error) {
	_, err := q.FindIdentityByCPFHmac(ctx, cpfHmac)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	default:
		return false, fmt.Errorf("detectar paciente por cpf_hmac: %w", err)
	}
}

// mintToken cunha um convite dentro de um SAVEPOINT (tx.Begin sobre a TX externa):
// se a inserção bater no ux_token_vivo (corrida com outro push), o savepoint é
// desfeito e a TX externa continua VIVA — devolvendo raced=true em vez de deixar a
// transação abortada (um 23505 solto envenena a TX inteira e derrubaria o commit).
func (s *GestaoIngestionStore) mintToken(ctx context.Context, tx pgx.Tx, linkID uuid.UUID, now time.Time) (minted *mintedInvite, raced bool, err error) {
	raw, hash, err := newInviteToken()
	if err != nil {
		return nil, false, err
	}
	tokenID, err := uuid.NewV7()
	if err != nil {
		return nil, false, fmt.Errorf("gerar uuid v7: %w", err)
	}
	expiresAt := now.Add(s.inviteTTL)

	sp, err := tx.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("abrir savepoint do convite: %w", err)
	}
	if _, err := s.q.WithTx(sp).InsertOnboardingToken(ctx, gen.InsertOnboardingTokenParams{
		ID: tokenID, GestaoEmployeeLinkID: linkID, TokenHash: hash, ExpiresAt: expiresAt,
	}); err != nil {
		_ = sp.Rollback(ctx)
		if isTokenRace(err) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("cunhar convite: %w", err)
	}
	if err := sp.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit do savepoint do convite: %w", err)
	}
	return &mintedInvite{raw: raw, expiresAt: expiresAt}, false, nil
}

func insertIngestionEvent(ctx context.Context, q *gen.Queries, eventType string, contractID pgtype.Text, linkID uuid.UUID, cpfHmac []byte) error {
	eventID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("gerar uuid v7: %w", err)
	}
	if err := q.InsertIngestionEvent(ctx, gen.InsertIngestionEventParams{
		ID: eventID, EventType: eventType, GestaoContractID: contractID,
		GestaoEmployeeLinkID: pgtype.UUID{Bytes: linkID, Valid: true},
		CpfHmac:              cpfHmac, Payload: []byte("{}"),
	}); err != nil {
		return fmt.Errorf("gravar evento de ingestão: %w", err)
	}
	return nil
}

// inviteURL monta a URL de onboarding a partir da base do front e do token cru.
func (s *GestaoIngestionStore) inviteURL(raw string) string {
	return strings.TrimRight(s.webBaseURL, "/") + "/onboarding/" + raw
}

// newInviteToken gera o token de convite: 32 bytes aleatórios em base64url (o
// segredo que vai na URL) e o SHA-256 dele (o que o banco guarda). Mesmo desenho da
// sessão (session.go): o token cru nunca toca o banco.
func newInviteToken() (string, []byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("gerar token de convite: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	return raw, hashInviteToken(raw), nil
}

// hashInviteToken devolve o SHA-256 do token cru — o que o banco guarda. O mesmo
// digest é usado ao cunhar (newInviteToken) e ao consumir o convite (onboarding).
func hashInviteToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// isTokenRace reconhece a violação do ux_token_vivo (dois convites vivos para a
// mesma pessoa) pelo NOME da constraint — como o care_journey faz com a idempotência.
func isTokenRace(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == "ux_token_vivo"
}

// contractEndedAt calcula ended_at: preenchido só em 'desligado' (o CHECK
// desligado_exige_data exige), nulo caso contrário (reativar limpa a data).
func contractEndedAt(in ContractPush, now time.Time) pgtype.Timestamptz {
	if in.Status != statusDesligado {
		return pgtype.Timestamptz{}
	}
	t := now
	if in.EndedAt != nil {
		t = *in.EndedAt
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func validateContractPush(in ContractPush) error {
	if len(in.Employee.CPFHmac) != 32 {
		return fmt.Errorf("%w: cpf_hmac deve ter 32 bytes", ErrInvalidContractPush)
	}
	switch in.Status {
	case "ativo", "afastado", statusDesligado:
	default:
		return fmt.Errorf("%w: status inválido %q", ErrInvalidContractPush, in.Status)
	}
	for campo, v := range map[string]string{
		"contract_id":          in.ContractID,
		"employee.id":          in.Employee.ID,
		"employee.name":        in.Employee.Name,
		"company.id":           in.Company.ID,
		"company.display_name": in.Company.DisplayName,
	} {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%w: %s é obrigatório", ErrInvalidContractPush, campo)
		}
	}
	return nil
}
