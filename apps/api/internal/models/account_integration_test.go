//go:build integration

package models_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/adapters/dav"
	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/testsupport"
)

// CPF sintético com DV válido (o mesmo do exemplo do spec da DAV).
const cpfValido = "94819089846"

// fakeDAV substitui a DAV nos testes. A interface vive no consumidor
// (models.DAVClient), então não é preciso subir HTTP para exercitar o cadastro.
type fakeDAV struct {
	findResult *dav.Person
	findErr    error
	createID   string
	createErr  error
	// getResult/getErr respondem à SONDA de reconciliação (GetPerson).
	getResult   *dav.Person
	getErr      error
	createCalls int
	findCalls   int
	getCalls    int
}

func (f *fakeDAV) GetPerson(_ context.Context, _ string) (*dav.Person, error) {
	f.getCalls++
	return f.getResult, f.getErr
}

func (f *fakeDAV) FindPersonByCPF(_ context.Context, _ string) (*dav.Person, error) {
	f.findCalls++
	return f.findResult, f.findErr
}

func (f *fakeDAV) CreatePerson(_ context.Context, in dav.CreatePersonInput) (string, error) {
	f.createCalls++
	if f.createErr != nil {
		return "", f.createErr
	}
	if f.createID != "" {
		return f.createID, nil
	}
	return in.ID, nil // honra o id do integrador, como a DAV real faz
}

