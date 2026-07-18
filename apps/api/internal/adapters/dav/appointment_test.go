package dav_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/renovisaude/renovi-care/internal/adapters/dav"
)

// Payloads REAIS, capturados por `make dav-probe` contra a homologação e
// registrados em docs/DAV-API-NOTAS.md (achados #12, #13, #14, #18). Não invente
// payload aqui: o spec deles se contradiz, e testar contra o spec é testar contra
// a ficção.
const (
	// Achados #13/#14: 201 com a url de cada participante. Repare que a resposta
	// traz `role`, embora o ParticipantResponseSchema declare só {id, url}.
	respostaCriacao = `{
  "id": "13cd147e-68a7-45da-a65b-80b826cf674a",
  "participants": [
    {
      "id": "019f6ceb-1ff2-7616-af46-7574a621ac28",
      "role": "PAT",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/sopr8brbkz"
    },
    {
      "id": "fcde8cbf-f54a-429d-9e6d-078848587100",
      "role": "MMD",
      "url": "https://renovisaude.atendimento.hom.dav.med.br/a/xucz7cx8cc"
    }
  ]
}`

	// Achado #12: o que a DAV responde se mandarmos um id nosso.
	respostaIDRecusado = `{
  "code": 400,
  "message": "Bad Request Exception",
  "trace": "0699ae63fd701a2003ca94b2f6eda92c6d2cd7a3",
  "detail": [{"message": "property id should not exist"}]
}`
)

const (
	pacienteDAV     = "019f6ceb-1ff2-7616-af46-7574a621ac28" // o nosso UUIDv7 lá
	profissionalDAV = "fcde8cbf-f54a-429d-9e6d-078848587100" // = tb_professionals.id
	apptDAV         = "13cd147e-68a7-45da-a65b-80b826cf674a" // o id da consulta que a DAV devolve
	urlDoPaciente   = "https://renovisaude.atendimento.hom.dav.med.br/a/sopr8brbkz"
	urlDoMedico     = "https://renovisaude.atendimento.hom.dav.med.br/a/xucz7cx8cc"
)

var saoPaulo = mustLoadTZ("America/Sao_Paulo")

func mustLoadTZ(n string) *time.Location {
	l, err := time.LoadLocation(n)
	if err != nil {
		panic(err)
	}
	return l
}

func entrada() dav.CreateAppointmentInput {
	return dav.CreateAppointmentInput{
		Title:          "Consulta de Psicologia",
		StartsAt:       time.Date(2026, 7, 20, 9, 0, 0, 0, saoPaulo),
		EndsAt:         time.Date(2026, 7, 20, 9, 25, 0, 0, saoPaulo),
		Specialty:      "Psicologia",
		ProfessionalID: profissionalDAV,
		PatientID:      pacienteDAV,
	}
}

// clientPara sobe um servidor de teste e devolve o Client apontado para ele.
// Reusa o newClient de client_test.go (mesmo pacote), que já cala o logger.
func clientPara(t *testing.T, h http.HandlerFunc) *dav.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return newClient(t, srv, nil)
}

func isMaybeApplied(err error) bool { return errors.Is(err, dav.ErrMaybeApplied) }
func isValidation(err error) bool   { return errors.Is(err, dav.ErrValidation) }

func TestCreateAppointment_DevolveOLinkDoPaciente(t *testing.T) {
	c := clientPara(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, respostaCriacao)
	})

	got, err := c.CreateAppointment(context.Background(), entrada())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got.ID != "13cd147e-68a7-45da-a65b-80b826cf674a" {
		t.Errorf("ID = %q", got.ID)
	}
	// A url do PACIENTE, não a do médico. Trocar as duas mandaria o paciente para
	// a sala com os controles do profissional.
	if got.PatientJoinURL != urlDoPaciente {
		t.Errorf("PatientJoinURL = %q, quero a do PAT (%q)", got.PatientJoinURL, urlDoPaciente)
	}
	if got.PatientJoinURL == urlDoMedico {
		t.Error("pegou a url do MMD — os participantes foram trocados")
	}
}

