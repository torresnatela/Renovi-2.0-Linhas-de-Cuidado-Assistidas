//go:build integration

package agenda_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/testsupport"
)

// legacy é o container compartilhado por TODOS os testes deste arquivo.
//
// Um MySQL leva dezenas de segundos para subir; um por teste faria esta bateria
// passar de dez minutos e ninguém rodaria `make test-integration`. Compartilhar é
// seguro aqui porque cada teste semeia o SEU próprio slot, com id próprio, e
// nunca mexe no do vizinho.
var legacy testsupport.LegacyMySQL

func TestMain(m *testing.M) {
	l, stop, err := testsupport.StartMySQLShared(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "não consegui subir o MySQL legado: %v\n", err)
		os.Exit(1)
	}
	legacy = l

	code := m.Run()

	// Sem defer: os.Exit não roda defer.
	_ = stop()
	os.Exit(code)
}

// Ids que vêm do deploy/mysql-legacy/init.sql (o mesmo mock do `make up`).
const (
	psicologia   = "11111111-1111-4111-8111-111111111111"
	psiquiatria  = "22222222-2222-4222-8222-222222222222"
	desativada   = "33333333-3333-4333-8333-333333333333"
	ana          = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" // Psicologia
	bruno        = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" // Psiquiatria
	turnoDaAna   = "a1a1a1a1-0000-4000-8000-000000000001"
	slotDoLegado = "50100000-0000-4000-8000-000000000003" // já booked pelo app legado
)

var saoPaulo = mustLoad("America/Sao_Paulo")

func mustLoad(n string) *time.Location {
	l, err := time.LoadLocation(n)
	if err != nil {
		panic(err)
	}
	return l
}

// suite liga o adapter (usuário restrito) e o root (para montar cenário) ao
// container compartilhado.
type suite struct {
	client *agenda.Client
	root   *sql.DB
}

func newSuite(t *testing.T) suite {
	t.Helper()

	client, err := agenda.New(agenda.Config{DSN: legacy.AppDSN})
	require.NoError(t, err, "abrir o adapter")
	t.Cleanup(func() { _ = client.Close() })

	root, err := sql.Open("mysql", legacy.RootDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	require.NoError(t, root.Ping(), "o container tem que estar de pé")

	return suite{client: client, root: root}
}

// seedSlot cria um horário com hora de parede EXATA, para o teste não depender do
// CURDATE() do container. Roda como root porque o usuário da aplicação não pode
// inserir — que é justamente o que TestPosturaDeEscrita prova.
func (s suite) seedSlot(t *testing.T, id, shiftID string, wall time.Time, booked bool) {
	t.Helper()
	_, err := s.root.Exec(
		`INSERT INTO tb_slots (id, shiftId, booked, startsAt, endsAt) VALUES (?, ?, ?, ?, ?)`,
		id, shiftID, booked, wall.Format("2006-01-02 15:04:05"), wall.Add(25*time.Minute).Format("2006-01-02 15:04:05"),
	)
	require.NoError(t, err, "semear slot")
}

func (s suite) booked(t *testing.T, slotID string) bool {
	t.Helper()
	var b bool
	require.NoError(t, s.root.QueryRow(`SELECT booked FROM tb_slots WHERE id = ?`, slotID).Scan(&b))
	return b
}

// ---------------------------------------------------------------------------
// O teste que justifica o desenho inteiro
// ---------------------------------------------------------------------------

// Não há unique nem FK protegendo o slot no schema real: `booked` é um flag solto
// e a DAV aceita dois appointments no mesmo horário para o mesmo profissional
// (achado #17). Ou seja, este CAS é a ÚNICA trava de double-booking que existe no
// sistema. Se ele deixar dois vencedores passarem, dois pacientes vão para a
// mesma consulta.
func TestBookSlot_SoUmVencedorSobConcorrencia(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	const slot = "aaaa0000-0000-4000-8000-00000000c0de"
	s.seedSlot(t, slot, turnoDaAna, time.Date(2030, 3, 4, 9, 0, 0, 0, saoPaulo), false)

	const disputantes = 12
	var (
		wg       sync.WaitGroup
		largada  = make(chan struct{})
		mu       sync.Mutex
		vitorias int
		outros   []error
	)

	for i := 0; i < disputantes; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-largada // todos batem no mesmo instante; sem isto viram 12 chamadas em fila
			err := s.client.BookSlot(ctx, slot)

			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				vitorias++
			case errors.Is(err, agenda.ErrSlotTaken):
				// esperado: perdeu a corrida
			default:
				outros = append(outros, err)
			}
		}()
	}
	close(largada)
	wg.Wait()

	require.Empty(t, outros, "ninguém pode falhar por outro motivo que não ErrSlotTaken")
	require.Equal(t, 1, vitorias,
		"EXATAMENTE um pode reservar. Mais de um = double-booking: dois pacientes na mesma consulta.")
	require.True(t, s.booked(t, slot), "o slot tem que ficar reservado no legado")
}

