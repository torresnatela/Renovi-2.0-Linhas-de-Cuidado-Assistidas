package models

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/db/gen"
	"github.com/renovisaude/renovi-care/internal/models/cpf"
)

// Erros da conclusão do onboarding. Use errors.Is.
var (
	// ErrTokenNotFound: não há convite para o token apresentado (→ 404).
	ErrTokenNotFound = errors.New("models: convite não encontrado")
	// ErrTokenExpired: o convite passou do prazo (INVITE_TTL) (→ 410).
	ErrTokenExpired = errors.New("models: convite expirado")
	// ErrTokenUsed: o convite já foi consumido (→ 410).
	ErrTokenUsed = errors.New("models: convite já utilizado")
	// ErrTokenRevoked: o convite foi revogado (reenvio ou cancelamento) (→ 410).
	ErrTokenRevoked = errors.New("models: convite revogado")
	// ErrCPFMismatch: o CPF digitado não corresponde ao do convite (→ 400). Sem isto,
	// um convite fecharia o vínculo para uma pessoa diferente da convidada.
	ErrCPFMismatch = errors.New("models: CPF não corresponde ao convite")
	// ErrOnboardingDeclined: a pessoa já recusou este vínculo (→ 409).
	ErrOnboardingDeclined = errors.New("models: vínculo recusado")
	// ErrOnboardingAlreadyDone: o vínculo já foi fechado numa conclusão anterior (→ 409).
	ErrOnboardingAlreadyDone = errors.New("models: onboarding já concluído")
)

const (
	statusVinculado = "vinculado"
	statusRecusado  = "recusado"
)

// accountRegistrar é o que a conclusão precisa do cadastro: criar a conta rodando a
// saga completa (validação, política de senha, vínculo DAV, enrollment universal).
// Interface no consumidor (ADR-012): o *AccountStore a satisfaz e o teste injeta um
// fake sem DAV.
type accountRegistrar interface {
	Register(ctx context.Context, in RegisterInput) (Account, error)
}

// OnboardingInfo é o que a página do convite mostra: o snapshot para pré-preencher o
// cadastro e as empresas do convite (para o passo "você faz parte da empresa X?").
type OnboardingInfo struct {
	InviteName  string
	InviteEmail string
	InvitePhone string
	Companies   []string
}

// OnboardingStore conclui (ou recusa) o onboarding a partir do token do convite.
// Orquestra a criação da conta (accounts) e o fechamento do vínculo da Gestão.
type OnboardingStore struct {
	pool     *pgxpool.Pool
	q        *gen.Queries
	pepper   []byte
	accounts accountRegistrar
}

// NewOnboardingStore monta o store. pepper é obrigatório (verifica o CPF do convite);
// accounts é o cadastro que cria a conta.
func NewOnboardingStore(pool *pgxpool.Pool, accounts accountRegistrar, pepper []byte) *OnboardingStore {
	return &OnboardingStore{pool: pool, q: gen.New(pool), pepper: pepper, accounts: accounts}
}

// Info valida o token e devolve o pré-preenchimento + as empresas do convite.
func (s *OnboardingStore) Info(ctx context.Context, rawToken string) (OnboardingInfo, error) {
	now := time.Now().UTC()

	tok, err := s.loadLiveToken(ctx, s.q, rawToken, now)
	if err != nil {
		return OnboardingInfo{}, err
	}
	emp, err := s.q.GetEmployeeLinkByID(ctx, tok.GestaoEmployeeLinkID)
	if err != nil {
		return OnboardingInfo{}, fmt.Errorf("buscar colaborador: %w", err)
	}
	if err := ensurePendente(emp.Status); err != nil {
		return OnboardingInfo{}, err
	}
	companies, err := s.q.ListLiveContractCompaniesByEmployeeLink(ctx, emp.ID)
	if err != nil {
		return OnboardingInfo{}, fmt.Errorf("listar empresas do convite: %w", err)
	}
	return OnboardingInfo{
		InviteName:  emp.InviteName,
		InviteEmail: emp.InviteEmail.String,
		InvitePhone: emp.InvitePhone.String,
		Companies:   companies,
	}, nil
}

