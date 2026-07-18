//go:build integration

package models_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/adapters/dav"
	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/models/scheduling"
	"github.com/renovisaude/renovi-care/internal/testsupport"
)

// A saga fala com dois sistemas externos. O Postgres é real (é o que guarda os
// CHECK que o desenho inteiro usa como rede); a agenda e a DAV são fakes, porque
// o que se testa aqui é a COORDENAÇÃO — cada um deles já tem a sua bateria contra
// o sistema de verdade.

var spTZ = mustTZ("America/Sao_Paulo")

func mustTZ(n string) *time.Location {
	l, err := time.LoadLocation(n)
	if err != nil {
		panic(err)
	}
	return l
}

const (
	slotID       = "50100000-0000-4000-8000-000000000001"
	profID       = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	specID       = "11111111-1111-4111-8111-111111111111"
	davPersonID  = "019f6ceb-1ff2-7616-af46-7574a621ac28"
	davApptID    = "13cd147e-68a7-45da-a65b-80b826cf674a"
	linkPaciente = "https://renovisaude.atendimento.hom.dav.med.br/a/sopr8brbkz"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakeAgenda struct {
	booking     agenda.Booking
	bookErr     error
	booked      bool
	released    bool
	releaseErr  error
	bookCalls   int
	loadErr     error
	releaseCall int
}

func novaAgenda(inicio time.Time) *fakeAgenda {
	return &fakeAgenda{
		booking: agenda.Booking{
			Slot:         agenda.Slot{ID: slotID, StartsAt: inicio, EndsAt: inicio.Add(25 * time.Minute)},
			Professional: agenda.Professional{ID: profID, FullName: "Ana Beatriz Moura"},
			Specialty:    agenda.Specialty{ID: specID, Name: "Psicologia"},
		},
	}
}

func (f *fakeAgenda) ListSpecialties(context.Context, time.Time) ([]agenda.Specialty, error) {
	return nil, nil
}
func (f *fakeAgenda) ListProfessionalsBySpecialty(context.Context, string, time.Time) ([]agenda.Professional, error) {
	return nil, nil
}
func (f *fakeAgenda) ListSlots(context.Context, string, time.Time, time.Time, time.Time) ([]agenda.Slot, error) {
	return nil, nil
}
func (f *fakeAgenda) GetProfessional(context.Context, string) (agenda.Professional, error) {
	return f.booking.Professional, nil
}
func (f *fakeAgenda) LoadBooking(context.Context, string, string) (agenda.Booking, error) {
	if f.loadErr != nil {
		return agenda.Booking{}, f.loadErr
	}
	return f.booking, nil
}
func (f *fakeAgenda) BookSlot(context.Context, string) error {
	f.bookCalls++
	if f.bookErr != nil {
		return f.bookErr
	}
	f.booked = true
	return nil
}
func (f *fakeAgenda) ReleaseSlot(context.Context, string) error {
	f.releaseCall++
	if f.releaseErr != nil {
		return f.releaseErr
	}
	f.released = true
	f.booked = false
	return nil
}
func (f *fakeAgenda) Location() *time.Location { return spTZ }

type fakeDAVAppts struct {
	err   error
	calls int
	// onCall roda DENTRO da chamada, antes de retornar. Serve para simular o
	// paciente fechando a aba no meio da chamada lenta à DAV (cancelando o ctx).
	onCall func()

	// Cancelamento: espelha o de cima. cancelledID guarda o id recebido, para o
	// teste provar que cancelamos a consulta certa na DAV.
	cancelErr   error
	cancelCalls int
	cancelledID string
}

func (f *fakeDAVAppts) CreateAppointment(context.Context, dav.CreateAppointmentInput) (dav.Appointment, error) {
	f.calls++
	if f.onCall != nil {
		f.onCall()
	}
	if f.err != nil {
		return dav.Appointment{}, f.err
	}
	return dav.Appointment{ID: davApptID, PatientJoinURL: linkPaciente}, nil
}

func (f *fakeDAVAppts) CancelAppointment(_ context.Context, id string) error {
	f.cancelCalls++
	f.cancelledID = id
	return f.cancelErr
}

// ---------------------------------------------------------------------------
// Cenário
// ---------------------------------------------------------------------------

type cenario struct {
	store *models.BookingStore
	ag    *fakeAgenda
	dv    *fakeDAVAppts
	pool  *pgxpool.Pool
	conta models.Account
	agora time.Time
}

func novoCenario(t *testing.T) cenario {
	t.Helper()
	dsn := testsupport.StartPostgres(t)

	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	agora := time.Date(2030, 3, 4, 8, 0, 0, 0, spTZ)
	inicio := time.Date(2030, 3, 4, 9, 0, 0, 0, spTZ)

	ag := novaAgenda(inicio)
	dv := &fakeDAVAppts{}
	store := models.NewBookingStore(pool, ag, dv,
		scheduling.Policy{OpensBefore: 30 * time.Minute}, nil)

	return cenario{store: store, ag: ag, dv: dv, pool: pool, conta: criaConta(t, pool), agora: agora}
}

func criaConta(t *testing.T, pool *pgxpool.Pool) models.Account {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO patient_account (id, full_name, email, phone, birth_date, password_hash,
		                             status, dav_person_id, dav_link_origin, dav_linked_at)
		VALUES ($1,'Maria de Teste','maria@example.com','11912345678','1990-01-01','x',
		        'ACTIVE',$2,'CREATED',now())`, id, davPersonID)
	require.NoError(t, err)
	return models.Account{ID: id, FullName: "Maria de Teste", Email: "maria@example.com"}
}

func (c cenario) entrada() models.BookInput {
	return models.BookInput{
		Account: c.conta, SlotID: slotID, SpecialtyID: specID, Now: c.agora,
	}
}

// statusNoBanco lê o estado interno da saga — o que o paciente vê é outra coisa.
func (c cenario) statusNoBanco(t *testing.T, id string) (status string, held, released bool) {
	t.Helper()
	err := c.pool.QueryRow(context.Background(),
		`SELECT status, slot_held_at IS NOT NULL, slot_released_at IS NOT NULL
		 FROM appointment WHERE id = $1`, id).Scan(&status, &held, &released)
	require.NoError(t, err)
	return
}

func (c cenario) idDaUnica(t *testing.T) string {
	t.Helper()
	var id string
	require.NoError(t, c.pool.QueryRow(context.Background(), `SELECT id FROM appointment`).Scan(&id))
	return id
}

// ---------------------------------------------------------------------------
// Caminho feliz
// ---------------------------------------------------------------------------

func TestBook_CaminhoFeliz(t *testing.T) {
	c := novoCenario(t)

	got, err := c.store.Book(context.Background(), c.entrada())
	require.NoError(t, err)

	require.Equal(t, "CONFIRMED", got.Status)
	require.Equal(t, "Psicologia", got.SpecialtyName)
	require.Equal(t, "America/Sao_Paulo", got.TimeZone)
	require.True(t, c.ag.booked, "o horário tem que ficar reservado no legado")

	status, held, released := c.statusNoBanco(t, got.ID)
	require.Equal(t, "CONFIRMED", status)
	require.True(t, held)
	require.False(t, released)

	// A url NÃO viaja no payload da consulta: só sai do JoinURL().
	url, err := c.store.JoinURL(context.Background(), uuid.MustParse(got.ID), c.conta.ID, c.agora.Add(45*time.Minute))
	require.NoError(t, err)
	require.Equal(t, linkPaciente, url)
}

// ---------------------------------------------------------------------------
// O ramo que justifica o desenho: desconhecido NÃO devolve o horário
// ---------------------------------------------------------------------------

// A DAV não respondeu. A consulta PODE existir lá, e nunca vamos descobrir
// sozinhos: o id é deles, não há rota de busca e o cancel deles responde 500.
// Devolver o horário deixaria outro paciente marcar em cima de uma consulta
// fantasma — e a DAV aceita duas no mesmo horário para o mesmo profissional.
func TestBook_DAVDesconhecido_SeguraOHorarioEChamaGente(t *testing.T) {
	c := novoCenario(t)
	c.dv.err = dav.ErrMaybeApplied

	_, err := c.store.Book(context.Background(), c.entrada())
	require.ErrorIs(t, err, models.ErrBookingUnconfirmed)

	require.True(t, c.ag.booked, "o horário NÃO pode voltar ao mercado: a consulta talvez exista")
	require.Zero(t, c.ag.releaseCall, "nem sequer tentamos devolver")
	require.Equal(t, 1, c.dv.calls, "escrita na DAV nunca repete: criaria uma segunda consulta real")

	status, held, released := c.statusNoBanco(t, c.idDaUnica(t))
	require.Equal(t, "DAV_UNKNOWN", status)
	require.True(t, held)
	require.False(t, released)
}

// M1: o paciente fecha a aba DURANTE a chamada lenta à DAV, cancelando o ctx do
// request. A finalização (markUnknown) roda em contexto DESACOPLADO, então o
// estado ainda transiciona — sem isso, a linha ficava presa em DAV_PENDING e, no
// caminho de sucesso, uma consulta real com link seria jogada fora. Este teste
// falha sem o detach() e passa com ele.
func TestBook_CtxCanceladoNaDAVAindaFinaliza(t *testing.T) {
	c := novoCenario(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.dv.err = dav.ErrMaybeApplied
	c.dv.onCall = cancel // o request morre no exato momento da chamada à DAV

	_, err := c.store.Book(ctx, c.entrada())
	require.ErrorIs(t, err, models.ErrBookingUnconfirmed)

	// Apesar do ctx cancelado, a transição foi gravada.
	status, held, released := c.statusNoBanco(t, c.idDaUnica(t))
	require.Equal(t, "DAV_UNKNOWN", status, "a finalização não pode se perder no cancelamento do request")
	require.True(t, held)
	require.False(t, released, "desconhecido NUNCA libera o horário")
}

// O paciente PRECISA ver esta consulta, mesmo sem confirmação: ele pode ter uma
// consulta de verdade marcada. Escondê-la seria pior que a incerteza.
func TestBook_DesconhecidoApareceComoUNCONFIRMED(t *testing.T) {
	c := novoCenario(t)
	c.dv.err = dav.ErrMaybeApplied
	_, _ = c.store.Book(context.Background(), c.entrada())

	lista, err := c.store.ListForAccount(context.Background(), c.conta.ID, c.agora)
	require.NoError(t, err)
	require.Len(t, lista, 1)
	require.Equal(t, "UNCONFIRMED", lista[0].Status)
	require.Equal(t, "UNAVAILABLE", string(lista[0].Join.Status), "não há sala para entrar")
}

// ---------------------------------------------------------------------------
// Ramos em que o horário PODE voltar
// ---------------------------------------------------------------------------

// 4xx é opinião firme da DAV: não houve efeito. Segurar o horário aqui seria
// vazá-lo à toa. E o erro é ErrBookingRejected (vira 422), NÃO ErrBookingUnconfirmed
// (502 "pode ter sido marcada") — não foi.
func TestBook_DAVRecusaDefinitiva_DevolveOHorario(t *testing.T) {
	c := novoCenario(t)
	c.dv.err = dav.ErrValidation

	_, err := c.store.Book(context.Background(), c.entrada())
	require.ErrorIs(t, err, models.ErrBookingRejected)
	require.NotErrorIs(t, err, models.ErrBookingUnconfirmed,
		"rejeição definitiva não pode virar 'desconhecido'")

	require.True(t, c.ag.released, "a DAV disse que não fez: o horário volta ao mercado")
	require.False(t, c.ag.booked)

	status, held, released := c.statusNoBanco(t, c.idDaUnica(t))
	require.Equal(t, "FAILED", status)
	require.True(t, held)
	require.True(t, released)
}

// Perdemos a corrida no CAS (o app legado ou outro processo nosso chegou antes).
// Não travamos nada, então não há o que compensar — e a DAV nem foi chamada.
func TestBook_HorarioTomadoNoLegado(t *testing.T) {
	c := novoCenario(t)
	c.ag.bookErr = agenda.ErrSlotTaken

	_, err := c.store.Book(context.Background(), c.entrada())
	require.ErrorIs(t, err, models.ErrSlotTaken)

	require.Zero(t, c.dv.calls, "não se fala com a DAV sem ter o horário")
	require.Zero(t, c.ag.releaseCall, "não travamos nada; não há o que devolver")

	status, _, _ := c.statusNoBanco(t, c.idDaUnica(t))
	require.Equal(t, "FAILED", status)
}

// Se a devolução falhar, a linha fica FAILED + held + !released — que é
// exatamente a fila que o worker varre. Perder o horário em silêncio seria pior.
func TestBook_FalhaAoDevolverDeixaAFilaDoWorker(t *testing.T) {
	c := novoCenario(t)
	c.dv.err = dav.ErrValidation
	c.ag.releaseErr = agenda.ErrUnavailable

	_, err := c.store.Book(context.Background(), c.entrada())
	require.ErrorIs(t, err, models.ErrBookingRejected)

	status, held, released := c.statusNoBanco(t, c.idDaUnica(t))
	require.Equal(t, "FAILED", status)
	require.True(t, held)
	require.False(t, released, "não anotamos o que não conseguimos fazer — o worker repete")
}

// ---------------------------------------------------------------------------
// Validações antes de escrever em qualquer lugar
// ---------------------------------------------------------------------------

func TestBook_RecusaAntesDeTocarEmQualquerSistema(t *testing.T) {
	casos := []struct {
		nome    string
		prepara func(*cenario)
		quero   error
	}{
		{"horário no passado", func(c *cenario) {
			c.agora = c.ag.booking.Slot.StartsAt.Add(time.Hour)
		}, models.ErrSlotExpired},
		{"horário já reservado", func(c *cenario) { c.ag.booking.Booked = true }, models.ErrSlotTaken},
		{"horário inexistente", func(c *cenario) { c.ag.loadErr = agenda.ErrSlotNotFound }, models.ErrSlotNotFound},
		{"profissional não atende a especialidade", func(c *cenario) {
			c.ag.loadErr = agenda.ErrSpecialtyMismatch
		}, models.ErrSpecialtyMismatch},
	}

	for _, tt := range casos {
		t.Run(tt.nome, func(t *testing.T) {
			c := novoCenario(t)
			tt.prepara(&c)

			_, err := c.store.Book(context.Background(), c.entrada())
			require.ErrorIs(t, err, tt.quero)
			require.Zero(t, c.ag.bookCalls, "nada de reservar")
			require.Zero(t, c.dv.calls, "nada de falar com a DAV")

			var n int
			require.NoError(t, c.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM appointment`).Scan(&n))
			require.Zero(t, n, "nem linha de intenção: a recusa é anterior a tudo")
		})
	}
}

