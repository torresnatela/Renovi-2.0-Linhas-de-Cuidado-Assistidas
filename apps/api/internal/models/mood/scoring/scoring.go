// Package scoring contém a pontuação PURA do Verificador de Humor (Anexo C):
// derivação de quadrante do circumplexo e escores de WHO-5 e PHQ-4.
//
// Regra do pacote: sem I/O, sem time.Now(). Os cortes entram por parâmetro —
// eles vivem em instrument_cutoff (validação BR, versionada), não em código.
// A pontuação é determinística: mesma resposta, mesmo escore.
package scoring

import "fmt"

// GridMidpoint é o corte determinístico entre o lado alto e o baixo da grade.
// >= é o lado ALTO (agradável / ativado).
const GridMidpoint = 50

// Quadrantes do circumplexo (valência × energia). Vocabulário PRÓPRIO da Renovi:
// a paleta e a nomenclatura curadas do Mood Meter são marca da Yale (RULER),
// portanto não são usadas aqui (Anexo C.4, ressalva de PI).
const (
	QuadranteAgradavelAtivado    = "agradavel_ativado"    // valência alta, energia alta
	QuadranteAgradavelCalmo      = "agradavel_calmo"      // valência alta, energia baixa
	QuadranteDesagradavelAtivado = "desagradavel_ativado" // valência baixa, energia alta (estresse/ansiedade)
	QuadranteDesagradavelCalmo   = "desagradavel_calmo"   // valência baixa, energia baixa (depressão/esgotamento)
)

// Faixas derivadas dos cortes (compartilhadas entre WHO-5 e PHQ-4).
const (
	FaixaNormal    = "normal"
	FaixaSinaliza  = "sinaliza"  // WHO-5: baixo bem-estar
	FaixaEncaminha = "encaminha" // WHO-5: rastreio positivo p/ depressão
	FaixaModerado  = "moderado"  // PHQ-4: sofrimento moderado (total)
)

// Quadrant deriva o quadrante do circumplexo a partir das coordenadas 0–100.
// Determinístico: >= GridMidpoint é o lado alto (agradável / ativado).
func Quadrant(valencia, energia int) string {
	agradavel := valencia >= GridMidpoint
	ativado := energia >= GridMidpoint
	switch {
	case agradavel && ativado:
		return QuadranteAgradavelAtivado
	case agradavel && !ativado:
		return QuadranteAgradavelCalmo
	case !agradavel && ativado:
		return QuadranteDesagradavelAtivado
	default:
		return QuadranteDesagradavelCalmo
	}
}

// IsQuadranteRisco diz se o quadrante é de risco — baixa valência (desagradável).
// É o sinal que o motor de gatilho (Anexo C.5.4) acumula para oferecer o WHO-5.
func IsQuadranteRisco(quadrante string) bool {
	return quadrante == QuadranteDesagradavelAtivado || quadrante == QuadranteDesagradavelCalmo
}

// WHO5Cutoffs são os cortes de sinalização do WHO-5 sobre o índice 0–100.
type WHO5Cutoffs struct {
	Sinaliza  int // índice < Sinaliza  => baixo bem-estar (sinaliza)
	Encaminha int // índice < Encaminha => rastreio positivo p/ depressão (encaminha)
}

// WHO5Result é o resultado pontuado do WHO-5.
type WHO5Result struct {
	Raw            int    // bruto 0–25
	Index          int    // índice 0–100 (bruto × 4)
	Faixa          string // normal | sinaliza | encaminha
	FlagEncaminhar bool   // índice < Encaminha
}

// ScoreWHO5 pontua o WHO-5: 5 itens Likert 0–5, índice = bruto × 4.
func ScoreWHO5(items []int, c WHO5Cutoffs) (WHO5Result, error) {
	if len(items) != 5 {
		return WHO5Result{}, fmt.Errorf("WHO-5 exige 5 itens, recebeu %d", len(items))
	}
	raw := 0
	for i, v := range items {
		if v < 0 || v > 5 {
			return WHO5Result{}, fmt.Errorf("WHO-5 item %d fora da faixa 0–5: %d", i+1, v)
		}
		raw += v
	}
	index := raw * 4

	faixa := FaixaNormal
	switch {
	case index < c.Encaminha:
		faixa = FaixaEncaminha
	case index < c.Sinaliza:
		faixa = FaixaSinaliza
	}
	return WHO5Result{
		Raw:            raw,
		Index:          index,
		Faixa:          faixa,
		FlagEncaminhar: index < c.Encaminha,
	}, nil
}

// PHQ4Cutoffs são os cortes do PHQ-4 sobre as subescalas e o total.
type PHQ4Cutoffs struct {
	SubescalaPositiva int // subescala (PHQ-2 ou GAD-2) >= => rastreio positivo
	TotalModerado     int // total >= => sofrimento moderado
}

// PHQ4Result é o resultado pontuado do PHQ-4 (subescalas PHQ-2 e GAD-2).
type PHQ4Result struct {
	Total          int
	PHQ2           int    // depressão (itens 1–2)
	GAD2           int    // ansiedade (itens 3–4)
	Faixa          string // normal | moderado (pelo total)
	FlagEncaminhar bool   // alguma subescala >= SubescalaPositiva
}

// ScorePHQ4 pontua o PHQ-4: 4 itens Likert 0–3. Itens 1–2 = PHQ-2 (depressão),
// itens 3–4 = GAD-2 (ansiedade). Subescala >= corte é rastreio positivo.
func ScorePHQ4(items []int, c PHQ4Cutoffs) (PHQ4Result, error) {
	if len(items) != 4 {
		return PHQ4Result{}, fmt.Errorf("PHQ-4 exige 4 itens, recebeu %d", len(items))
	}
	for i, v := range items {
		if v < 0 || v > 3 {
			return PHQ4Result{}, fmt.Errorf("PHQ-4 item %d fora da faixa 0–3: %d", i+1, v)
		}
	}
	phq2 := items[0] + items[1]
	gad2 := items[2] + items[3]
	total := phq2 + gad2

	faixa := FaixaNormal
	if total >= c.TotalModerado {
		faixa = FaixaModerado
	}
	return PHQ4Result{
		Total:          total,
		PHQ2:           phq2,
		GAD2:           gad2,
		Faixa:          faixa,
		FlagEncaminhar: phq2 >= c.SubescalaPositiva || gad2 >= c.SubescalaPositiva,
	}, nil
}