// Complete consome o convite: verifica o CPF, cria a conta (saga do cadastro) e fecha
// o vínculo. A criação da conta (lenta, DAV) roda FORA de TX; o fechamento é uma TX
// curta ao final, com retry, para ser idempotente diante de falha transitória.
func (s *OnboardingStore) Complete(ctx context.Context, rawToken string, in RegisterInput) (Account, error) {
	now := time.Now().UTC().Truncate(time.Microsecond)

	// 1. Token vivo + pessoa pendente.
	tok, err := s.loadLiveToken(ctx, s.q, rawToken, now)
	if err != nil {
		return Account{}, err
	}
	emp, err := s.q.GetEmployeeLinkByID(ctx, tok.GestaoEmployeeLinkID)
	if err != nil {
		return Account{}, fmt.Errorf("buscar colaborador: %w", err)
	}
	if err := ensurePendente(emp.Status); err != nil {
		return Account{}, err
	}

	// 2. O CPF digitado tem de ser o do convite (o convite só guarda o cpf_hmac).
	if err := s.verifyCPF(in.CPF, emp.CpfHmac); err != nil {
		return Account{}, err
	}

	// 3. CPF que já tem conta → recusa e manda logar. O aceite por usuário já cadastrado
	// (com consentimento) é fatia futura (casos 4/5); aqui só o caminho do paciente novo.
	exists, err := s.patientExists(ctx, emp.CpfHmac)
	if err != nil {
		return Account{}, err
	}
	if exists {
		return Account{}, ErrAlreadyHasAccount
	}

	// 4. Cria a conta (saga completa). Fora de TX — a DAV é lenta.
	acc, err := s.accounts.Register(ctx, in)
	if err != nil {
		return Account{}, err
	}

	// 5. Fecha o vínculo numa TX curta, com retry: a conta já existe; aqui só selamos o
	// vínculo, o consentimento e o consumo do token. Erros de conflito não se retentam.
	var closeErr error
	for i := 0; i < 3; i++ {
		closeErr = s.closeLink(ctx, emp.ID, acc.ID, tok.TokenHash, emp.CpfHmac, now)
		if closeErr == nil || isOnboardingConflict(closeErr) {
			break
		}
	}
	if closeErr != nil {
		return Account{}, closeErr
	}
	return acc, nil
}

// Decline registra que a pessoa abriu o convite e disse que NÃO faz parte da empresa:
// marca o vínculo como 'recusado' (visível na tabela), revoga o token e audita. Não
// cria conta. Idempotente para uma recusa repetida do mesmo convite.
func (s *OnboardingStore) Decline(ctx context.Context, rawToken string) error {
	now := time.Now().UTC().Truncate(time.Microsecond)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrir transação da recusa: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	tok, err := s.loadLiveToken(ctx, q, rawToken, now)
	if err != nil {
		return err
	}
	emp, err := q.GetEmployeeLinkByID(ctx, tok.GestaoEmployeeLinkID)
	if err != nil {
		return fmt.Errorf("buscar colaborador: %w", err)
	}
	if err := ensurePendente(emp.Status); err != nil {
		if errors.Is(err, ErrOnboardingDeclined) {
			return nil // já recusado: idempotente
		}
		return err
	}

	rows, err := q.MarkEmployeeLinkDeclined(ctx, gen.MarkEmployeeLinkDeclinedParams{ID: emp.ID, UpdatedAt: now})
	if err != nil {
		return fmt.Errorf("registrar recusa: %w", err)
	}
	if rows == 0 {
		return nil // corrida: o status mudou no meio; idempotente
	}
	if err := q.RevokeLiveTokensByLink(ctx, emp.ID); err != nil {
		return fmt.Errorf("revogar convite: %w", err)
	}
	if err := insertIngestionEvent(ctx, q, "onboarding_recusado", pgtype.Text{}, emp.ID, emp.CpfHmac); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// loadLiveToken busca o convite pelo hash do token cru e o classifica: inexistente,
// expirado, usado ou revogado viram erro; um convite vivo volta a linha.
func (s *OnboardingStore) loadLiveToken(ctx context.Context, q *gen.Queries, rawToken string, now time.Time) (gen.OnboardingToken, error) {
	tok, err := q.FindTokenByHash(ctx, hashInviteToken(rawToken))
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.OnboardingToken{}, ErrTokenNotFound
	}
	if err != nil {
		return gen.OnboardingToken{}, fmt.Errorf("buscar convite: %w", err)
	}
	if err := classifyToken(tok, now); err != nil {
		return gen.OnboardingToken{}, err
	}
	return tok, nil
}