// Conta sem vínculo não tem participante PAT, e consulta sem paciente não é
// consulta. Na prática ela nem chega aqui (a sessão só valida conta ACTIVE, e o
// CHECK active_exige_vinculo_dav garante que ACTIVE tem vínculo) — mas o
// agendamento não deve depender disso para não criar consulta órfã na DAV.
func TestBook_ContaSemVinculoComADAV(t *testing.T) {
	c := novoCenario(t)

	id, err := uuid.NewV7()
	require.NoError(t, err)
	_, err = c.pool.Exec(context.Background(), `
		INSERT INTO patient_account (id, full_name, email, phone, birth_date, password_hash, status)
		VALUES ($1,'Pendente','pendente@example.com','11912345671','1990-01-01','x','PENDING_DAV')`, id)
	require.NoError(t, err, "PENDING_DAV é o único estado sem dav_person_id que o banco aceita")

	in := c.entrada()
	in.Account = models.Account{ID: id, FullName: "Pendente", Email: "pendente@example.com"}

	_, err = c.store.Book(context.Background(), in)
	require.ErrorIs(t, err, models.ErrAccountNotLinked)
	require.Zero(t, c.ag.bookCalls, "nem chega a reservar horário")
	require.Zero(t, c.dv.calls, "e muito menos a falar com a DAV")
}