// O app legado escreve neste banco o tempo todo. Reservar por cima da reserva
// dele é o mesmo double-booking, só que com o médico descobrindo na hora.
func TestBookSlot_RespeitaReservaDoAppLegado(t *testing.T) {
	s := newSuite(t)

	err := s.client.BookSlot(context.Background(), slotDoLegado)
	require.ErrorIs(t, err, agenda.ErrSlotTaken)
}

func TestBookSlot_SlotInexistenteNaoEhSlotTomado(t *testing.T) {
	s := newSuite(t)

	// A diferença importa: "sumiu" é 404, "tomado" é "escolha outro horário".
	err := s.client.BookSlot(context.Background(), "nao-existe")
	require.ErrorIs(t, err, agenda.ErrSlotNotFound)
}

// A compensação roda num worker que pode repetir depois de um crash. Se soltar um
// slot já solto virasse erro, o worker repetiria para sempre.
func TestReleaseSlot_EhIdempotente(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	const slot = "aaaa0000-0000-4000-8000-00000000fee1"
	s.seedSlot(t, slot, turnoDaAna, time.Date(2030, 3, 5, 9, 0, 0, 0, saoPaulo), false)

	require.NoError(t, s.client.BookSlot(ctx, slot))
	require.NoError(t, s.client.ReleaseSlot(ctx, slot), "primeira devolução")
	require.False(t, s.booked(t, slot))
	require.NoError(t, s.client.ReleaseSlot(ctx, slot), "devolver de novo é sucesso, não erro")
	require.NoError(t, s.client.ReleaseSlot(ctx, "nao-existe"), "soltar o que não existe também")
}

// ---------------------------------------------------------------------------
// Fuso — o bug mais provável desta feature
// ---------------------------------------------------------------------------

// O legado grava DATETIME ingênuo. Se o adapter ler no fuso errado, a consulta
// acontece 3h fora do horário: sem erro, sem aviso, só errado. E o DSN do
// .env.example não pede parseTime nem loc — é o adapter que precisa forçar.
func TestListSlots_LeDatetimeIngenuoComoHoraDeSaoPaulo(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	const slot = "aaaa0000-0000-4000-8000-0000000fu50"
	// 09:00 gravado no banco significa 09:00 em São Paulo.
	parede := time.Date(2030, 3, 6, 9, 0, 0, 0, saoPaulo)
	s.seedSlot(t, slot, turnoDaAna, parede, false)

	slots, err := s.client.ListSlots(ctx,
		ana,
		time.Date(2030, 3, 6, 0, 0, 0, 0, saoPaulo),
		time.Date(2030, 3, 7, 0, 0, 0, 0, saoPaulo),
		time.Date(2030, 3, 1, 0, 0, 0, 0, saoPaulo),
	)
	require.NoError(t, err)
	require.Len(t, slots, 1)

	got := slots[0]
	require.True(t, got.StartsAt.Equal(parede),
		"09:00 no legado tem que virar o INSTANTE 09:00-03:00, não 09:00Z (que seria 06:00 em SP). Veio %s", got.StartsAt)
	require.Equal(t, 9, got.StartsAt.In(saoPaulo).Hour(), "hora de parede em SP")
	// O mesmo instante em UTC é meio-dia: se isto der 09:00, lemos como UTC.
	require.Equal(t, 12, got.StartsAt.UTC().Hour(), "o mesmo instante em UTC")
}

