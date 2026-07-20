package careline

import (
	"fmt"
	"sort"
	"strings"
)

// ValidatePublish valida uma linha de cuidado antes de publicar.
// specialtyNames: nomes das especialidades ativas do legado (ex. "Psicologia").
// Retorna a lista completa de erros em PT-BR (vazia = publicável) — acumula
// todos, não para no primeiro.
func ValidatePublish(items []Item, rules map[string][]Rule, specialtyNames []string) []string {
	var errs []string

	// (1) pelo menos um item.
	if len(items) == 0 {
		errs = append(errs, "a linha de cuidado precisa de pelo menos um item")
	}

	itemRefs := make(map[string]bool, len(items))
	for _, it := range items {
		itemRefs[it.Ref] = true
	}

	// (5) especialidade de cada item precisa existir no legado (comparação
	// normalizada: maiúsculas, sem acentos, sem espaços nas pontas).
	known := make(map[string]bool, len(specialtyNames))
	for _, n := range specialtyNames {
		known[NormalizeSpecialty(n)] = true
	}
	for _, it := range items {
		// Atividades (ex.: check-in de humor) não têm especialidade — pula a checagem.
		if it.Kind == KindAtividade {
			continue
		}
		if !known[NormalizeSpecialty(it.SpecialtyCode)] {
			errs = append(errs, fmt.Sprintf(
				"item %q: especialidade %q não encontrada no legado", it.Ref, it.SpecialtyCode))
		}
	}

	// (2) params de toda regra + (3) alvo de PREREQUISITE existe entre os itens.
	// Itera em ordem determinística para a lista de erros ser estável.
	prereqEdges := map[string][]string{}
	refs := make([]string, 0, len(rules))
	for ref := range rules {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		for _, r := range rules[ref] {
			parsed, err := ParseRuleParams(r.Type, r.Params)
			if err != nil {
				errs = append(errs, fmt.Sprintf("item %q, regra %s: %v", ref, r.Type, err))
				continue
			}
			if p, ok := parsed.(PrerequisiteParams); ok {
				if !itemRefs[p.ItemRef] {
					errs = append(errs, fmt.Sprintf(
						"item %q: pré-requisito referencia item inexistente %q", ref, p.ItemRef))
					continue
				}
				prereqEdges[ref] = append(prereqEdges[ref], p.ItemRef)
			}
		}
	}

	// (4) sem ciclos de PREREQUISITE (DFS ref → item_ref exigido).
	errs = append(errs, prereqCycleErrors(items, prereqEdges)...)

	return errs
}

// prereqCycleErrors detecta ciclos no grafo de pré-requisitos por DFS com
// três cores, reportando o caminho do ciclo (A -> B -> A).
func prereqCycleErrors(items []Item, edges map[string][]string) []string {
	const (
		white = iota // não visitado
		gray         // na pilha atual
		black        // terminado
	)
	color := map[string]int{}
	var errs []string
	var stack []string

	var visit func(ref string)
	visit = func(ref string) {
		color[ref] = gray
		stack = append(stack, ref)
		for _, dep := range edges[ref] {
			switch color[dep] {
			case white:
				visit(dep)
			case gray:
				// dep está na pilha: fecha um ciclo a partir dele.
				i := 0
				for k, s := range stack {
					if s == dep {
						i = k
						break
					}
				}
				cycle := append(append([]string{}, stack[i:]...), dep)
				errs = append(errs, "ciclo de pré-requisitos: "+strings.Join(cycle, " -> "))
			}
		}
		stack = stack[:len(stack)-1]
		color[ref] = black
	}

	for _, it := range items { // ordem dos itens = ordem determinística
		if color[it.Ref] == white {
			visit(it.Ref)
		}
	}
	return errs
}

// accentFold — tabela manual de acentos pt-BR (sem dependência externa; o
// pacote é puro e o legado só usa esse repertório).
var accentFold = map[rune]rune{
	'Á': 'A', 'À': 'A', 'Â': 'A', 'Ã': 'A', 'Ä': 'A',
	'É': 'E', 'È': 'E', 'Ê': 'E', 'Ë': 'E',
	'Í': 'I', 'Ì': 'I', 'Î': 'I', 'Ï': 'I',
	'Ó': 'O', 'Ò': 'O', 'Ô': 'O', 'Õ': 'O', 'Ö': 'O',
	'Ú': 'U', 'Ù': 'U', 'Û': 'U', 'Ü': 'U',
	'Ç': 'C',
}

// NormalizeSpecialty compara código/nome de especialidade de forma tolerante:
// "PSICOLOGIA" casa "Psicologia"; "NUTRICAO" casa "Nutrição". Exportada porque a
// jornada resolve a especialidade do item contra o catálogo do legado com a MESMA
// normalização do publish — duas cópias divergiriam no primeiro acento.
func NormalizeSpecialty(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if f, ok := accentFold[r]; ok {
			r = f
		}
		b.WriteRune(r)
	}
	return b.String()
}