// O corpo é o que a sondagem provou ser aceito, e cada campo aqui tem um achado
// atrás.
func TestCreateAppointment_MontaOCorpoQueADAVAceita(t *testing.T) {
	var enviado map[string]any
	c := clientPara(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&enviado)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, respostaCriacao)
	})

	if _, err := c.CreateAppointment(context.Background(), entrada()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	// Achado #12: mandar `id` derruba a criação com 400.
	if _, tem := enviado["id"]; tem {
		t.Error("o corpo NÃO pode levar `id`: a DAV responde 400 'property id should not exist'")
	}
	// Achado #16: ela respeita o offset, então mandamos o horário como o negócio o
	// enxerga em vez de converter para UTC.
	if got := enviado["start_date_time"]; got != "2026-07-20T09:00:00-03:00" {
		t.Errorf("start_date_time = %v, quero RFC3339 com offset de São Paulo", got)
	}
	if got := enviado["appointment_reason"]; got != "elective" {
		t.Errorf("appointment_reason = %v, quero o único valor do enum", got)
	}

	ps, ok := enviado["participants"].([]any)
	if !ok || len(ps) != 2 {
		t.Fatalf("participants = %v, quero exatamente 2 (o mínimo da DAV)", enviado["participants"])
	}
	papeis := map[string]string{}
	for _, p := range ps {
		m := p.(map[string]any)
		papeis[m["role"].(string)] = m["id"].(string)
		// Achado #13: o spec diz que `url` é obrigatória no request. É mentira — e
		// seria impossível, porque a url é o que eles geram.
		if _, tem := m["url"]; tem {
			t.Error("participante do REQUEST não leva `url`, apesar do que o spec diz")
		}
	}
	if papeis["MMD"] != profissionalDAV {
		t.Errorf("MMD = %q, quero o tb_professionals.id", papeis["MMD"])
	}
	if papeis["PAT"] != pacienteDAV {
		t.Errorf("PAT = %q, quero o dav_person_id do paciente", papeis["PAT"])
	}
}

// A regra do ADR-011b, agora sem rede de proteção: aqui não há sonda possível.
func TestCreateAppointment_NuncaRetenta(t *testing.T) {
	casos := []struct {
		nome   string
		status int
	}{
		{"504 do gateway", http.StatusGatewayTimeout},
		{"500", http.StatusInternalServerError},
	}

	for _, tt := range casos {
		t.Run(tt.nome, func(t *testing.T) {
			var chamadas int
			c := clientPara(t, func(w http.ResponseWriter, r *http.Request) {
				chamadas++
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, `{"code":500,"message":"erro","trace":"abc"}`)
			})

			_, err := c.CreateAppointment(context.Background(), entrada())
			if !isMaybeApplied(err) {
				t.Errorf("erro = %v, quero ErrMaybeApplied — 5xx numa escrita é DESCONHECIDO, não falha", err)
			}
			if chamadas != 1 {
				t.Errorf("chamou %d vezes; escrita na DAV NUNCA repete: a consulta pode ter sido criada "+
					"e repetir criaria uma segunda de verdade (a DAV aceita duas no mesmo horário)", chamadas)
			}
		})
	}
}

// 201 sem url do paciente é o caso mais traiçoeiro: a consulta EXISTE lá.
// Devolver ErrUnavailable faria o chamador concluir "falhou" e soltar o horário —
// e aí outro paciente marca por cima de uma consulta real.
func TestCreateAppointment_CriouMasSemURLEhDesconhecidoNaoFalha(t *testing.T) {
	c := clientPara(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		// Participante existe, mas com OUTRO id: não achamos a url do nosso paciente.
		_, _ = io.WriteString(w, `{"id":"abc","participants":[{"id":"outro-alguem","url":"https://x/a/1"}]}`)
	})

	_, err := c.CreateAppointment(context.Background(), entrada())
	if !isMaybeApplied(err) {
		t.Errorf("erro = %v, quero ErrMaybeApplied", err)
	}
}

