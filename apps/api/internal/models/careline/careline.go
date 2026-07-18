// Package careline contém o motor PURO de regras das linhas de cuidado.
//
// Regra do pacote: sem I/O, sem time.Now() — todo tempo entra por parâmetro.
// A tabela de testes T1–T19 em evaluate_test.go é a especificação normativa
// deste slice: mudar semântica aqui exige mudar a tabela primeiro.
package careline

import (
	"encoding/json"
	"time"
)

// Tipos de regra suportados pelo motor.
const (
	RuleVigencia     = "VIGENCIA"
	RuleQuota        = "QUOTA"
	RuleMinInterval  = "MIN_INTERVAL"
	RuleMaxAdvance   = "MAX_ADVANCE"
	RulePrerequisite = "PREREQUISITE"
)

// Períodos de quota são durações fixas (política ADR: janela móvel, não mês civil).
const (
	WeekWindow                  = 7 * 24 * time.Hour
	MonthWindow                 = 30 * 24 * time.Hour
	DefaultCancelCountThreshold = 24 * time.Hour
)

// Tipos de item da linha de cuidado (espelha o CHECK de care_line_item.kind).
// CONSULTA aponta para uma especialidade do legado; ATIVIDADE (ex.: check-in de
// humor do Anexo C) não tem especialidade e é executada dentro da própria
// plataforma.
const (
	KindConsulta  = "CONSULTA"
	KindAtividade = "ATIVIDADE"
)

// Statuses de care_appointment (vocabulário do slice, em PT).
const (
	StatusAgendada    = "agendada"
	StatusConfirmada  = "confirmada"
	StatusEmAndamento = "em_andamento"
	StatusRealizada   = "realizada"
	StatusFalta       = "falta"
	StatusCancelada   = "cancelada"
)

// Statuses de matrícula.
const (
	EnrollmentAtiva     = "ativa"
	EnrollmentPausada   = "pausada"
	EnrollmentConcluida = "concluida"
	EnrollmentEncerrada = "encerrada"
	EnrollmentExpirada  = "expirada"
)

// Journey é a matrícula do paciente na linha de cuidado, com tudo que o motor
// precisa para decidir — nada é buscado fora daqui.
type Journey struct {
	Status       string // status da matrícula
	ValidFrom    time.Time
	ValidUntil   time.Time
	LineItems    []Item               // itens da linha (para resolver label em PREREQUISITE)
	Appointments []JourneyAppointment // TODAS as consultas da matrícula (todos os itens)

	// CancelCountThreshold: antecedência mínima de cancelamento para a consulta
	// NÃO contar na cota. 0 => DefaultCancelCountThreshold.
	CancelCountThreshold time.Duration
}

// JourneyAppointment é uma consulta da matrícula, vista pelo motor.
type JourneyAppointment struct {
	ItemRef     string
	Status      string
	ScheduledAt time.Time
	CancelledAt *time.Time // preenchido quando Status == cancelada
}

// Item é um item da linha de cuidado (ex.: consulta de psicologia).
type Item struct {
	Ref           string
	Kind          string
	SpecialtyCode string
	Label         string
}

// Rule é uma regra configurada para um item, com params ainda crus (JSON).
type Rule struct {
	Type   string
	Params json.RawMessage
}

// Block explica ao paciente por que o agendamento está bloqueado.
type Block struct {
	RuleType      string     // VIGENCIA | QUOTA | MIN_INTERVAL | MAX_ADVANCE | PREREQUISITE
	Reason        string     // PT-BR, exibível ao paciente
	AvailableFrom *time.Time // quando aplicável (nil = depende de ação, não de tempo)
}

// Eligibility é o veredito do motor para um intendedAt.
type Eligibility struct {
	Allowed bool
	Blocks  []Block // vazio quando Allowed
}
