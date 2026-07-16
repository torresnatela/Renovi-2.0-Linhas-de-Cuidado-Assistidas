package scheduling_test

import (
	"testing"
	"time"

	"github.com/renovisaude/renovi-care/internal/models/scheduling"
)

// A política do piloto: abre 30 min antes e fecha junto com o fim da consulta.
var politica = scheduling.Policy{OpensBefore: 30 * time.Minute, ClosesAfter: 0}

// inicio é o horário da consulta usado em toda a tabela. Fuso de São Paulo
// porque é o do negócio — e porque uma regra que só passa em UTC não serve.
var (
	saoPaulo = mustLoad("America/Sao_Paulo")
	inicio   = time.Date(2026, 7, 20, 9, 0, 0, 0, saoPaulo)
	fim      = inicio.Add(25 * time.Minute)
)

func mustLoad(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		nome       string
		estado     scheduling.State
		now        time.Time
		wantStatus scheduling.Status
		wantReason string
	}{
		// --- A fronteira dos 30 minutos. É o que a regra existe para decidir,
		// então é onde os testes têm que ser chatos. ---
		{
			nome:       "exatamente 30min antes: ABRE (a fronteira é inclusiva)",
			estado:     scheduling.StateConfirmed,
			now:        inicio.Add(-30 * time.Minute),
			wantStatus: scheduling.StatusOpen,
		},
		{
			nome:       "1 segundo antes de abrir: ainda não",
			estado:     scheduling.StateConfirmed,
			now:        inicio.Add(-30*time.Minute - time.Second),
			wantStatus: scheduling.StatusTooEarly,
			wantReason: scheduling.ReasonTooEarly,
		},
		{
			nome:       "1 segundo depois de abrir",
			estado:     scheduling.StateConfirmed,
			now:        inicio.Add(-30*time.Minute + time.Second),
			wantStatus: scheduling.StatusOpen,
		},
		{
			nome:       "no horário da consulta",
			estado:     scheduling.StateConfirmed,
			now:        inicio,
			wantStatus: scheduling.StatusOpen,
		},
		{
			nome:       "no meio da consulta (atrasado, mas entra)",
			estado:     scheduling.StateConfirmed,
			now:        inicio.Add(10 * time.Minute),
			wantStatus: scheduling.StatusOpen,
		},
		{
			nome:       "exatamente no fim: ainda abre (fronteira inclusiva)",
			estado:     scheduling.StateConfirmed,
			now:        fim,
			wantStatus: scheduling.StatusOpen,
		},
		{
			nome:       "1 segundo depois do fim: acabou",
			estado:     scheduling.StateConfirmed,
			now:        fim.Add(time.Second),
			wantStatus: scheduling.StatusTooLate,
			wantReason: scheduling.ReasonTooLate,
		},
		{
			nome:       "muito cedo (véspera)",
			estado:     scheduling.StateConfirmed,
			now:        inicio.Add(-24 * time.Hour),
			wantStatus: scheduling.StatusTooEarly,
			wantReason: scheduling.ReasonTooEarly,
		},

		// --- O estado da consulta vence o relógio: não adianta estar na hora se
		// não há sala para entrar. ---
		{
			nome:       "cancelada, mesmo dentro da janela",
			estado:     scheduling.StateCancelled,
			now:        inicio,
			wantStatus: scheduling.StatusUnavailable,
			wantReason: scheduling.ReasonCancelled,
		},
		{
			nome:       "ainda não confirmada na DAV, dentro da janela",
			estado:     scheduling.StatePending,
			now:        inicio,
			wantStatus: scheduling.StatusUnavailable,
			wantReason: scheduling.ReasonNotConfirmed,
		},
		{
			nome:       "cancelada e cedo: o cancelamento é o motivo mais útil",
			estado:     scheduling.StateCancelled,
			now:        inicio.Add(-24 * time.Hour),
			wantStatus: scheduling.StatusUnavailable,
			wantReason: scheduling.ReasonCancelled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			got := scheduling.Evaluate(politica, tt.estado, inicio, fim, tt.now)

			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, quero %q", got.Status, tt.wantStatus)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("Reason = %q, quero %q", got.Reason, tt.wantReason)
			}
			if (got.Status == scheduling.StatusOpen) != got.Allowed {
				t.Errorf("Allowed = %v mas Status = %q — os dois têm que concordar",
					got.Allowed, got.Status)
			}
		})
	}
}

