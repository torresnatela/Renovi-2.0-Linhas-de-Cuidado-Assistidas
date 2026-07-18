package controllers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/models/scheduling"
)

var spLoc = mustLoc("America/Sao_Paulo")

func mustLoc(n string) *time.Location {
	l, err := time.LoadLocation(n)
	if err != nil {
		panic(err)
	}
	return l
}

var (
	agoraFixo = time.Date(2026, 7, 20, 8, 0, 0, 0, spLoc)
	inicioFix = time.Date(2026, 7, 20, 9, 0, 0, 0, spLoc)
	linkDaSal = "https://renovisaude.atendimento.hom.dav.med.br/a/sopr8brbkz"
)

// fakeBookings implementa a interface que o CONTROLLER declara (ADR-012). Struct
// à mão, sem framework de mock — é o padrão da casa.
type fakeBookings struct {
	specialties []agenda.Specialty
	specErr     error
	slots       []agenda.Slot
	appt        models.Appointment
	bookErr     error
	joinURL     string
	joinErr     error
	getErr      error
}

func (f *fakeBookings) ListSpecialties(context.Context, time.Time) ([]agenda.Specialty, error) {
	return f.specialties, f.specErr
}
func (f *fakeBookings) ListProfessionals(context.Context, string, time.Time) ([]agenda.Professional, error) {
	return nil, f.specErr
}
func (f *fakeBookings) ListSlotPage(context.Context, string, time.Time, time.Time, time.Time) (models.SlotPage, error) {
	if f.specErr != nil {
		return models.SlotPage{}, f.specErr
	}
	return models.SlotPage{
		Professional: agenda.Professional{ID: "prof-1", FullName: "Ana Beatriz Moura"},
		Slots:        f.slots,
	}, nil
}
func (f *fakeBookings) Book(context.Context, models.BookInput) (models.Appointment, error) {
	if f.bookErr != nil {
		return models.Appointment{}, f.bookErr
	}
	return f.appt, nil
}
func (f *fakeBookings) ListForAccount(context.Context, uuid.UUID, time.Time) ([]models.Appointment, error) {
	return []models.Appointment{f.appt}, nil
}
func (f *fakeBookings) GetForAccount(context.Context, uuid.UUID, uuid.UUID, time.Time) (models.Appointment, error) {
	if f.getErr != nil {
		return models.Appointment{}, f.getErr
	}
	return f.appt, nil
}
func (f *fakeBookings) JoinURL(context.Context, uuid.UUID, uuid.UUID, time.Time) (string, error) {
	return f.joinURL, f.joinErr
}
func (f *fakeBookings) Location() *time.Location { return spLoc }

func consultaConfirmada() models.Appointment {
	return models.Appointment{
		ID: uuid.NewString(), Status: "CONFIRMED",
		StartsAt: inicioFix, EndsAt: inicioFix.Add(25 * time.Minute),
		TimeZone:    "America/Sao_Paulo",
		SpecialtyID: "esp-1", SpecialtyName: "Psicologia",
		ProfessionalID: "prof-1", ProfessionalName: "Ana Beatriz Moura",
		CreatedAt: agoraFixo,
		Join: scheduling.Evaluate(scheduling.Policy{OpensBefore: 30 * time.Minute},
			scheduling.StateConfirmed, inicioFix, inicioFix.Add(25*time.Minute), agoraFixo),
	}
}

