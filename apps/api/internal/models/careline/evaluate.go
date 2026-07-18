package careline

import "time"

// Evaluate decide se o paciente pode agendar `item` em `intendedAt`, dado o
// estado da matrícula e as regras do item. `now` só importa para MAX_ADVANCE.
// Avalia TODAS as regras e devolve TODOS os blocks (vigência primeiro).
func Evaluate(j Journey, item Item, rules []Rule, intendedAt, now time.Time) Eligibility {
	return Eligibility{}
}
