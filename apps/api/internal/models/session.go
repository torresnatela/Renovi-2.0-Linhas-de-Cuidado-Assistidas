package models

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/db/gen"
)

// ErrNoSession: não há sessão viva para este token. Um erro só para token
// inexistente, expirado, revogado e conta bloqueada — quem recebe só precisa
// saber que tem de logar de novo.
var ErrNoSession = errors.New("models: sessão inexistente ou expirada")

// tokenBytes é a entropia do token de sessão. 32 bytes (256 bits) é o mesmo
// tamanho do hash — não adianta ter mais.
const tokenBytes = 32

// SessionStore cuida do ciclo de vida das sessões.
//
// A sessão é OPACA (ADR-010): um valor aleatório sem significado, e o banco
// guarda apenas o SHA-256 dele. Duas consequências que motivaram a escolha
// contra JWT:
//
//   - revogação é instantânea (logout, bloqueio de conta, incidente);
//   - um dump do banco não permite se passar por ninguém, porque o token não
//     está lá — só o hash, e hash não se inverte.
//
// Não há segredo de assinatura para vazar ou rotacionar.
type SessionStore struct {
	q   *gen.Queries
	ttl time.Duration
}

// NewSessionStore monta o store. ttl é a validade de cada sessão.
func NewSessionStore(pool *pgxpool.Pool, ttl time.Duration) *SessionStore {
	return &SessionStore{q: gen.New(pool), ttl: ttl}
}

// Create abre uma sessão e devolve o token em claro.
//
// Esta é a ÚNICA vez que o token existe fora do browser: ele não é recuperável
// depois, nem por nós. Quem chama põe no cookie httpOnly e esquece.
func (s *SessionStore) Create(ctx context.Context, accountID uuid.UUID) (string, time.Time, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, fmt.Errorf("gerar token de sessão: %w", err)
	}
	// base64url: cabe num cookie sem escaping.
	token := base64.RawURLEncoding.EncodeToString(raw)

	id, err := uuid.NewV7()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("gerar uuid v7: %w", err)
	}

	expiresAt := time.Now().Add(s.ttl)
	if err := s.q.InsertSession(ctx, gen.InsertSessionParams{
		ID:        id,
		AccountID: accountID,
		TokenHash: hashToken(token),
		ExpiresAt: expiresAt,
	}); err != nil {
		return "", time.Time{}, fmt.Errorf("gravar sessão: %w", err)
	}

	return token, expiresAt, nil
}

// Validate devolve a conta dona de uma sessão viva.
//
// "Viva" inclui a conta ainda estar ACTIVE: a checagem é feita no mesmo SELECT
// (ver FindLiveSession), então bloquear uma conta derruba as sessões dela na
// requisição seguinte, sem varredura nem cache para invalidar.
func (s *SessionStore) Validate(ctx context.Context, token string) (Account, error) {
	if token == "" {
		return Account{}, ErrNoSession
	}

	row, err := s.q.FindLiveSession(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Account{}, ErrNoSession
		}
		return Account{}, fmt.Errorf("procurar sessão: %w", err)
	}

	return Account{ID: row.AccountID, FullName: row.FullName, Email: row.Email}, nil
}

// Revoke encerra a sessão. Token desconhecido não é erro: logout com cookie
// velho é rotina, e responder diferente diria ao cliente quais tokens existem.
func (s *SessionStore) Revoke(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := s.q.RevokeSessionByTokenHash(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("revogar sessão: %w", err)
	}
	return nil
}

// hashToken reduz o token ao que o banco pode guardar.
//
// SHA-256 sem salt de propósito — e isto é o oposto do que fazemos com senha.
// A diferença: o token tem 256 bits de entropia real e vida curta, então não há
// dicionário a construir nem força bruta viável. Salt e custo alto (Argon2)
// existem para compensar a entropia baixa de senhas humanas; aqui só custariam
// latência em toda requisição autenticada.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