// serve monta o controller com uma conta na sessão e devolve a resposta.
func serve(t *testing.T, f *fakeBookings, method, rota, alvo, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	c := controllers.SchedulingController{Bookings: f, Now: func() time.Time { return agoraFixo }}

	r := chi.NewRouter()
	// Usa o RequireSession DE VERDADE, com um fake de sessões: injetar a conta no
	// contexto por fora testaria um caminho que não existe em produção — e é
	// justamente o middleware que garante que estas rotas têm dono.
	r.Use(controllers.RequireSession(&fakeSessions{
		validated: models.Account{ID: uuid.New(), FullName: "Maria de Teste"},
	}))
	r.Get("/specialties", c.ListSpecialties)
	r.Get("/professionals/{professional_id}/slots", c.ListSlots)
	r.Get("/appointments", c.List)
	r.Get("/appointments/{appointment_id}", c.Get)
	r.Post("/appointments", c.Create)
	r.Post("/appointments/{appointment_id}/join", c.Join)

	var body *strings.Reader
	if corpo == "" {
		body = strings.NewReader("")
	} else {
		body = strings.NewReader(corpo)
	}
	req := httptest.NewRequest(method, alvo, body)
	req.AddCookie(&http.Cookie{Name: controllers.SessionCookieName, Value: "token-de-teste"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// A regra que mais importa: o link não vaza
// ---------------------------------------------------------------------------

// Se o link viajasse na listagem, a janela dos 30 minutos viraria decoração:
// bastaria abrir o DevTools para entrar a qualquer hora, e o cache do cliente
// guardaria N links de teleconsulta. Ele só sai do /join.
func TestListEGet_NuncaDevolvemOLinkDaSala(t *testing.T) {
	f := &fakeBookings{appt: consultaConfirmada(), joinURL: linkDaSal}

	for _, alvo := range []string{"/appointments", "/appointments/" + f.appt.ID} {
		t.Run(alvo, func(t *testing.T) {
			w := serve(t, f, http.MethodGet, "", alvo, "")
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d", w.Code)
			}
			if strings.Contains(w.Body.String(), linkDaSal) {
				t.Errorf("o link da sala vazou no payload:\n%s", w.Body.String())
			}
			if strings.Contains(w.Body.String(), "join_url") {
				t.Error("o payload não pode ter campo de url")
			}
		})
	}
}

// O contrato não carrega "30 minutos": ele carrega a HORA em que abre, já
// calculada. É o que permite mudar a antecedência sem deploy do front.
func TestGet_LevaOEstadoDaJanelaComAHoraDeAbrir(t *testing.T) {
	f := &fakeBookings{appt: consultaConfirmada()}
	w := serve(t, f, http.MethodGet, "", "/appointments/"+f.appt.ID, "")

	var got struct {
		Join struct {
			Status  string `json:"status"`
			OpensAt string `json:"opens_at"`
		} `json:"join"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.Join.Status != "TOO_EARLY" {
		t.Errorf("status da janela = %q, quero TOO_EARLY (agora = 1h antes)", got.Join.Status)
	}
	if got.Join.OpensAt != inicioFix.Add(-30*time.Minute).Format(time.RFC3339) {
		t.Errorf("opens_at = %q, quero 08:30-03:00", got.Join.OpensAt)
	}
}

func TestJoin_ForaDaJanelaDevolve409ComMotivoLegivelPorMaquina(t *testing.T) {
	f := &fakeBookings{
		appt: consultaConfirmada(),
		joinErr: models.JoinDenied{
			Reason:  scheduling.ReasonTooEarly,
			OpensAt: inicioFix.Add(-30 * time.Minute),
		},
	}
	w := serve(t, f, http.MethodPost, "", "/appointments/"+f.appt.ID+"/join", "")

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, quero 409", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Errorf("content-type = %q, quero problem+json", ct)
	}
	var p struct {
		Reason struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"reason"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	// O front reage diferente a "cedo demais" e a "já terminou", e os dois são
	// 409. Casar pelo texto seria casar por frase escrita para humano.
	if p.Reason.Code != "JOIN_TOO_EARLY" {
		t.Errorf("reason.code = %q", p.Reason.Code)
	}
	if strings.Contains(w.Body.String(), linkDaSal) {
		t.Error("o 409 não pode conter o link")
	}
}

func TestJoin_NaJanelaDevolveOLinkESemCache(t *testing.T) {
	f := &fakeBookings{appt: consultaConfirmada(), joinURL: linkDaSal}
	w := serve(t, f, http.MethodPost, "", "/appointments/"+f.appt.ID+"/join", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.URL != linkDaSal {
		t.Errorf("url = %q", got.URL)
	}
	// O link é credencial: não pode ficar em cache de proxy nenhum do caminho.
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, quero no-store", cc)
	}
}

// ---------------------------------------------------------------------------
// Erros do agendamento
// ---------------------------------------------------------------------------

func TestCreate_TraduzOsErrosDoModel(t *testing.T) {
	casos := []struct {
		nome       string
		err        error
		wantStatus int
		wantReason string
	}{
		{"horário tomado", models.ErrSlotTaken, http.StatusConflict, "SLOT_TAKEN"},
		{"horário no passado", models.ErrSlotExpired, http.StatusConflict, "SLOT_EXPIRED"},
		{"horário inexistente", models.ErrSlotNotFound, http.StatusNotFound, ""},
		{"especialidade errada", models.ErrSpecialtyMismatch, http.StatusBadRequest, ""},
		{"especialidade desativada", models.ErrSpecialtyInactive, http.StatusBadRequest, ""},
		{"conta sem vínculo", models.ErrAccountNotLinked, http.StatusForbidden, ""},
		// 422 e não 502: a DAV recusou os dados, nada foi criado, o horário voltou.
		{"DAV recusou os dados", models.ErrBookingRejected, http.StatusUnprocessableEntity, "BOOKING_REJECTED"},
		// 502 e não 500: o problema é da DAV, e o resultado é desconhecido.
		{"DAV não confirmou", models.ErrBookingUnconfirmed, http.StatusBadGateway, "BOOKING_UNCONFIRMED"},
		{"agenda fora do ar", agenda.ErrUnavailable, http.StatusServiceUnavailable, ""},
	}

	for _, tt := range casos {
		t.Run(tt.nome, func(t *testing.T) {
			f := &fakeBookings{bookErr: tt.err}
			w := serve(t, f, http.MethodPost, "", "/appointments",
				`{"slot_id":"s1","specialty_id":"e1"}`)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, quero %d (corpo: %s)", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantReason != "" {
				var p struct {
					Reason struct {
						Code string `json:"code"`
					} `json:"reason"`
				}
				_ = json.Unmarshal(w.Body.Bytes(), &p)
				if p.Reason.Code != tt.wantReason {
					t.Errorf("reason.code = %q, quero %q", p.Reason.Code, tt.wantReason)
				}
			}
		})
	}
}

// O 502 tem que dizer que a consulta PODE existir. "Falhou" mandaria o paciente
// tentar de novo — e repetir criaria uma segunda consulta de verdade, porque a
// DAV não aceita id nosso e não dá para reconciliar.
func TestCreate_O502NaoDizQueFalhou(t *testing.T) {
	f := &fakeBookings{bookErr: models.ErrBookingUnconfirmed}
	w := serve(t, f, http.MethodPost, "", "/appointments", `{"slot_id":"s1","specialty_id":"e1"}`)

	corpo := strings.ToLower(w.Body.String())
	if !strings.Contains(corpo, "pode ter sido marcada") {
		t.Errorf("o 502 precisa admitir a incerteza; veio: %s", w.Body.String())
	}
	if strings.Contains(corpo, "não foi marcada") || strings.Contains(corpo, "tente novamente") {
		t.Errorf("o 502 NÃO pode afirmar falha nem convidar a repetir; veio: %s", w.Body.String())
	}
}

func TestCreate_ExigeHorarioEEspecialidade(t *testing.T) {
	f := &fakeBookings{appt: consultaConfirmada()}
	for _, corpo := range []string{`{}`, `{"slot_id":"s1"}`, `{"specialty_id":"e1"}`} {
		w := serve(t, f, http.MethodPost, "", "/appointments", corpo)
		if w.Code != http.StatusBadRequest {
			t.Errorf("corpo %s: status = %d, quero 400", corpo, w.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// Catálogo
// ---------------------------------------------------------------------------

// "Não há especialidades" e "não conseguimos ler as especialidades" são coisas
// diferentes: confundi-las faz o paciente desistir de algo que estava disponível.
func TestListSpecialties_ListaVaziaNaoEhErro(t *testing.T) {
	w := serve(t, &fakeBookings{specialties: nil}, http.MethodGet, "", "/specialties", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"items":[]}` {
		t.Errorf("corpo = %s, quero items vazio (e não null)", got)
	}
}

func TestListSpecialties_LegadoForaDoArEh503(t *testing.T) {
	w := serve(t, &fakeBookings{specErr: agenda.ErrUnavailable}, http.MethodGet, "", "/specialties", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, quero 503 (não 200 com lista vazia)", w.Code)
	}
}

// O intervalo default sai do servidor porque "hoje" só existe dentro de um fuso —
// e quem conhece o fuso da agenda é o servidor.
func TestListSlots_EcoaOIntervaloQueUsou(t *testing.T) {
	w := serve(t, &fakeBookings{}, http.MethodGet, "", "/professionals/p1/slots", "")

	var got struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.From != "2026-07-20" {
		t.Errorf("from = %q, quero o hoje do fuso da agenda", got.From)
	}
	if got.To != "2026-08-19" {
		t.Errorf("to = %q, quero hoje+30d", got.To)
	}
}

// Este teste existe porque o bug aconteceu: o contrato declarava
// `SlotPage.professional` obrigatório e o DTO simplesmente não tinha o campo. Os
// testes do front passavam (o mock deles devolvia o profissional) e a API real
// mandava a página sem ele — a tela quebrava em branco com "Cannot read
// properties of undefined". Só rodar de verdade pegou.
func TestListSlots_LevaOProfissionalJunto(t *testing.T) {
	w := serve(t, &fakeBookings{}, http.MethodGet, "", "/professionals/prof-1/slots", "")

	var got struct {
		Professional struct {
			ID       string `json:"id"`
			FullName string `json:"full_name"`
		} `json:"professional"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	// A tela é "os horários da Ana": sem isto o front não sabe de quem está
	// mostrando a agenda.
	if got.Professional.FullName != "Ana Beatriz Moura" {
		t.Errorf("professional.full_name = %q, quero o nome de quem tem a agenda", got.Professional.FullName)
	}
}

func TestListSlots_RecusaIntervaloAbsurdo(t *testing.T) {
	w := serve(t, &fakeBookings{}, http.MethodGet, "",
		"/professionals/p1/slots?from=2026-01-01&to=2027-01-01", "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, quero 400: um ano de agenda é pedido demais ao MySQL de terceiro", w.Code)
	}
}

func TestListSlots_RecusaDataMalformada(t *testing.T) {
	w := serve(t, &fakeBookings{}, http.MethodGet, "", "/professionals/p1/slots?from=ontem", "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, quero 400", w.Code)
	}
}

// A borda do teto de 60 dias, que antes tinha off-by-one (from..to a 60 dias
// consultava 61). A janela real é [from, to+1d): 59 dias de distância = 60 de
// janela (ok); 60 de distância = 61 de janela (recusa).
func TestListSlots_LimiteDe60Dias(t *testing.T) {
	// from=01/01, to=01/03 → 59 dias de distância, janela de 60 → passa.
	ok := serve(t, &fakeBookings{}, http.MethodGet, "",
		"/professionals/p1/slots?from=2026-01-01&to=2026-03-01", "")
	if ok.Code != http.StatusOK {
		t.Errorf("59 dias de distância (janela de 60) devia passar; veio %d", ok.Code)
	}
	// from=01/01, to=02/03 → 60 dias de distância, janela de 61 → recusa.
	nok := serve(t, &fakeBookings{}, http.MethodGet, "",
		"/professionals/p1/slots?from=2026-01-01&to=2026-03-02", "")
	if nok.Code != http.StatusBadRequest {
		t.Errorf("60 dias de distância (janela de 61) devia recusar; veio %d", nok.Code)
	}
}

func TestGet_ConsultaDeTerceiroEh404(t *testing.T) {
	f := &fakeBookings{getErr: models.ErrAppointmentNotFound}
	w := serve(t, f, http.MethodGet, "", "/appointments/"+uuid.NewString(), "")
	// 403 confirmaria que o id existe e a rota viraria oráculo de ids válidos.
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, quero 404 (nunca 403)", w.Code)
	}
}

func TestGet_IdMalformadoTambemEh404(t *testing.T) {
	w := serve(t, &fakeBookings{appt: consultaConfirmada()}, http.MethodGet, "", "/appointments/nao-e-uuid", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, quero 404: um 400 diria 'este formato existe'", w.Code)
	}
}

// Falha de infra NÃO pode virar 404. "Sua consulta não existe" quando o banco
// caiu manda o paciente embora de algo que está lá. 404 é só para
// ErrAppointmentNotFound; o resto é 500.
func TestGet_FalhaDeInfraEh500NaoEh404(t *testing.T) {
	f := &fakeBookings{getErr: errors.New("banco fora do ar")}
	w := serve(t, f, http.MethodGet, "", "/appointments/"+uuid.NewString(), "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, quero 500 (não 404)", w.Code)
	}
}