// classifyToken decide se o convite está vivo a partir do estado da linha e do relógio.
// Pura: alvo de teste unitário.
func classifyToken(tok gen.OnboardingToken, now time.Time) error {
	switch {
	case tok.UsedAt.Valid:
		return ErrTokenUsed
	case tok.RevokedAt.Valid:
		return ErrTokenRevoked
	case !tok.ExpiresAt.After(now):
		return ErrTokenExpired
	default:
		return nil
	}
}

// ensurePendente traduz o status da pessoa em erro quando o convite não pode mais ser
// concluído: vinculado (já feito), recusado, ou cancelado.
func ensurePendente(status string) error {
	switch status {
	case statusPendente:
		return nil
	case statusVinculado:
		return ErrOnboardingAlreadyDone
	case statusRecusado:
		return ErrOnboardingDeclined
	default: // cancelado ou desconhecido: o convite não vale mais
		return ErrTokenRevoked
	}
}

// verifyCPF confere que o CPF digitado corresponde ao cpf_hmac do convite (comparação
// em tempo constante). CPF malformado conta como não-correspondência.
func (s *OnboardingStore) verifyCPF(rawCPF string, want []byte) error {
	parsed, err := cpf.Parse(rawCPF)
	if err != nil {
		return ErrCPFMismatch
	}
	got, err := parsed.HMAC(s.pepper)
	if err != nil {
		return fmt.Errorf("verificar cpf do convite: %w", err) // pepper ausente: erro de config
	}
	if !hmac.Equal(got, want) {
		return ErrCPFMismatch
	}
	return nil
}

func (s *OnboardingStore) patientExists(ctx context.Context, cpfHmac []byte) (bool, error) {
	_, err := s.q.FindIdentityByCPFHmac(ctx, cpfHmac)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	default:
		return false, fmt.Errorf("detectar paciente por cpf_hmac: %w", err)
	}
}

// closeLink fecha o vínculo numa TX: vínculo -> vinculado, consentimento nos contratos
// vivos, convite consumido, evento. Idempotente: se o vínculo já foi fechado para ESTA
// conta (retry após falha transitória), segue selando token/evento sem erro.
func (s *OnboardingStore) closeLink(ctx context.Context, linkID, patientID uuid.UUID, tokenHash, cpfHmac []byte, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrir transação do fechamento: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	rows, err := q.CloseEmployeeLink(ctx, gen.CloseEmployeeLinkParams{
		ID:        linkID,
		PatientID: pgUUID(patientID),
		LinkedAt:  pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("fechar vínculo: %w", err)
	}
	if rows == 0 {
		// Não estava 'pendente'. Se já fechou para ESTA conta, é retry idempotente;
		// senão o vínculo mudou de estado no meio (recusa/cancelamento).
		cur, ferr := q.GetEmployeeLinkByID(ctx, linkID)
		if ferr != nil {
			return fmt.Errorf("reler vínculo: %w", ferr)
		}
		closedToUs := cur.Status == statusVinculado && cur.PatientID.Valid &&
			uuid.UUID(cur.PatientID.Bytes) == patientID
		if !closedToUs {
			if perr := ensurePendente(cur.Status); perr != nil {
				return perr
			}
			return fmt.Errorf("fechamento do vínculo não afetou linhas")
		}
	}

	if _, err := q.SetLiveContractsAcceptedByEmployeeLink(ctx, gen.SetLiveContractsAcceptedByEmployeeLinkParams{
		GestaoEmployeeLinkID: linkID,
		AcceptedAt:           pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return fmt.Errorf("marcar consentimento: %w", err)
	}
	if _, err := q.MarkTokenUsed(ctx, gen.MarkTokenUsedParams{
		TokenHash: tokenHash,
		UsedAt:    pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return fmt.Errorf("consumir convite: %w", err)
	}
	if err := insertIngestionEvent(ctx, q, "onboarding_concluido", pgtype.Text{}, linkID, cpfHmac); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// isOnboardingConflict marca os erros de negócio que NÃO devem ser retentados no
// fechamento (o retry só cobre falha transitória de infra).
func isOnboardingConflict(err error) bool {
	return errors.Is(err, ErrOnboardingDeclined) ||
		errors.Is(err, ErrTokenRevoked) ||
		errors.Is(err, ErrOnboardingAlreadyDone)
}
