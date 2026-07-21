package models

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/adapters/dav"
	"github.com/renovisaude/renovi-care/internal/db/gen"
	"github.com/renovisaude/renovi-care/internal/models/cpf"
	"github.com/renovisaude/renovi-care/internal/models/credential"
)

// Erros do cadastro e do login. Use errors.Is.
var (
	// ErrInvalidRegistration: os dados enviados não passam na validação.
	ErrInvalidRegistration = errors.New("models: dados de cadastro inválidos")
	// ErrAlreadyRegistered: já existe conta ativa para este CPF ou e-mail.
	// Genérico de propósito: dizer qual dos dois colidiu permitiria enumerar.
	ErrAlreadyRegistered = errors.New("models: já existe conta para estes dados")
	// ErrEmailTakenAtDAV: o e-mail pertence a OUTRA pessoa na DAV, que exige
	// e-mail único. É o caso do casal que compartilha e-mail — merece mensagem
	// própria na UI, não um "dados inválidos" genérico.
	ErrEmailTakenAtDAV = errors.New("models: e-mail já em uso na Doutor ao Vivo")
	// ErrDAVUnavailable: a DAV não confirmou. A conta fica PENDING_DAV e não
	// autentica; tentar de novo é seguro.
	ErrDAVUnavailable = errors.New("models: a Doutor ao Vivo não confirmou o cadastro")
	// ErrInvalidCredentials: login recusado. Um só erro para CPF inexistente,
	// senha errada e conta não vinculada — diferenciá-los diria ao atacante
	// quais CPFs têm conta.
	ErrInvalidCredentials = errors.New("models: credenciais inválidas")
)

const (
	statusPendingDAV = "PENDING_DAV"
	statusActive     = "ACTIVE"

	originCreated  = "CREATED"
	originAttached = "ATTACHED"

	uniqueViolation = "23505" // SQLSTATE
)

// DAVClient é o que o cadastro precisa da DAV.
//
// A interface vive aqui, no consumidor, e não no adapter: quem usa declara o que
// precisa, o teste injeta um fake, e o método que não interessa ao cadastro
// (GetPerson) não polui o contrato.
type DAVClient interface {
	FindPersonByCPF(ctx context.Context, cpf string) (*dav.Person, error)
	CreatePerson(ctx context.Context, in dav.CreatePersonInput) (string, error)
	// GetPerson é a SONDA de reconciliação: quando a criação termina sem
	// resposta utilizável, é ela que diz se a pessoa foi criada mesmo assim.
	GetPerson(ctx context.Context, id string) (*dav.Person, error)
}

// Account é a conta do paciente como o resto do sistema a enxerga.
//
// Enxuta de propósito: NADA que venha da DAV entra aqui. O lookup por CPF de lá
// devolve 12 campos de qualquer pessoa, e o que não existe nesta struct não tem
// como vazar na resposta do cadastro.
type Account struct {
	ID       uuid.UUID
	FullName string
	Email    string
}

// Address é o endereço do paciente.
type Address struct {
	ZipCode      string
	Street       string
	Number       string
	Complement   string
	Neighborhood string
	City         string
	State        string
	Country      string
}

// RegisterInput são os dados do cadastro.
type RegisterInput struct {
	FullName  string
	CPF       string
	BirthDate time.Time
	Email     string
	Phone     string
	Password  string
	Address   Address
	// RequestIP alimenta a auditoria do vínculo. Vazio é aceito (some da trilha).
	RequestIP string
}

// AccountStore é a camada de dados + regra do cadastro.
type AccountStore struct {
	pool *pgxpool.Pool
	q    *gen.Queries
	dav  DAVClient
}

// NewAccountStore monta o store.
func NewAccountStore(pool *pgxpool.Pool, davClient DAVClient) *AccountStore {
	return &AccountStore{pool: pool, q: gen.New(pool), dav: davClient}
}

