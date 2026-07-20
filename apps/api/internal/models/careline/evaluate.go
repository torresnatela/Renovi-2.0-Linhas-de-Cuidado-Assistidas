package careline

import (
	"fmt"
	"sort"
	"time"
)

const dateLayout = "02/01/2006"

// Evaluate decide se o paciente pode agendar `item` em `intendedAt`, dado o
// estado da matrícula e as regras do item. `now` só importa para MAX_ADVANCE.
// Avalia TODAS as regras e devolve TODOS os blocks — vigência primeiro, depois
// na ordem das rules. Allowed == (nenhum block).
func Evaluate(j Journey, item Item, rules []Rule, intendedAt, now time.Time) Eligibility {
	threshold := j.CancelCountThreshold
	if threshold == 0 {
		threshold = DefaultCancelCountThreshold
	}

	var blocks []Block
	if b := vigenciaBlock(j, intendedAt); b != nil {
		blocks = append(blocks, *b)
	}

	for _, r := range rules {
		if r.Type == RuleVigencia {
			// Vigência é pré-condição intrínseca — já avaliada acima.
			continue
		}
		parsed, err := ParseRuleParams(r.Type, r.Params)
		if err != nil {
			// Falha FECHADA: o publish valida antes, mas se uma regra inválida
			// chegar aqui, bloqueia em vez de fingir que ela não existe.
			blocks = append(blocks, Block{
				RuleType: r.Type,
				Reason:   "Regra inválida na configuração da linha de cuidado",
			})
			continue
		}

		var b *Block
		switch p := parsed.(type) {
		case QuotaParams:
			b = quotaBlock(j, item, p, intendedAt, threshold)
		case MinIntervalParams:
			b = minIntervalBlock(j, item, p, intendedAt, threshold)
		case MaxAdvanceParams:
			b = maxAdvanceBlock(p, intendedAt, now)
		case PrerequisiteParams:
			b = prerequisiteBlock(j, p, intendedAt)
		}
		if b != nil {
			blocks = append(blocks, *b)
		}
	}

	return Eligibility{Allowed: len(blocks) == 0, Blocks: blocks}
}

// vigenciaBlock — pré-condição da matrícula. Nunca curto-circuita as demais
// regras; só contribui (ou não) com o primeiro block da lista.
func vigenciaBlock(j Journey, intendedAt time.Time) *Block {
	if j.Status != EnrollmentAtiva {
		var reason string
		switch j.Status {
		case EnrollmentPausada:
			reason = "Sua matrícula está pausada"
		case EnrollmentConcluida:
			reason = "Sua matrícula foi concluída"
		case EnrollmentEncerrada:
			reason = "Sua matrícula foi encerrada"
		case EnrollmentExpirada:
			reason = "Sua matrícula expirou. Renove para continuar"
		default:
			reason = "Sua matrícula não está ativa"
		}
		return &Block{RuleType: RuleVigencia, Reason: reason}
	}
	if intendedAt.Before(j.ValidFrom) {
		from := j.ValidFrom
		return &Block{
			RuleType:      RuleVigencia,
			Reason:        fmt.Sprintf("Seu plano inicia em %s", from.Format(dateLayout)),
			AvailableFrom: &from,
		}
	}
	if intendedAt.After(j.ValidUntil) {
		// nil: depende de renovar, não de esperar.
		return &Block{
			RuleType: RuleVigencia,
			Reason: fmt.Sprintf("Seu plano vai até %s. Renove para agendar além dessa data",
				j.ValidUntil.Format(dateLayout)),
		}
	}
	return nil
}

