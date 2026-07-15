// Package eligibility é o coração testável do sistema: o motor que decide, para
// cada item de uma linha de cuidado, "o que o paciente pode fazer agora e por quê".
//
// REGRA INEGOCIÁVEL: este pacote é PURO — sem I/O, sem banco, sem HTTP, sem
// relógio implícito. Recebe tudo por parâmetro (template, eventos da jornada,
// `now`) e devolve vereditos. É o único lugar do MVC que fica isolado assim,
// justamente porque é o que mais precisa ser exaustivamente testado (table-driven).
//
// STATUS: RESERVADO (fundação). Os tipos abaixo fixam o contrato descrito no
// SPEC §3.3; a implementação de Evaluate entra como primeiro código de negócio
// do MVP, guiada por testes. Ver docs/PROGRESSO.md.
package eligibility

import "time"

// Status é o veredito de um item.
type Status string

const (
	StatusAvailable      Status = "AVAILABLE"       // pode agendar/usar agora
	StatusBlocked        Status = "BLOCKED"         // falta um pré-requisito
	StatusQuotaExhausted Status = "QUOTA_EXHAUSTED" // cota da janela já usada
	StatusNotYetOpen     Status = "NOT_YET_OPEN"    // janela de liberação futura
	StatusOverdue        Status = "OVERDUE"         // atrasado (alimenta lembretes)
)

// Reason é máquina-legível; o front o traduz em frase para o paciente.
// Ex.: {Code: "QUOTA_EXHAUSTED_WEEKLY"}, {Code: "MISSING_PREREQ", Detail: "aval-psiquiatrica"}.
type Reason struct {
	Code   string
	Detail string
}

// QuotaWindow descreve a janela corrente de cota (ex.: 1/1 nesta semana).
type QuotaWindow struct {
	Start time.Time
	End   time.Time
	Used  int
	Limit int
}

// ItemVerdict é a saída por item do motor.
type ItemVerdict struct {
	ItemCode        string
	Status          Status
	Reasons         []Reason
	Window          QuotaWindow
	NextAvailableAt *time.Time // quando destrava (próxima janela / fim do bloqueio)
}

// RecurrenceRule: RRULE simplificada (SPEC §3.2a). Ex.: {WEEKLY,1,1}.
type RecurrenceRule struct {
	Freq     string // WEEKLY | MONTHLY
	Interval int
	Quota    int
}

// Template é a visão congelada (versão) da linha de cuidado que o motor avalia.
type Template struct {
	Code    string
	Version int
	Items   []Item
}

// Item é um passo da linha de cuidado (no MVP, sempre APPOINTMENT).
type Item struct {
	Code         string
	Type         string
	Recurrence   *RecurrenceRule
	Dependencies []Dependency
}

// Dependency: "este item só libera após N ocorrências concluídas do item X".
type Dependency struct {
	RequiresItemCode string
	N                int
}

// JourneyEvent é a única fonte que o motor lê (event log append-only, SPEC §3.3).
type JourneyEvent struct {
	ItemCode   string
	EventType  string // APPOINTMENT_SCHEDULED | APPOINTMENT_COMPLETED | APPOINTMENT_CANCELLED | ...
	OccurredAt time.Time
}

// Evaluate calcula o veredito de cada item do template a partir dos eventos da
// jornada e do instante `now`.
//
// RESERVADO: a implementação entra no MVP (TDD). A assinatura é o contrato.
func Evaluate(t Template, events []JourneyEvent, now time.Time) []ItemVerdict {
	// TODO(mvp): implementar ordem de avaliação do SPEC §3.3:
	//  1) janela de liberação -> NOT_YET_OPEN
	//  2) dependências (conta COMPLETED) -> BLOCKED
	//  3) cota na janela corrente (SCHEDULED+COMPLETED; CANCELLED devolve) -> QUOTA_EXHAUSTED
	//  4) senão -> AVAILABLE (+ flag OVERDUE se última ocorrência > período)
	return nil
}