// OpensAt é o campo que substitui a regra no front: em vez de o cliente saber
// "30 minutos", ele recebe a HORA. Se este cálculo estiver errado, a tela mente
// para o paciente sobre quando voltar.
func TestEvaluate_SempreDizQuandoAbreEFecha(t *testing.T) {
	casos := []struct {
		nome   string
		estado scheduling.State
		now    time.Time
	}{
		{"cedo demais", scheduling.StateConfirmed, inicio.Add(-2 * time.Hour)},
		{"aberta", scheduling.StateConfirmed, inicio},
		{"tarde demais", scheduling.StateConfirmed, fim.Add(time.Hour)},
		// Mesmo indisponível: a tela ainda quer dizer que horas ERA a consulta.
		{"cancelada", scheduling.StateCancelled, inicio},
	}

	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			got := scheduling.Evaluate(politica, c.estado, inicio, fim, c.now)

			if want := inicio.Add(-30 * time.Minute); !got.OpensAt.Equal(want) {
				t.Errorf("OpensAt = %s, quero %s", got.OpensAt, want)
			}
			if !got.ClosesAt.Equal(fim) {
				t.Errorf("ClosesAt = %s, quero %s", got.ClosesAt, fim)
			}
		})
	}
}

// A política é parâmetro, não constante: produto muda "30 minutos" sem que este
// pacote (ou o front) saiba.
func TestEvaluate_RespeitaAPolitica(t *testing.T) {
	generosa := scheduling.Policy{OpensBefore: 2 * time.Hour, ClosesAfter: 15 * time.Minute}

	umaHoraAntes := scheduling.Evaluate(generosa, scheduling.StateConfirmed, inicio, fim, inicio.Add(-time.Hour))
	if umaHoraAntes.Status != scheduling.StatusOpen {
		t.Errorf("com OpensBefore=2h, 1h antes devia abrir; veio %q", umaHoraAntes.Status)
	}

	depoisDaTolerancia := scheduling.Evaluate(generosa, scheduling.StateConfirmed, inicio, fim, fim.Add(10*time.Minute))
	if depoisDaTolerancia.Status != scheduling.StatusOpen {
		t.Errorf("com ClosesAfter=15min, 10min depois do fim devia abrir; veio %q", depoisDaTolerancia.Status)
	}

	foraDaTolerancia := scheduling.Evaluate(generosa, scheduling.StateConfirmed, inicio, fim, fim.Add(16*time.Minute))
	if foraDaTolerancia.Status != scheduling.StatusTooLate {
		t.Errorf("16min depois do fim, com tolerância de 15, devia fechar; veio %q", foraDaTolerancia.Status)
	}
}

// O veredito é sobre INSTANTES, não sobre a hora de parede de quem pergunta. O
// mesmo momento, expresso em qualquer fuso, tem que dar o mesmo resultado —
// senão a sala abre na hora errada para quem estiver viajando.
func TestEvaluate_NaoDependeDoFusoDoChamador(t *testing.T) {
	momento := inicio.Add(-10 * time.Minute)

	emSaoPaulo := scheduling.Evaluate(politica, scheduling.StateConfirmed, inicio, fim, momento)
	emUTC := scheduling.Evaluate(politica, scheduling.StateConfirmed, inicio, fim, momento.UTC())
	emLisboa := scheduling.Evaluate(politica, scheduling.StateConfirmed, inicio, fim, momento.In(mustLoad("Europe/Lisbon")))

	if emSaoPaulo.Status != emUTC.Status || emSaoPaulo.Status != emLisboa.Status {
		t.Errorf("mesmo instante deu vereditos diferentes: SP=%q UTC=%q Lisboa=%q",
			emSaoPaulo.Status, emUTC.Status, emLisboa.Status)
	}
}