// Register cadastra o paciente e o vincula à DAV, de forma síncrona.
//
// O desenho tem três passos, e a divisão é deliberada:
//
//  1. TX curta   — reserva o CPF (a conta nasce PENDING_DAV, que não autentica)
//  2. Sem TX     — fala com a DAV (lenta: ~2s, e o gateway dela corta em 29s)
//  3. TX curta   — grava o vínculo e ativa a conta
//
// Por que não uma transação só: segurar uma transação aberta durante uma chamada
// HTTP de dezenas de segundos prende uma conexão do pool por todo esse tempo e
// derruba a API sob carga.
//
// A exigência "só cadastra se tiver tudo certo lá" é honrada porque PENDING_DAV
// não loga — e isso está gravado no banco (CHECK active_exige_vinculo_dav), não
// só neste código.
//
// O retry sai de graça: o passo 2 COMEÇA pelo lookup por CPF, então uma tentativa
// que criou a pessoa na DAV e morreu antes de gravar é reencontrada na seguinte.
func (s *AccountStore) Register(ctx context.Context, in RegisterInput) (Account, error) {
	v, err := validate(in)
	if err != nil {
		return Account{}, err
	}

	row, err := s.reserve(ctx, v)
	if err != nil {
		return Account{}, err
	}

	davPersonID, origin, err := s.resolveDAV(ctx, row.ID, v)
	if err != nil {
		return Account{}, err
	}

	if err := s.commitLink(ctx, row.ID, davPersonID, origin, v.ip); err != nil {
		return Account{}, err
	}

	return Account{ID: row.ID, FullName: row.FullName, Email: row.Email}, nil
}

