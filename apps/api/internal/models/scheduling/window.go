// Package scheduling guarda as decisões PURAS do agendamento.
//
// REGRA (mesma do models/careline, ADR-002): sem I/O, sem banco, sem HTTP,
// sem relógio implícito. Tudo entra por parâmetro — inclusive `now` e a política.
// É o que permite testar a fronteira dos 30 minutos com uma tabela em vez de
// esperar meia hora.
//
// Hoje só mora aqui a janela de entrada na teleconsulta. É pouco código para um
// pacote, mas é a única regra de negócio de verdade do agendamento (o resto é
// coordenação entre dois bancos e a DAV), e é a que o produto mais vai querer
// mexer.
package scheduling

import "time"

// Status espelha JoinWindow.status do openapi.yaml.
type Status string

const (
	StatusOpen        Status = "OPEN"        // pode entrar agora
	StatusTooEarly    Status = "TOO_EARLY"   // ainda não; OpensAt diz quando
	StatusTooLate     Status = "TOO_LATE"    // a consulta já terminou
	StatusUnavailable Status = "UNAVAILABLE" // não há sala para entrar
)

// Códigos de motivo. Máquina-legíveis; quem traduz em frase é o front, com a
// mesma tabela que já traduz os Reason do motor de elegibilidade.
const (
	ReasonTooEarly     = "JOIN_TOO_EARLY"
	ReasonTooLate      = "JOIN_TOO_LATE"
	ReasonCancelled    = "JOIN_CANCELLED"
	ReasonNotConfirmed = "JOIN_UNAVAILABLE"
)

// State é o que a janela precisa saber sobre a consulta.
//
// É um enum próprio, e não o status do banco, de propósito: o pacote puro não
// deve conhecer o vocabulário da saga (PENDING_DAV, DAV_UNKNOWN, NEEDS_REVIEW…).
// Quem mapeia é o model. Assim, acrescentar um estado interno novo não mexe
// nesta regra nem nos testes dela.
type State int

const (
	// StateConfirmed: existe na DAV e temos o link do paciente. É o único estado
	// em que entrar faz sentido.
	StateConfirmed State = iota
	// StatePending: reservamos o horário mas a DAV ainda não confirmou — ou nunca
	// vai confirmar (o resultado ficou desconhecido; ver achado #12).
	StatePending
	StateCancelled
)

// Policy é a antecedência e a tolerância. Vem da config e entra por parâmetro:
// o dia em que o produto quiser 15 minutos, ninguém precisa tocar nesta regra.
type Policy struct {
	OpensBefore time.Duration // quanto antes do início a sala abre
	ClosesAfter time.Duration // quanto depois do fim ainda deixa entrar
}

// Window é o veredito.
type Window struct {
	Status  Status
	Allowed bool
	// Reason é vazio quando Allowed. É o que a tela mostra no lugar de um botão
	// desabilitado mudo (regra de ouro do apps/web/CLAUDE.md).
	Reason string
	// OpensAt/ClosesAt saem SEMPRE, mesmo quando não se pode entrar: é o que
	// substitui a regra no cliente. O front não sabe que são 30 minutos — ele
	// recebe a hora e espera por ela.
	OpensAt  time.Time
	ClosesAt time.Time
}

// Evaluate decide se o paciente pode entrar na consulta agora.
//
// As fronteiras são INCLUSIVAS dos dois lados: exatamente 30min antes já abre, e
// exatamente no fim ainda entra. É decisão, não acaso — está na tabela de testes
// para que ninguém a "conserte" sem querer.
func Evaluate(p Policy, state State, startsAt, endsAt, now time.Time) Window {
	w := Window{
		OpensAt:  startsAt.Add(-p.OpensBefore),
		ClosesAt: endsAt.Add(p.ClosesAfter),
	}

	// O estado vence o relógio: não adianta ser a hora certa se não há sala.
	switch state {
	case StateCancelled:
		w.Status, w.Reason = StatusUnavailable, ReasonCancelled
		return w
	case StatePending:
		w.Status, w.Reason = StatusUnavailable, ReasonNotConfirmed
		return w
	}

	switch {
	case now.Before(w.OpensAt):
		w.Status, w.Reason = StatusTooEarly, ReasonTooEarly
	case now.After(w.ClosesAt):
		w.Status, w.Reason = StatusTooLate, ReasonTooLate
	default:
		w.Status, w.Allowed = StatusOpen, true
	}
	return w
}