// Sem parseTime o driver nem consegue ler DATETIME; com loc=UTC ele lê 3h errado.
// O adapter força os dois — este teste prova que ele não confia no DSN.
func TestNew_ForcaFusoMesmoComDSNCru(t *testing.T) {
	// AppDSN não tem parseTime nem loc de propósito.
	c, err := agenda.New(agenda.Config{DSN: legacy.AppDSN})
	require.NoError(t, err)
	defer c.Close()

	require.Equal(t, "America/Sao_Paulo", c.Location().String())
	require.NoError(t, c.Ping(context.Background()))
}

func TestNew_RecusaDSNQuePedeOutroFuso(t *testing.T) {
	// Errar o fuso em silêncio é pior que não subir.
	_, err := agenda.New(agenda.Config{
		DSN: "renovi:renovi@tcp(localhost:3306)/renovi_legacy?parseTime=true&loc=America%2FNew_York",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "America/Sao_Paulo")
}

// ---------------------------------------------------------------------------
// Leitura
// ---------------------------------------------------------------------------

func TestListSlots_SoOfereceLivreEFuturo(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	dia := time.Date(2030, 4, 10, 0, 0, 0, 0, saoPaulo)
	s.seedSlot(t, "bbbb0000-0000-4000-8000-000000000001", turnoDaAna, dia.Add(9*time.Hour), false)  // livre, futuro
	s.seedSlot(t, "bbbb0000-0000-4000-8000-000000000002", turnoDaAna, dia.Add(10*time.Hour), true)  // ocupado
	s.seedSlot(t, "bbbb0000-0000-4000-8000-000000000003", turnoDaAna, dia.Add(-2*time.Hour), false) // livre, mas passado

	// "agora" = 08:00 do dia. O slot das 22:00 da véspera fica para trás.
	agora := dia.Add(8 * time.Hour)
	slots, err := s.client.ListSlots(ctx, ana, dia.Add(-24*time.Hour), dia.Add(24*time.Hour), agora)
	require.NoError(t, err)

	ids := make([]string, 0, len(slots))
	for _, s := range slots {
		ids = append(ids, s.ID)
	}
	require.Contains(t, ids, "bbbb0000-0000-4000-8000-000000000001")
	require.NotContains(t, ids, "bbbb0000-0000-4000-8000-000000000002", "ocupado não serve para nada")
	require.NotContains(t, ids, "bbbb0000-0000-4000-8000-000000000003", "a DAV recusa início no passado (422)")
}

func TestListSpecialties_SoAtivasEComHorarioLivre(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	s.seedSlot(t, "cccc0000-0000-4000-8000-000000000001", turnoDaAna,
		time.Date(2030, 5, 2, 9, 0, 0, 0, saoPaulo), false)

	got, err := s.client.ListSpecialties(ctx, time.Date(2030, 5, 1, 0, 0, 0, 0, saoPaulo))
	require.NoError(t, err)

	nomes := map[string]bool{}
	for _, e := range got {
		nomes[e.ID] = true
	}
	require.True(t, nomes[psicologia], "a Ana tem horário livre em Psicologia")
	require.False(t, nomes[desativada], "especialidade inativa nunca aparece")
	require.False(t, nomes[psiquiatria],
		"o Bruno não tem horário livre neste intervalo: oferecer levaria a uma lista vazia de profissionais")
}

func TestListProfessionals_TrazRegistroNoConselho(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	s.seedSlot(t, "dddd0000-0000-4000-8000-000000000001", turnoDaAna,
		time.Date(2030, 6, 3, 9, 0, 0, 0, saoPaulo), false)

	got, err := s.client.ListProfessionalsBySpecialty(ctx, psicologia, time.Date(2030, 6, 1, 0, 0, 0, 0, saoPaulo))
	require.NoError(t, err)
	require.Len(t, got, 1)

	p := got[0]
	require.Equal(t, ana, p.ID)
	require.Equal(t, "Ana Beatriz Moura", p.FullName, "firstName + lastName já juntos")
	require.Equal(t, "CRP", p.LicenseCouncil)
	require.Equal(t, "SP", p.LicenseRegion)
	require.Empty(t, p.RQE, "NULL no banco vira vazio, não panic")
	require.Empty(t, p.ImageURL)
}

// ---------------------------------------------------------------------------
// LoadBooking
// ---------------------------------------------------------------------------

func TestLoadBooking_ResolveSlotProfissionalEEspecialidade(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	const slot = "eeee0000-0000-4000-8000-000000000001"
	parede := time.Date(2030, 7, 8, 9, 0, 0, 0, saoPaulo)
	s.seedSlot(t, slot, turnoDaAna, parede, false)

	b, err := s.client.LoadBooking(ctx, slot, psicologia)
	require.NoError(t, err)

	require.Equal(t, slot, b.Slot.ID)
	require.True(t, b.Slot.StartsAt.Equal(parede))
	require.Equal(t, ana, b.Professional.ID, "é este id que vira o participante MMD na DAV")
	require.Equal(t, "Ana Beatriz Moura", b.Professional.FullName)
	require.Equal(t, "Psicologia", b.Specialty.Name)
	require.False(t, b.Booked)
}

// O vínculo profissional-especialidade é muitos-para-muitos: pedir a
// especialidade errada tem que ser 400 ("escolha de novo"), e não 404 ("sumiu").
func TestLoadBooking_EspecialidadeQueOProfissionalNaoAtende(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	const slot = "eeee0000-0000-4000-8000-000000000002"
	s.seedSlot(t, slot, turnoDaAna, time.Date(2030, 7, 9, 9, 0, 0, 0, saoPaulo), false)

	_, err := s.client.LoadBooking(ctx, slot, psiquiatria) // a Ana é psicóloga
	require.ErrorIs(t, err, agenda.ErrSpecialtyMismatch)
}

func TestLoadBooking_SlotInexistente(t *testing.T) {
	s := newSuite(t)

	_, err := s.client.LoadBooking(context.Background(), "nao-existe", psicologia)
	require.ErrorIs(t, err, agenda.ErrSlotNotFound)
}

// ---------------------------------------------------------------------------
// A postura de escrita (ADR-004), provada e não prometida
// ---------------------------------------------------------------------------

// O init.sql rebaixa o usuário da aplicação. Este teste é o que impede alguém de
// "só desta vez" escrever no banco de terceiro: se a permissão for afrouxada, ele
// quebra.
func TestPosturaDeEscrita_SoBookedEmTbSlots(t *testing.T) {
	app, err := sql.Open("mysql", legacy.AppDSN)
	require.NoError(t, err)
	defer app.Close()

	_, err = app.Exec(`INSERT INTO tb_appointments (id,userId,professionalId,title,status,startsAt,endsAt)
	                   VALUES ('x','y',?,'t','SCHEDULED',NOW(),NOW())`, ana)
	require.Error(t, err, "a consulta vive no NOSSO Postgres — o legado não pode receber INSERT nosso")

	_, err = app.Exec(`UPDATE tb_slots SET startsAt = NOW() WHERE id = ?`, slotDoLegado)
	require.Error(t, err, "só booked e updatedAt são nossos")

	_, err = app.Exec(`DELETE FROM tb_slots WHERE id = ?`, slotDoLegado)
	require.Error(t, err, "nunca apagamos nada do legado")
}

// ---------------------------------------------------------------------------
// Deriva de schema
// ---------------------------------------------------------------------------

// deploy/mysql-legacy/init.sql é uma CÓPIA de um banco que não é nosso: se o
// legado mudar, ninguém nos avisa e todo teste continua verde. Este teste não
// resolve isso — mas enumera explicitamente as colunas de que dependemos, para
// que apontá-lo para uma réplica real (num job noturno) seja trivial e para que a
// lista não viva só espalhada dentro das queries.
func TestSchema_ColunasDeQueDependemos(t *testing.T) {
	s := newSuite(t)

	requerido := map[string][]string{
		"tb_specialities":               {"id", "name", "active"},
		"tb_professionals":              {"id", "firstName", "lastName", "imageUrl", "licenseNumber", "licenseRegion", "licenseCouncil", "rqe"},
		"tb_professionals_specialities": {"professionalId", "specialityId"},
		"tb_shifts":                     {"id", "professionalId", "startsAt", "endsAt"},
		"tb_slots":                      {"id", "shiftId", "booked", "startsAt", "endsAt", "updatedAt"},
	}

	for tabela, colunas := range requerido {
		for _, coluna := range colunas {
			var n int
			err := s.root.QueryRow(`
				SELECT COUNT(*) FROM information_schema.columns
				WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
				tabela, coluna).Scan(&n)
			require.NoError(t, err)
			require.Equal(t, 1, n, "o adapter depende de %s.%s", tabela, coluna)
		}
	}
}