// reserve é a TX1: garante que este CPF é nosso antes de gastar uma chamada à
// DAV, e evita que dois cadastros simultâneos criem a mesma pessoa lá.
func (s *AccountStore) reserve(ctx context.Context, v validated) (gen.PatientAccount, error) {
	var zero gen.PatientAccount

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return zero, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	var row gen.PatientAccount
	existing, err := q.FindAccountByCPF(ctx, v.cpf.String())
	switch {
	case err == nil:
		// Uma conta ACTIVE (ou BLOCKED) nunca é sobrescrita por quem só conhece
		// o CPF. Já uma PENDING_DAV é uma tentativa que não vingou: reaproveitar
		// é o que permite ao usuário simplesmente tentar de novo.
		if existing.Status != statusPendingDAV {
			return zero, ErrAlreadyRegistered
		}
		row, err = q.RefreshPendingAccount(ctx, gen.RefreshPendingAccountParams{
			ID: existing.ID, FullName: v.in.FullName, Email: v.email,
			Phone: v.in.Phone, BirthDate: v.birthDate, PasswordHash: v.hash,
		})
		if err != nil {
			return zero, mapWriteErr(err)
		}

	case errors.Is(err, pgx.ErrNoRows):
		id, err := uuid.NewV7()
		if err != nil {
			return zero, fmt.Errorf("gerar uuid v7: %w", err)
		}
		row, err = q.InsertAccount(ctx, gen.InsertAccountParams{
			ID: id, FullName: v.in.FullName, Email: v.email,
			Phone: v.in.Phone, BirthDate: v.birthDate, PasswordHash: v.hash,
		})
		if err != nil {
			return zero, mapWriteErr(err)
		}
		if err := q.InsertIdentity(ctx, gen.InsertIdentityParams{
			AccountID: row.ID, Cpf: v.cpf.String(),
		}); err != nil {
			return zero, mapWriteErr(err)
		}

	default:
		return zero, fmt.Errorf("procurar conta por cpf: %w", err)
	}

	if err := q.UpsertAddress(ctx, gen.UpsertAddressParams{
		AccountID: row.ID, ZipCode: v.zipCode, Street: v.in.Address.Street,
		Number: v.in.Address.Number, Complement: text(v.in.Address.Complement),
		Neighborhood: v.in.Address.Neighborhood, City: v.in.Address.City,
		State: strings.ToUpper(v.in.Address.State), Country: v.country,
	}); err != nil {
		return zero, mapWriteErr(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return zero, fmt.Errorf("commit da reserva: %w", err)
	}
	return row, nil
}

// resolveDAV é o passo 2: descobre o id da pessoa na DAV, criando-a se preciso.
// Roda FORA de transação — a DAV é lenta demais para segurar uma.
func (s *AccountStore) resolveDAV(ctx context.Context, accountID uuid.UUID, v validated) (string, string, error) {
	// Sempre começa pelo lookup. É isto que torna o retry idempotente e o que
	// implementa o item 2 do escopo: se a pessoa já existe lá, anexa em vez de
	// criar (é onde estão os dados de saúde dela).
	p, err := s.dav.FindPersonByCPF(ctx, v.cpf.String())
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrDAVUnavailable, err)
	}
	if p != nil {
		return p.ID, originFor(p.ID, accountID), nil
	}

	davID, err := s.dav.CreatePerson(ctx, dav.CreatePersonInput{
		ID:        accountID.String(), // a DAV honra o id do integrador
		Name:      v.in.FullName,
		CPF:       v.cpf.String(),
		BirthDate: v.in.BirthDate.Format(time.DateOnly),
		Email:     v.email,
		CellPhone: v.in.Phone,
		Address: &dav.Address{
			ZipCode: v.zipCode, Street: v.in.Address.Street, Number: v.in.Address.Number,
			Complement: v.in.Address.Complement, Neighborhood: v.in.Address.Neighborhood,
			City: v.in.Address.City, State: strings.ToUpper(v.in.Address.State),
			Country: v.country,
		},
	})

	switch {
	case err == nil:
		// Criação confirmada é SEMPRE CREATED, mesmo que a DAV tenha devolvido um
		// id diferente do nosso. Usar originFor aqui marcaria a criação como
		// ATTACHED no dia em que ela parasse de honrar o id do integrador — e
		// ATTACHED na trilha significa "ligou-se ao prontuário de um terceiro",
		// que é justamente o evento que o ADR-013 manda revisar. Mentir aí
		// envenenaria a auditoria de forma silenciosa.
		return davID, originCreated, nil

	case errors.Is(err, dav.ErrDuplicateEmail):
		return "", "", ErrEmailTakenAtDAV

	case errors.Is(err, dav.ErrMaybeApplied):
		// O caso que a realidade nos ensinou. A criação terminou sem resposta
		// utilizável (504 do gateway aos 29s, timeout, ou 409 "id already
		// exists") — mas pode ter funcionado. Concluir "falhou" aqui reprova um
		// cadastro que deu certo E prende o usuário: toda nova tentativa
		// esbarraria na pessoa que já existe lá.
		//
		// A sonda é barata e decide: perguntamos pelo NOSSO id, que a DAV honra.
		return s.reconcile(ctx, accountID, err)

	case errors.Is(err, dav.ErrDuplicateCPF):
		// Corrida: alguém criou a pessoa entre o nosso lookup e o nosso create.
		if p, ferr := s.dav.FindPersonByCPF(ctx, v.cpf.String()); ferr == nil && p != nil {
			return p.ID, originFor(p.ID, accountID), nil
		}
		return "", "", fmt.Errorf("%w: a DAV diz que o cpf existe, mas não o devolve", ErrDAVUnavailable)

	case errors.Is(err, dav.ErrValidation):
		return "", "", fmt.Errorf("%w: a DAV recusou os dados", ErrInvalidRegistration)

	default:
		return "", "", fmt.Errorf("%w: %v", ErrDAVUnavailable, err)
	}
}