func newStore(t *testing.T, d models.DAVClient) (*models.AccountStore, *pgxpool.Pool) {
	t.Helper()
	dsn := testsupport.StartPostgres(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return models.NewAccountStore(pool, d), pool
}

func validInput() models.RegisterInput {
	return models.RegisterInput{
		FullName:  "Roberval Juvencio Lazaroti",
		CPF:       cpfValido,
		BirthDate: time.Date(1976, 1, 23, 0, 0, 0, 0, time.UTC),
		Email:     "roberval@example.com",
		Phone:     "11912345678",
		Password:  "cavalo-bateria-grampo-correto",
		RequestIP: "203.0.113.7",
		Address: models.Address{
			ZipCode: "06472000", Street: "Avenida Copacabana", Number: "238",
			Neighborhood: "Dezoito do Forte", City: "Barueri", State: "SP",
		},
	}
}

// dbState lê o estado cru da conta, para asserir o que o domínio não expõe.
type dbState struct {
	status      string
	davPersonID *string
	origin      *string
	auditCount  int
	auditOrigin *string
}

func readDB(t *testing.T, pool *pgxpool.Pool, cpf string) dbState {
	t.Helper()
	var s dbState
	err := pool.QueryRow(context.Background(), `
		SELECT a.status, a.dav_person_id, a.dav_link_origin
		FROM patient_account a JOIN patient_identity i ON i.account_id = a.id
		WHERE i.cpf = $1`, cpf).Scan(&s.status, &s.davPersonID, &s.origin)
	require.NoError(t, err)

	err = pool.QueryRow(context.Background(), `
		SELECT count(*), max(l.origin)
		FROM dav_link_audit l JOIN patient_identity i ON i.account_id = l.account_id
		WHERE i.cpf = $1`, cpf).Scan(&s.auditCount, &s.auditOrigin)
	require.NoError(t, err)
	return s
}

// ---------------------------------------------------------------------------
// Register — os dois caminhos do item 2 do escopo
// ---------------------------------------------------------------------------

func TestRegister_CriaPessoaNaDAVQuandoNaoExiste(t *testing.T) {
	f := &fakeDAV{findResult: nil} // 204: ninguém com este CPF lá
	store, pool := newStore(t, f)

	acc, err := store.Register(context.Background(), validInput())
	require.NoError(t, err)

	require.Equal(t, 1, f.findCalls, "o cadastro precisa começar pelo lookup de CPF")
	require.Equal(t, 1, f.createCalls)

	s := readDB(t, pool, cpfValido)
	require.Equal(t, "ACTIVE", s.status)
	require.NotNil(t, s.davPersonID)
	// Criamos nós, então o id da DAV é o nosso — é o que dá idempotência ao retry.
	require.Equal(t, acc.ID.String(), *s.davPersonID)
	require.Equal(t, "CREATED", *s.origin)
	require.Equal(t, 1, s.auditCount, "todo vínculo precisa ser auditado")
	require.Equal(t, "CREATED", *s.auditOrigin)
}

func TestRegister_AnexaPessoaQueJaExisteNaDAV(t *testing.T) {
	f := &fakeDAV{findResult: &dav.Person{
		ID: "id-antigo-da-dav", Name: "Roberval J. Lazaroti", CPF: cpfValido,
		BirthDate: "1976-01-23", Email: "outro@example.com",
	}}
	store, pool := newStore(t, f)

	acc, err := store.Register(context.Background(), validInput())
	require.NoError(t, err)

	require.Equal(t, 0, f.createCalls, "a pessoa já existia: não pode criar de novo")

	s := readDB(t, pool, cpfValido)
	require.Equal(t, "ACTIVE", s.status)
	require.Equal(t, "id-antigo-da-dav", *s.davPersonID, "tem que anexar ao id que já existia")
	require.Equal(t, "ATTACHED", *s.origin)
	require.Equal(t, "ATTACHED", *s.auditOrigin, "ATTACHED é o caso sensível — precisa de trilha")

	// A conta devolvida não pode carregar NADA da pessoa encontrada na DAV:
	// nome, e-mail e id de lá são dados de terceiro até prova em contrário.
	require.Equal(t, validInput().FullName, acc.FullName)
	require.Equal(t, validInput().Email, acc.Email)
	require.NotEqual(t, "id-antigo-da-dav", acc.ID.String())
}

// Se uma tentativa anterior criou a pessoa e caiu antes de gravar, o retry a
// encontra pelo CPF — mas com o NOSSO id. Marcar isso como ATTACHED mentiria na
// auditoria: fomos nós que criamos.
func TestRegister_PessoaEncontradaComNossoIDContaComoCREATED(t *testing.T) {
	f := &fakeDAV{findErr: dav.ErrUnavailable}
	store, pool := newStore(t, f)

	// 1ª tentativa: a DAV cai depois de (hipoteticamente) criar a pessoa. A conta
	// fica PENDING_DAV, com o nosso id já reservado. É o cenário real do 504: o
	// gateway desiste em ~29s, mas a criação pode ter chegado ao backend deles.
	_, err := store.Register(context.Background(), validInput())
	require.ErrorIs(t, err, models.ErrDAVUnavailable)

	accountID := accountIDOf(t, pool, cpfValido)

	// 2ª tentativa: a DAV volta e o lookup devolve uma pessoa com o NOSSO id —
	// prova de que a tentativa anterior chegou a criá-la lá.
	f.findErr = nil
	f.findResult = &dav.Person{ID: accountID, CPF: cpfValido}
	_, err = store.Register(context.Background(), validInput())
	require.NoError(t, err)

	s := readDB(t, pool, cpfValido)
	require.Equal(t, "CREATED", *s.origin,
		"o id encontrado é o nosso: fomos nós que criamos, não é anexação de prontuário alheio")
	require.Equal(t, 0, f.createCalls, "a pessoa já está lá: criar de novo daria 422")
}

func accountIDOf(t *testing.T, pool *pgxpool.Pool, cpf string) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT a.id::text FROM patient_account a
		JOIN patient_identity i ON i.account_id = a.id WHERE i.cpf = $1`, cpf).Scan(&id))
	return id
}

// ---------------------------------------------------------------------------
// A regra central: sem DAV, não há conta utilizável
// ---------------------------------------------------------------------------

func TestRegister_DAVForaDeixaContaPendenteEBloqueiaLogin(t *testing.T) {
	f := &fakeDAV{findErr: dav.ErrUnavailable}
	store, pool := newStore(t, f)
	in := validInput()

	_, err := store.Register(context.Background(), in)
	require.ErrorIs(t, err, models.ErrDAVUnavailable)

	s := readDB(t, pool, cpfValido)
	require.Equal(t, "PENDING_DAV", s.status, "sem confirmação da DAV a conta não pode ativar")
	require.Nil(t, s.davPersonID)
	require.Equal(t, 0, s.auditCount, "não houve vínculo: nada a auditar")

	// A exigência do escopo: a conta não existe do ponto de vista do usuário.
	_, err = store.Authenticate(context.Background(), in.CPF, in.Password)
	require.ErrorIs(t, err, models.ErrInvalidCredentials, "conta PENDING_DAV não pode logar")
}

// Tentar de novo depois que a DAV voltou tem que funcionar — e reaproveitar a
// linha pendente, sem esbarrar na unicidade do CPF que ela mesma reservou.
func TestRegister_RetryDepoisDaDAVVoltarReaproveitaLinhaPendente(t *testing.T) {
	f := &fakeDAV{findErr: dav.ErrUnavailable}
	store, pool := newStore(t, f)

	_, err := store.Register(context.Background(), validInput())
	require.ErrorIs(t, err, models.ErrDAVUnavailable)

	f.findErr = nil
	f.findResult = nil
	acc, err := store.Register(context.Background(), validInput())
	require.NoError(t, err, "o retry não pode falhar por CPF duplicado da própria tentativa anterior")

	s := readDB(t, pool, cpfValido)
	require.Equal(t, "ACTIVE", s.status)
	require.Equal(t, acc.ID.String(), *s.davPersonID)
}

func TestRegister_CPFJaCadastradoEAtivo(t *testing.T) {
	store, _ := newStore(t, &fakeDAV{})

	_, err := store.Register(context.Background(), validInput())
	require.NoError(t, err)

	outro := validInput()
	outro.Email = "outro@example.com"
	_, err = store.Register(context.Background(), outro)
	require.ErrorIs(t, err, models.ErrAlreadyRegistered)
}

func TestRegister_EmailJaCadastrado(t *testing.T) {
	store, _ := newStore(t, &fakeDAV{})

	_, err := store.Register(context.Background(), validInput())
	require.NoError(t, err)

	outro := validInput()
	outro.CPF = "00000003700" // CPF válido diferente
	_, err = store.Register(context.Background(), outro)
	require.ErrorIs(t, err, models.ErrAlreadyRegistered)
}

// A DAV exige e-mail único (achado #6). Casal que compartilha e-mail cai aqui, e
// a UI precisa de uma mensagem própria — não um "dados inválidos" genérico.
func TestRegister_EmailJaUsadoNaDAVTemErroProprio(t *testing.T) {
	f := &fakeDAV{createErr: dav.ErrDuplicateEmail}
	store, pool := newStore(t, f)

	_, err := store.Register(context.Background(), validInput())
	require.ErrorIs(t, err, models.ErrEmailTakenAtDAV)

	require.Equal(t, "PENDING_DAV", readDB(t, pool, cpfValido).status)
}

func TestRegister_ValidaEntrada(t *testing.T) {
	tests := []struct {
		nome  string
		mutar func(*models.RegisterInput)
	}{
		{"CPF com DV inválido", func(i *models.RegisterInput) { i.CPF = "12345678901" }},
		{"CPF vazio", func(i *models.RegisterInput) { i.CPF = "" }},
		{"senha curta", func(i *models.RegisterInput) { i.Password = "curta123" }},
		{"nome vazio", func(i *models.RegisterInput) { i.FullName = "  " }},
		{"e-mail sem @", func(i *models.RegisterInput) { i.Email = "nao-e-email" }},
	}
	store, _ := newStore(t, &fakeDAV{})

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			in := validInput()
			tt.mutar(&in)
			_, err := store.Register(context.Background(), in)
			require.ErrorIs(t, err, models.ErrInvalidRegistration)
		})
	}
}

// Entrada inválida não pode gastar uma chamada à DAV — que custa ~2s.
func TestRegister_EntradaInvalidaNaoChamaADAV(t *testing.T) {
	f := &fakeDAV{}
	store, _ := newStore(t, f)

	in := validInput()
	in.CPF = "12345678901"
	_, _ = store.Register(context.Background(), in)

	require.Equal(t, 0, f.findCalls, "validação tem que barrar antes do round-trip")
}

// ---------------------------------------------------------------------------
// Authenticate
// ---------------------------------------------------------------------------

func TestAuthenticate(t *testing.T) {
	store, _ := newStore(t, &fakeDAV{})
	in := validInput()
	acc, err := store.Register(context.Background(), in)
	require.NoError(t, err)

	t.Run("aceita CPF e senha corretos", func(t *testing.T) {
		got, err := store.Authenticate(context.Background(), in.CPF, in.Password)
		require.NoError(t, err)
		require.Equal(t, acc.ID, got.ID)
	})

	t.Run("aceita CPF formatado", func(t *testing.T) {
		_, err := store.Authenticate(context.Background(), "948.190.898-46", in.Password)
		require.NoError(t, err)
	})

	t.Run("recusa senha errada", func(t *testing.T) {
		_, err := store.Authenticate(context.Background(), in.CPF, "senha-errada-mesmo")
		require.ErrorIs(t, err, models.ErrInvalidCredentials)
	})

	t.Run("recusa CPF inexistente com o MESMO erro", func(t *testing.T) {
		// Erro idêntico ao de senha errada: distinguir os dois diria ao atacante
		// quais CPFs têm conta.
		_, err := store.Authenticate(context.Background(), "00000003700", in.Password)
		require.ErrorIs(t, err, models.ErrInvalidCredentials)
	})

	t.Run("recusa CPF inválido", func(t *testing.T) {
		_, err := store.Authenticate(context.Background(), "12345678901", in.Password)
		require.ErrorIs(t, err, models.ErrInvalidCredentials)
	})
}

// ---------------------------------------------------------------------------
// Reconciliação do 504 que mente
// ---------------------------------------------------------------------------

// O cenário que reprovou o primeiro cadastro real pelo browser: o POST estourou
// o teto de 29s do gateway (ou levou 409 "id already exists"), mas a pessoa FOI
// criada. Concluir "falhou" reprova um cadastro que deu certo — e o usuário
// fica preso, porque toda nova tentativa esbarra na pessoa que já existe lá.
func TestRegister_POSTIncertoMasAplicado_ReconciliaESegue(t *testing.T) {
	f := &fakeDAV{
		findResult: nil,                 // o lookup não achou: vamos criar
		createErr:  dav.ErrMaybeApplied, // ...e o POST morre sem dizer se pegou
	}
	store, pool := newStore(t, f)

	// A sonda encontra a pessoa: o POST tinha funcionado.
	f.getResult = &dav.Person{CPF: cpfValido}
	acc, err := store.Register(context.Background(), validInput())
	require.NoError(t, err, "a pessoa existe na DAV: o cadastro tem que concluir")

	require.Equal(t, 1, f.getCalls, "ErrMaybeApplied EXIGE sondar antes de concluir")

	s := readDB(t, pool, cpfValido)
	require.Equal(t, "ACTIVE", s.status)
	require.Equal(t, acc.ID.String(), *s.davPersonID)
	require.Equal(t, "CREATED", *s.origin, "fomos nós que criamos, ainda que sem confirmação")
}

// Se a sonda NÃO acha, aí sim falhou de verdade.
func TestRegister_POSTIncertoENaoAplicado_Falha(t *testing.T) {
	f := &fakeDAV{createErr: dav.ErrMaybeApplied, getResult: nil}
	store, pool := newStore(t, f)

	_, err := store.Register(context.Background(), validInput())
	require.ErrorIs(t, err, models.ErrDAVUnavailable)
	require.Equal(t, 1, f.getCalls)
	require.Equal(t, "PENDING_DAV", readDB(t, pool, cpfValido).status)
}

// Se a própria sonda cai, não dá para saber: PENDING_DAV e tenta de novo depois.
func TestRegister_SondaIndisponivel_DeixaPendente(t *testing.T) {
	f := &fakeDAV{createErr: dav.ErrMaybeApplied, getErr: dav.ErrUnavailable}
	store, pool := newStore(t, f)

	_, err := store.Register(context.Background(), validInput())
	require.ErrorIs(t, err, models.ErrDAVUnavailable)
	require.Equal(t, "PENDING_DAV", readDB(t, pool, cpfValido).status)
}

// Se a DAV honrar nosso id, ótimo — mas se ela devolver um id PRÓPRIO, a criação
// continua sendo uma criação. Marcá-la como ATTACHED gravaria na trilha de
// auditoria que este paciente se ligou ao prontuário de um terceiro, que é
// exatamente o evento que o ADR-013 manda revisar. Um POST confirmado é sempre
// CREATED, independente do id que volte.
func TestRegister_CriacaoComIDEscolhidoPelaDAVAindaEhCREATED(t *testing.T) {
	f := &fakeDAV{findResult: nil, createID: "id-que-a-dav-escolheu"}
	store, pool := newStore(t, f)

	_, err := store.Register(context.Background(), validInput())
	require.NoError(t, err)

	s := readDB(t, pool, cpfValido)
	require.Equal(t, "id-que-a-dav-escolheu", *s.davPersonID)
	require.Equal(t, "CREATED", *s.origin, "nós criamos: não é anexação de prontuário alheio")
	require.Equal(t, "CREATED", *s.auditOrigin)
}