// ---------------------------------------------------------------------------
// Corrida entre dois pacientes NOSSOS
// ---------------------------------------------------------------------------

// Contra o app legado quem defende é o CAS no MySQL; contra nós mesmos, o índice
// único parcial. São adversários diferentes e precisamos dos dois.
func TestBook_DoisPacientesNossosNoMesmoHorario(t *testing.T) {
	c := novoCenario(t)
	ctx := context.Background()

	_, err := c.store.Book(ctx, c.entrada())
	require.NoError(t, err)

	outra := criaConta2(t, c.pool)
	in := c.entrada()
	in.Account = outra
	_, err = c.store.Book(ctx, in)
	require.ErrorIs(t, err, models.ErrSlotTaken, "o índice único parcial barra antes de tocar no legado")
}

func criaConta2(t *testing.T, pool *pgxpool.Pool) models.Account {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
		INSERT INTO patient_account (id, full_name, email, phone, birth_date, password_hash,
		                             status, dav_person_id, dav_link_origin, dav_linked_at)
		VALUES ($1,'Joao de Teste','joao@example.com','11912345670','1990-01-01','x',
		        'ACTIVE','outro-dav-id','CREATED',now())`, id)
	require.NoError(t, err)
	return models.Account{ID: id, FullName: "Joao de Teste", Email: "joao@example.com"}
}

// ---------------------------------------------------------------------------
// A janela de entrada, ponta a ponta
// ---------------------------------------------------------------------------

func TestJoinURL_RespeitaAJanelaEOEDono(t *testing.T) {
	c := novoCenario(t)
	ctx := context.Background()

	got, err := c.store.Book(ctx, c.entrada())
	require.NoError(t, err)
	id := uuid.MustParse(got.ID)
	inicio := c.ag.booking.Slot.StartsAt

	t.Run("cedo demais não entrega o link", func(t *testing.T) {
		_, err := c.store.JoinURL(ctx, id, c.conta.ID, inicio.Add(-31*time.Minute))
		require.ErrorIs(t, err, models.ErrJoinNotAllowed)

		var negado models.JoinDenied
		require.True(t, errors.As(err, &negado))
		require.Equal(t, scheduling.ReasonTooEarly, negado.Reason)
		require.True(t, negado.OpensAt.Equal(inicio.Add(-30*time.Minute)),
			"o motivo tem que dizer QUANDO abre — é o que a tela mostra no lugar do botão")
	})

	t.Run("na janela entrega", func(t *testing.T) {
		url, err := c.store.JoinURL(ctx, id, c.conta.ID, inicio.Add(-30*time.Minute))
		require.NoError(t, err)
		require.Equal(t, linkPaciente, url)
	})

	t.Run("depois do fim não entrega", func(t *testing.T) {
		_, err := c.store.JoinURL(ctx, id, c.conta.ID, inicio.Add(2*time.Hour))
		require.ErrorIs(t, err, models.ErrJoinNotAllowed)
	})

	t.Run("consulta de outro paciente é 404, não 403", func(t *testing.T) {
		outra := criaConta2(t, c.pool)
		_, err := c.store.JoinURL(ctx, id, outra.ID, inicio)
		require.ErrorIs(t, err, models.ErrAppointmentNotFound,
			"403 confirmaria que o id existe e a rota viraria oráculo de ids")
	})
}

func TestGetForAccount_NaoVazaConsultaDeTerceiro(t *testing.T) {
	c := novoCenario(t)
	ctx := context.Background()

	got, err := c.store.Book(ctx, c.entrada())
	require.NoError(t, err)

	outra := criaConta2(t, c.pool)
	_, err = c.store.GetForAccount(ctx, uuid.MustParse(got.ID), outra.ID, c.agora)
	require.ErrorIs(t, err, models.ErrAppointmentNotFound)
}

// ---------------------------------------------------------------------------
// Cancelamento
// ---------------------------------------------------------------------------

// bookaConfirmada roda a saga até CONFIRMED e devolve o id — o ponto de partida de
// todo teste de cancelamento.
func (c cenario) bookaConfirmada(t *testing.T) uuid.UUID {
	t.Helper()
	got, err := c.store.Book(context.Background(), c.entrada())
	require.NoError(t, err)
	require.Equal(t, "CONFIRMED", got.Status)
	require.True(t, c.ag.booked, "o horário tem que estar reservado antes de cancelar")
	return uuid.MustParse(got.ID)
}

// Caminho feliz: CANCELLED no Postgres, horário de volta ao mercado (slot_released_at
// preenchido), fakeAgenda liberou o slot e a DAV foi cancelada com o id certo.
func TestCancel_CaminhoFeliz(t *testing.T) {
	c := novoCenario(t)
	id := c.bookaConfirmada(t)

	res, err := c.store.Cancel(context.Background(), c.conta, id, c.agora)
	require.NoError(t, err)
	require.True(t, res.DAVCancelled, "a DAV cancelou")
	require.Empty(t, res.DAVError)
	require.Equal(t, slotID, res.SlotID)

	require.Equal(t, 1, c.dv.cancelCalls, "cancelou na DAV uma vez")
	require.Equal(t, davApptID, c.dv.cancelledID, "cancelou com o dav_appointment_id certo")
	require.True(t, c.ag.released, "o horário voltou ao mercado no legado")
	require.False(t, c.ag.booked)

	status, held, released := c.statusNoBanco(t, id.String())
	require.Equal(t, "CANCELLED", status)
	require.True(t, held)
	require.True(t, released, "slot_released_at preenchido")
}

// A DAV responde erro no cancel (é o 500 real da HML — achado #20). O cancelamento
// AINDA é sucesso: a consulta fica CANCELLED, o horário volta, e o erro tolerado
// vai no resultado (DAVCancelled=false, DAVError preenchido).
func TestCancel_DAVFalha_ToleraEDevolveSucesso(t *testing.T) {
	c := novoCenario(t)
	id := c.bookaConfirmada(t)
	c.dv.cancelErr = errors.New("dav: PUT /appointment/{id}/cancel respondeu 500 (trace 50295b5)")

	res, err := c.store.Cancel(context.Background(), c.conta, id, c.agora)
	require.NoError(t, err, "o cancel da DAV é best-effort — não derruba o cancelamento do paciente")
	require.False(t, res.DAVCancelled)
	require.NotEmpty(t, res.DAVError, "o erro tolerado fica registrado no resultado")
	require.Equal(t, 1, c.dv.cancelCalls, "escrita na DAV não repete")

	status, held, released := c.statusNoBanco(t, id.String())
	require.Equal(t, "CANCELLED", status)
	require.True(t, held)
	require.True(t, released, "o horário voltou mesmo com a DAV falhando")
	require.True(t, c.ag.released)
}

// Devolver o horário falha (fakeAgenda com erro injetado). O cancelamento continua
// sucesso: PG CANCELLED, mas slot_released_at NULO — a linha vira fila do worker,
// que tenta devolver de novo. NÃO desfazemos o CANCELLED.
func TestCancel_ReleaseSlotFalha_DeixaAFilaDoWorker(t *testing.T) {
	c := novoCenario(t)
	id := c.bookaConfirmada(t)
	c.ag.releaseErr = agenda.ErrUnavailable

	res, err := c.store.Cancel(context.Background(), c.conta, id, c.agora)
	require.NoError(t, err, "devolver o horário é best-effort")
	require.True(t, res.DAVCancelled, "a DAV ainda foi cancelada")

	status, held, released := c.statusNoBanco(t, id.String())
	require.Equal(t, "CANCELLED", status)
	require.True(t, held)
	require.False(t, released, "slot_released_at NULO: fica para o worker devolver")
}

// Cancelar consulta de OUTRA conta é 404, nunca 403: um 403 confirmaria que o id
// existe e a rota viraria oráculo de ids. E a consulta do dono fica intacta.
func TestCancel_DeOutraConta_EhNotFound(t *testing.T) {
	c := novoCenario(t)
	id := c.bookaConfirmada(t)

	outra := criaConta2(t, c.pool)
	_, err := c.store.Cancel(context.Background(), outra, id, c.agora)
	require.ErrorIs(t, err, models.ErrAppointmentNotFound)

	require.Zero(t, c.dv.cancelCalls, "consulta de outro nem toca na DAV")
	status, _, _ := c.statusNoBanco(t, id.String())
	require.Equal(t, "CONFIRMED", status, "a consulta do dono fica como estava")
}

// Uma consulta ainda em voo (PENDING_SLOT) não é do paciente cancelar — quem a
// resolve é a saga/worker. ErrCancelNotAllowed (o controller mapeia para 409).
func TestCancel_NaoConfirmada_EhCancelNotAllowed(t *testing.T) {
	c := novoCenario(t)
	id := c.criaPendente(t)

	_, err := c.store.Cancel(context.Background(), c.conta, id, c.agora)
	require.ErrorIs(t, err, models.ErrCancelNotAllowed)
	require.Zero(t, c.dv.cancelCalls, "nem toca na DAV")

	status, _, _ := c.statusNoBanco(t, id.String())
	require.Equal(t, "PENDING_SLOT", status, "a consulta em voo fica como estava")
}

// Cancelar duas vezes: a segunda recusa (já não está CONFIRMED). Cobre a corrida
// entre dois cancelamentos — o guard `status = 'CONFIRMED'` do UPDATE decide.
func TestCancel_DuasVezes_SegundaRecusa(t *testing.T) {
	c := novoCenario(t)
	id := c.bookaConfirmada(t)

	_, err := c.store.Cancel(context.Background(), c.conta, id, c.agora)
	require.NoError(t, err)

	_, err = c.store.Cancel(context.Background(), c.conta, id, c.agora)
	require.ErrorIs(t, err, models.ErrCancelNotAllowed, "já cancelada não cancela de novo")
	require.Equal(t, 1, c.dv.cancelCalls, "a DAV só foi chamada na primeira")
}

// criaPendente insere uma consulta em PENDING_SLOT direto no banco: é um estado
// intermediário da saga que o caminho feliz não deixa parado, então o montamos
// à mão para exercitar a recusa do cancelamento.
func (c cenario) criaPendente(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)
	_, err = c.pool.Exec(context.Background(), `
		INSERT INTO appointment (id, account_id, legacy_slot_id, legacy_professional_id,
		                         legacy_specialty_id, professional_name, specialty_name,
		                         starts_at, ends_at, status)
		VALUES ($1,$2,$3,$4,$5,'Ana Beatriz Moura','Psicologia',$6,$7,'PENDING_SLOT')`,
		id, c.conta.ID, slotID, profID, specID,
		c.ag.booking.Slot.StartsAt, c.ag.booking.Slot.EndsAt)
	require.NoError(t, err)
	return id
}