func TestCreateAppointment_2xxSemIDEhDesconhecido(t *testing.T) {
	c := clientPara(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"participants":[]}`)
	})

	_, err := c.CreateAppointment(context.Background(), entrada())
	if !isMaybeApplied(err) {
		t.Errorf("erro = %v, quero ErrMaybeApplied", err)
	}
}

// 4xx é opinião firme da DAV sobre o payload: não houve efeito, e o horário pode
// voltar ao mercado com segurança.
func TestCreateAppointment_4xxEhRecusaDefinitiva(t *testing.T) {
	casos := []struct {
		nome   string
		status int
		corpo  string
	}{
		{"id recusado (achado #12)", http.StatusBadRequest, respostaIDRecusado},
		{"início no passado (achado #18)", http.StatusUnprocessableEntity,
			`{"code":422,"message":"Appointment invalid","trace":"xyz"}`},
	}

	for _, tt := range casos {
		t.Run(tt.nome, func(t *testing.T) {
			var chamadas int
			c := clientPara(t, func(w http.ResponseWriter, r *http.Request) {
				chamadas++
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.corpo)
			})

			_, err := c.CreateAppointment(context.Background(), entrada())
			if !isValidation(err) {
				t.Errorf("erro = %v, quero ErrValidation", err)
			}
			if isMaybeApplied(err) {
				t.Error("4xx NÃO pode virar ErrMaybeApplied: a DAV disse que não fez, e segurar o " +
					"horário por causa disso vazaria horário à toa")
			}
			if chamadas != 1 {
				t.Errorf("chamou %d vezes; 4xx não se repete", chamadas)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CancelAppointment — best-effort (ADR-011b, achado #20)
// ---------------------------------------------------------------------------

// Caminho feliz: 2xx vira nil, e o adapter bate no PUT certo. Confere método E
// path porque trocar qualquer um cancelaria a consulta errada — ou nenhuma.
func TestCancelAppointment_2xxCancela(t *testing.T) {
	casos := []struct {
		nome   string
		status int
	}{
		{"204 sem corpo", http.StatusNoContent},
		{"200 com corpo", http.StatusOK},
	}
	for _, tt := range casos {
		t.Run(tt.nome, func(t *testing.T) {
			var gotMethod, gotPath, gotKey string
			c := clientPara(t, func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath, gotKey = r.Method, r.URL.Path, r.Header.Get("x-api-key")
				w.WriteHeader(tt.status)
			})

			if err := c.CancelAppointment(context.Background(), apptDAV); err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if gotMethod != http.MethodPut {
				t.Errorf("método = %q, quero PUT", gotMethod)
			}
			if gotPath != "/appointment/"+apptDAV+"/cancel" {
				t.Errorf("path = %q, quero /appointment/{id}/cancel", gotPath)
			}
			if gotKey != "chave-de-teste" {
				t.Errorf("x-api-key = %q; a chave não foi enviada", gotKey)
			}
		})
	}
}

// 500 é o comportamento REAL da HML hoje (docs/DAV-API-NOTAS.md, achado #20): o
// cancel da DAV responde 500. Por isso ele é best-effort — o adapter devolve erro
// (com o status, sem vazar corpo) e quem chama decide tolerar. E NUNCA repete
// (ADR-011b): repetir um cancel que talvez tenha pego não muda o 500 fixo da HML.
func TestCancelAppointment_500EhErro(t *testing.T) {
	var chamadas int
	c := clientPara(t, func(w http.ResponseWriter, r *http.Request) {
		chamadas++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"code":500,"message":"Unexpected token '<'","trace":"50295b5"}`)
	})

	err := c.CancelAppointment(context.Background(), apptDAV)
	if err == nil {
		t.Fatal("500 não devolveu erro; o cancel precisa sinalizar que não cancelou")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("erro = %v; quero que carregue o status 500", err)
	}
	if chamadas != 1 {
		t.Errorf("chamou %d vezes; o cancel NUNCA repete", chamadas)
	}
}

// Cancelamento do contexto para na hora: se o request morreu, não há motivo para
// insistir contra uma API lenta.
func TestCancelAppointment_RespeitaCancelamentoDoContexto(t *testing.T) {
	var chamadas int
	c := clientPara(t, func(w http.ResponseWriter, r *http.Request) {
		chamadas++
		w.WriteHeader(http.StatusInternalServerError)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.CancelAppointment(ctx, apptDAV)
	if err == nil {
		t.Fatal("contexto cancelado não devolveu erro")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("erro = %v; quero que case com context.Canceled", err)
	}
	if chamadas != 0 {
		t.Errorf("chamou %d vezes; contexto já cancelado não deve bater na DAV", chamadas)
	}
}