// counts diz se uma consulta entra na contagem de QUOTA e MIN_INTERVAL.
// Política: agendada/confirmada/em_andamento/realizada/falta contam sempre.
// Cancelada só NÃO conta quando o cancelamento aconteceu com pelo menos
// `threshold` de antecedência; cancelamento tardio (ou sem CancelledAt
// registrado) consome a vaga do mesmo jeito.
func counts(a JourneyAppointment, threshold time.Duration) bool {
	switch a.Status {
	case StatusAgendada, StatusConfirmada, StatusEmAndamento, StatusRealizada, StatusFalta:
		return true
	case StatusCancelada:
		if a.CancelledAt != nil && !a.CancelledAt.After(a.ScheduledAt.Add(-threshold)) {
			return false // cancelou cedo o bastante: libera a vaga
		}
		return true
	default:
		return false
	}
}

// countedOf filtra as consultas de um item que contam para as cotas.
func countedOf(j Journey, itemRef string, threshold time.Duration) []JourneyAppointment {
	var out []JourneyAppointment
	for _, a := range j.Appointments {
		if a.ItemRef == itemRef && counts(a, threshold) {
			out = append(out, a)
		}
	}
	return out
}

// quotaBlock — QUOTA com janela móvel GERAL: bloqueia se ALGUMA janela de
// duração `period` contendo intendedAt já tem >= max consultas que contam.
// Não é só as duas janelas ancoradas no intendedAt: uma janela começando numa
// consulta antiga também bloqueia (caso T18 — a janela móvel não respeita
// fronteira de ciclo).
func quotaBlock(j Journey, item Item, p QuotaParams, intendedAt time.Time, threshold time.Duration) *Block {
	counted := countedOf(j, item.Ref, threshold)

	if p.Period == "total" {
		if len(counted) >= p.Max {
			// Janela nunca libera: só cancelamento com antecedência devolve vaga.
			return &Block{
				RuleType: RuleQuota,
				Reason:   fmt.Sprintf("Você atingiu o limite de %d consulta(s) desta linha de cuidado", p.Max),
			}
		}
		return nil
	}

	period := WeekWindow
	label := "semana"
	if p.Period == "month" {
		period = MonthWindow
		label = "mês"
	}

	// Só consultas a menos de `period` do intendedAt podem dividir com ele uma
	// janela de duração `period` (janelas são semiabertas: distância exata de
	// `period` fica de fora — é o que salva o T17).
	var cand []time.Time
	for _, a := range counted {
		d := a.ScheduledAt.Sub(intendedAt)
		if d < 0 {
			d = -d
		}
		if d < period {
			cand = append(cand, a.ScheduledAt)
		}
	}
	if len(cand) < p.Max {
		return nil
	}
	sort.Slice(cand, func(i, k int) bool { return cand[i].Before(cand[k]) })

	// Basta testar janelas ancoradas em cada consulta — começando nela
	// [c, c+period) ou terminando nela (c-period, c] — e as duas ancoradas no
	// próprio intendedAt. O(n²), listas pequenas.
	type window struct {
		start     time.Time
		openStart bool // true: (start, start+period]; false: [start, start+period)
	}
	var wins []window
	for _, c := range cand {
		wins = append(wins,
			window{start: c},
			window{start: c.Add(-period), openStart: true},
		)
	}
	wins = append(wins,
		window{start: intendedAt},
		window{start: intendedAt.Add(-period), openStart: true},
	)

	contains := func(w window, t time.Time) bool {
		end := w.start.Add(period)
		if w.openStart {
			return t.After(w.start) && !t.After(end)
		}
		return !t.Before(w.start) && t.Before(end)
	}

	// Entre as janelas que bloqueiam, fica a de ÂNCORA mais antiga; AvailableFrom é
	// quando ELA libera. A vaga só abre quando consultas suficientes "saem" da
	// janela: é preciso esperar a (max)-ésima mais RECENTE envelhecer, ou seja, a
	// (n-max+1)-ésima mais ANTIGA da janela + period. Com n == max isso é a mais
	// antiga + period (preserva T02/T03/T18 e o E2E semanal); só diverge quando a
	// janela está super-lotada (n > max: config reduzida, override, dados migrados),
	// caso em que a mais antiga + period ainda estaria bloqueado.
	var blocking *window
	var af time.Time
	for i := range wins {
		w := wins[i]
		if !contains(w, intendedAt) {
			continue
		}
		var inWin []time.Time // consultas da janela em ordem crescente (cand já ordenado)
		for _, c := range cand {
			if contains(w, c) {
				inWin = append(inWin, c)
			}
		}
		if len(inWin) < p.Max {
			continue
		}
		if blocking == nil || w.start.Before(blocking.start) {
			ww := w
			blocking = &ww
			af = inWin[len(inWin)-p.Max].Add(period)
		}
	}
	if blocking == nil {
		return nil
	}
	return &Block{
		RuleType: RuleQuota,
		Reason: fmt.Sprintf("Você atingiu o limite de %d consulta(s) por %s. Disponível a partir de %s",
			p.Max, label, af.Format(dateLayout)),
		AvailableFrom: &af,
	}
}