// reconcile pergunta à DAV se a criação incerta chegou a acontecer.
//
// Três desfechos: achou (a pessoa é nossa → CREATED), não achou (falhou de
// verdade), ou a própria sonda caiu (continua desconhecido → PENDING_DAV, e o
// usuário tenta de novo em segurança).
func (s *AccountStore) reconcile(ctx context.Context, accountID uuid.UUID, cause error) (string, string, error) {
	p, probeErr := s.dav.GetPerson(ctx, accountID.String())
	if probeErr != nil {
		return "", "", fmt.Errorf("%w: criação incerta (%v) e a sonda também falhou (%v)",
			ErrDAVUnavailable, cause, probeErr)
	}
	if p == nil {
		return "", "", fmt.Errorf("%w: %v", ErrDAVUnavailable, cause)
	}

	// A pessoa está lá com o nosso id: o POST tinha funcionado. Origem é CREATED
	// — fomos nós, ainda que sem confirmação na hora.
	return accountID.String(), originCreated, nil
}

// originFor distingue "criamos" de "anexamos".
//
// Sutil e importante: se o id que a DAV devolveu é o NOSSO, então fomos nós que
// criamos a pessoa (numa tentativa anterior que caiu). Marcar isso como ATTACHED
// mentiria na auditoria — e a auditoria é justamente o que vai permitir revisar
// anexações quando o fator de posse existir.
func originFor(davPersonID string, accountID uuid.UUID) string {
	if davPersonID == accountID.String() {
		return originCreated
	}
	return originAttached
}

// commitLink é a TX3: ativa a conta e grava a trilha, atomicamente.
func (s *AccountStore) commitLink(ctx context.Context, accountID uuid.UUID, davPersonID, origin string, ip *netip.Addr) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("abrir transação do vínculo: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	n, err := q.LinkAccountToDav(ctx, gen.LinkAccountToDavParams{
		ID: accountID, DavPersonID: text(davPersonID), DavLinkOrigin: text(origin),
	})
	if err != nil {
		return fmt.Errorf("gravar vínculo: %w", err)
	}
	// Zero linhas = a conta deixou de estar PENDING_DAV enquanto falávamos com a
	// DAV (outra requisição a ativou). Parar aqui evita auditar um vínculo que
	// não aconteceu — a trilha só registra o que o banco de fato aplicou.
	if n == 0 {
		return ErrAlreadyRegistered
	}

	auditID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("gerar uuid v7: %w", err)
	}
	if err := q.InsertDavLinkAudit(ctx, gen.InsertDavLinkAuditParams{
		ID: auditID, AccountID: accountID, DavPersonID: davPersonID,
		Origin: origin, RequestIp: ip,
	}); err != nil {
		return fmt.Errorf("auditar vínculo: %w", err)
	}

	// A conta acabou de virar ACTIVE: matricula-a na linha de cuidado universal
	// (Verificador de Humor para todos, Degrau 1/ADR-040), NA MESMA TX — ativação e
	// matrícula comitam juntas. Idempotente e fail-open (seed ausente não bloqueia).
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := insertUniversalEnrollment(ctx, q, accountID, now); err != nil {
		return fmt.Errorf("matricular na linha universal: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit do vínculo: %w", err)
	}
	return nil
}

// Authenticate confere CPF e senha. Só conta ACTIVE entra.
func (s *AccountStore) Authenticate(ctx context.Context, rawCPF, password string) (Account, error) {
	c, err := cpf.Parse(rawCPF)
	if err != nil {
		return Account{}, ErrInvalidCredentials
	}

	row, err := s.q.FindAccountByCPF(ctx, c.String())
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Gasta o mesmo tempo de um Verify real. Sem isto, um CPF sem conta
			// responde em microssegundos e um com conta demora ~50ms — e essa
			// diferença é um oráculo de quem tem cadastro.
			_ = credential.Verify(timingDummyHash(), password)
			return Account{}, ErrInvalidCredentials
		}
		return Account{}, fmt.Errorf("procurar conta por cpf: %w", err)
	}

	// PENDING_DAV não loga: é o que faz "só cadastra se tiver tudo certo lá"
	// valer na prática.
	if row.Status != statusActive {
		_ = credential.Verify(timingDummyHash(), password)
		return Account{}, ErrInvalidCredentials
	}

	if err := credential.Verify(row.PasswordHash, password); err != nil {
		return Account{}, ErrInvalidCredentials
	}

	return Account{ID: row.ID, FullName: row.FullName, Email: row.Email}, nil
}

