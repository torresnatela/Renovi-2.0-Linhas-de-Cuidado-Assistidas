// Package trigger contém o motor PURO do gatilho de aprofundamento (Anexo C.5.4).
//
// Regra do pacote: sem I/O, sem time.Now(). O estado do gatilho é DERIVADO sob
// demanda de um retrato (Snapshot) do histórico imutável de check-ins e
// aplicações — o wiring monta o Snapshot; aqui só se aplica a máquina de estados.
// A tabela de testes é a especificação.
//
// O gatilho NÃO é elegibilidade de agendamento (por isso fica fora do motor
// `careline`): é "oferecer aprofundar" — deterioração no anel diário oferece
// WHO-5; WHO-5 sinalizando oferece PHQ-4; PHQ-4 positivo escala à trilha clínica.
package trigger

// DefaultN é o nº padrão de dias consecutivos em risco que oferece o WHO-5.
const DefaultN = 4

// State é o estado do gatilho (o que oferecer agora).
type State string

const (
	StateNormal         State = "NORMAL"
	StateOferecerWHO5   State = "OFERECER_WHO5"
	StateOferecerPHQ4   State = "OFERECER_PHQ4"
	StateEscalarClinico State = "ESCALAR_CLINICO"
)

// Params são os parâmetros versionados do gatilho.
type Params struct {
	N int // dias consecutivos em risco p/ oferecer WHO-5 (0 => DefaultN)
}

// Snapshot é o retrato derivado do histórico que a máquina de estados consome.
type Snapshot struct {
	RiskStreak   int  // dias (check-ins) consecutivos recentes em quadrante de risco
	WHO5Recent   bool // há WHO-5 recente (ciclo atual)
	WHO5Sinaliza bool // esse WHO-5 sinaliza (índice < corte de sinalização)
	PHQ4Recent   bool // há PHQ-4 recente
	PHQ4Positivo bool // esse PHQ-4 tem rastreio positivo (subescala ≥ corte)
}

// Evaluate aplica a máquina de estados: o anel mais PROFUNDO respondido
// recentemente decide o estado; sem resposta recente, a deterioração diária
// (RiskStreak ≥ N) oferece o WHO-5.
func Evaluate(s Snapshot, p Params) State {
	n := p.N
	if n <= 0 {
		n = DefaultN
	}
	switch {
	case s.PHQ4Recent:
		// Anel mais profundo respondido: positivo escala, negativo encerra o ciclo.
		if s.PHQ4Positivo {
			return StateEscalarClinico
		}
		return StateNormal
	case s.WHO5Recent:
		// WHO-5 respondido: sinalizando oferece PHQ-4; ok encerra.
		if s.WHO5Sinaliza {
			return StateOferecerPHQ4
		}
		return StateNormal
	case s.RiskStreak >= n:
		return StateOferecerWHO5
	default:
		return StateNormal
	}
}

// Offer é o instrumento a oferecer (vazio quando não há oferta). ESCALAR não
// oferece instrumento — sinaliza escalonamento (ver Escalate).
func (st State) Offer() string {
	switch st {
	case StateOferecerWHO5:
		return "WHO5"
	case StateOferecerPHQ4:
		return "PHQ4"
	default:
		return ""
	}
}

// Escalate diz se o estado exige encaminhamento à trilha clínica.
func (st State) Escalate() bool {
	return st == StateEscalarClinico
}