// minIntervalBlock — bidirecional: bloqueia se alguma consulta que conta (do
// mesmo item) está a MENOS de D dias do intendedAt. Distância exata de D dias
// é permitida.
func minIntervalBlock(j Journey, item Item, p MinIntervalParams, intendedAt time.Time, threshold time.Duration) *Block {
	minGap := time.Duration(p.Days) * 24 * time.Hour
	var latest time.Time
	found := false
	for _, a := range countedOf(j, item.Ref, threshold) {
		d := a.ScheduledAt.Sub(intendedAt)
		if d < 0 {
			d = -d
		}
		if d < minGap {
			// AvailableFrom parte do vizinho conflitante MAIS TARDIO.
			if !found || a.ScheduledAt.After(latest) {
				latest = a.ScheduledAt
				found = true
			}
		}
	}
	if !found {
		return nil
	}
	af := latest.Add(minGap)
	return &Block{
		RuleType: RuleMinInterval,
		Reason: fmt.Sprintf("Muito perto de outra consulta já marcada. Disponível a partir de %s",
			af.Format(dateLayout)),
		AvailableFrom: &af,
	}
}

// maxAdvanceBlock — bloqueia agendamento além de D dias no futuro (a partir
// de now). AvailableFrom é o instante em que esse horário abrirá.
func maxAdvanceBlock(p MaxAdvanceParams, intendedAt, now time.Time) *Block {
	horizon := time.Duration(p.Days) * 24 * time.Hour
	if !intendedAt.After(now.Add(horizon)) {
		return nil
	}
	af := intendedAt.Add(-horizon)
	return &Block{
		RuleType:      RuleMaxAdvance,
		Reason:        fmt.Sprintf("Só é possível agendar com até %d dias de antecedência", p.Days),
		AvailableFrom: &af,
	}
}

// prerequisiteBlock — exige consulta do item referenciado com o status pedido
// dentro da janela retroativa de within_days. Depende de ação do paciente,
// então AvailableFrom fica nil.
func prerequisiteBlock(j Journey, p PrerequisiteParams, intendedAt time.Time) *Block {
	cutoff := intendedAt.Add(-time.Duration(p.WithinDays) * 24 * time.Hour)
	for _, a := range j.Appointments {
		// Janela RETROATIVA [intendedAt-within, intendedAt]: uma consulta do
		// pré-requisito marcada DEPOIS do horário pretendido não satisfaz
		// "realize primeiro" — o limite superior é tão obrigatório quanto o cutoff.
		if a.ItemRef == p.ItemRef && a.Status == p.Status &&
			!a.ScheduledAt.Before(cutoff) && !a.ScheduledAt.After(intendedAt) {
			return nil
		}
	}
	label := p.ItemRef
	for _, it := range j.LineItems {
		if it.Ref == p.ItemRef {
			if it.Label != "" {
				label = it.Label
			}
			break
		}
	}
	return &Block{RuleType: RulePrerequisite, Reason: "Realize primeiro: " + label}
}