// timingDummyHash é um hash real, calculado uma vez, para equalizar o tempo do
// login quando a conta não existe.
var timingDummyHash = sync.OnceValue(func() string {
	h, err := credential.Hash("senha-irrelevante-so-para-gastar-o-mesmo-tempo")
	if err != nil {
		return ""
	}
	return h
})

// ---------------------------------------------------------------------------
// Validação
// ---------------------------------------------------------------------------

type validated struct {
	in        RegisterInput
	cpf       cpf.CPF
	email     string
	hash      string
	zipCode   string
	country   string
	birthDate pgtype.Date
	ip        *netip.Addr
}

func validate(in RegisterInput) (validated, error) {
	var v validated

	if strings.TrimSpace(in.FullName) == "" {
		return v, fmt.Errorf("%w: nome é obrigatório", ErrInvalidRegistration)
	}

	c, err := cpf.Parse(in.CPF)
	if err != nil {
		return v, fmt.Errorf("%w: %v", ErrInvalidRegistration, err)
	}

	addr, err := mail.ParseAddress(strings.TrimSpace(in.Email))
	if err != nil {
		return v, fmt.Errorf("%w: e-mail inválido", ErrInvalidRegistration)
	}

	if err := credential.CheckPolicy(in.Password); err != nil {
		return v, fmt.Errorf("%w: %v", ErrInvalidRegistration, err)
	}
	hash, err := credential.Hash(in.Password)
	if err != nil {
		return v, fmt.Errorf("gerar hash da senha: %w", err)
	}

	if in.BirthDate.IsZero() || in.BirthDate.After(time.Now()) {
		return v, fmt.Errorf("%w: data de nascimento inválida", ErrInvalidRegistration)
	}

	zip := digitsOnly(in.Address.ZipCode)
	if len(zip) != 8 {
		return v, fmt.Errorf("%w: CEP inválido", ErrInvalidRegistration)
	}
	if len(strings.TrimSpace(in.Address.State)) != 2 {
		return v, fmt.Errorf("%w: UF inválida", ErrInvalidRegistration)
	}
	for campo, valor := range map[string]string{
		"logradouro": in.Address.Street, "número": in.Address.Number,
		"bairro": in.Address.Neighborhood, "cidade": in.Address.City,
	} {
		if strings.TrimSpace(valor) == "" {
			return v, fmt.Errorf("%w: %s é obrigatório", ErrInvalidRegistration, campo)
		}
	}
	if strings.TrimSpace(in.Phone) == "" {
		return v, fmt.Errorf("%w: celular é obrigatório", ErrInvalidRegistration)
	}

	country := strings.ToUpper(strings.TrimSpace(in.Address.Country))
	if country == "" {
		country = "BR"
	}

	v = validated{
		in: in, cpf: c, email: addr.Address, hash: hash,
		zipCode: zip, country: country,
		birthDate: pgtype.Date{Time: in.BirthDate, Valid: true},
		ip:        parseIP(in.RequestIP),
	}
	return v, nil
}

// mapWriteErr traduz violação de unicidade em ErrAlreadyRegistered. Cobre tanto
// o e-mail (índice funcional) quanto o CPF (corrida entre duas requisições).
func mapWriteErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return ErrAlreadyRegistered
	}
	return err
}

func parseIP(s string) *netip.Addr {
	a, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return &a
}

func text(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
