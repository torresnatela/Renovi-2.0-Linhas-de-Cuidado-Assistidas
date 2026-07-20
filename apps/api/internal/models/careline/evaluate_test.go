package careline_test

// Tabela normativa T1–T19 do motor de linhas de cuidado. Esta tabela É a
// especificação do slice: cada caso pinça uma fronteira semântica (janela
// móvel GERAL de quota, política de contagem de canceladas, composição sem
// curto-circuito). Mudar comportamento = mudar aqui primeiro.

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/models/careline"
)

// base fixa a linha do tempo: day(1) = 01/08/2026 12:00 UTC.
var base = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

func day(n int) time.Time { return base.AddDate(0, 0, n-1) }

var (
	psicologia  = careline.Item{Ref: "psicologia", Kind: "consulta", SpecialtyCode: "PSICOLOGIA", Label: "Psicologia"}
	psiquiatria = careline.Item{Ref: "psiquiatria", Kind: "consulta", SpecialtyCode: "PSIQUIATRIA", Label: "Consulta psiquiátrica"}
)

// activeJourney monta a matrícula padrão: ativa, vigente de day(1) a day(31).
func activeJourney(appts ...careline.JourneyAppointment) careline.Journey {
	return careline.Journey{
		Status:       careline.EnrollmentAtiva,
		ValidFrom:    day(1),
		ValidUntil:   day(31),
		LineItems:    []careline.Item{psicologia, psiquiatria},
		Appointments: appts,
	}
}

func agendada(n int) careline.JourneyAppointment {
	return consulta("psicologia", n, careline.StatusAgendada)
}

func consulta(itemRef string, n int, status string) careline.JourneyAppointment {
	return careline.JourneyAppointment{ItemRef: itemRef, Status: status, ScheduledAt: day(n)}
}

// canceladaComAntecedencia: cancelada `antecedencia` antes do horário marcado.
func canceladaComAntecedencia(n int, antecedencia time.Duration) careline.JourneyAppointment {
	c := day(n).Add(-antecedencia)
	return careline.JourneyAppointment{
		ItemRef: "psicologia", Status: careline.StatusCancelada,
		ScheduledAt: day(n), CancelledAt: &c,
	}
}

func quotaMonthly(max int) careline.Rule {
	return careline.Rule{Type: careline.RuleQuota,
		Params: json.RawMessage(fmt.Sprintf(`{"max":%d,"period":"month","window":"rolling"}`, max))}
}

func quotaTotal(max int) careline.Rule {
	return careline.Rule{Type: careline.RuleQuota,
		Params: json.RawMessage(fmt.Sprintf(`{"max":%d,"period":"total"}`, max))}
}

func minInterval(days int) careline.Rule {
	return careline.Rule{Type: careline.RuleMinInterval,
		Params: json.RawMessage(fmt.Sprintf(`{"days":%d}`, days))}
}

func maxAdvance(days int) careline.Rule {
	return careline.Rule{Type: careline.RuleMaxAdvance,
		Params: json.RawMessage(fmt.Sprintf(`{"days":%d}`, days))}
}

func prerequisite(itemRef, status string, withinDays int) careline.Rule {
	return careline.Rule{Type: careline.RulePrerequisite,
		Params: json.RawMessage(fmt.Sprintf(`{"item_ref":%q,"status":%q,"within_days":%d}`, itemRef, status, withinDays))}
}

func findBlock(e careline.Eligibility, ruleType string) *careline.Block {
	for i := range e.Blocks {
		if e.Blocks[i].RuleType == ruleType {
			return &e.Blocks[i]
		}
	}
	return nil
}

// requireAvailableFrom exige AvailableFrom preenchido e igual a want.
func requireAvailableFrom(t *testing.T, b *careline.Block, want time.Time) {
	t.Helper()
	require.NotNil(t, b)
	require.NotNil(t, b.AvailableFrom, "AvailableFrom deveria estar preenchido")
	assert.Truef(t, b.AvailableFrom.Equal(want),
		"AvailableFrom = %s, quero %s", b.AvailableFrom, want)
}

func TestEvaluate_TabelaNormativa(t *testing.T) {
	quatroMarcadas := []careline.JourneyAppointment{
		agendada(5), agendada(12), agendada(19), agendada(26),
	}

	tests := []struct {
		name        string
		j           careline.Journey
		item        careline.Item
		rules       []careline.Rule
		intendedAt  time.Time
		now         time.Time
		wantAllowed bool
		wantTypes   []string // RuleTypes que DEVEM aparecer nos Blocks
		check       func(t *testing.T, got careline.Eligibility)
	}{
		{
			name:        "T01_journey_vazia_quota_4_month_permite",
			j:           activeJourney(),
			rules:       []careline.Rule{quotaMonthly(4)},
			intendedAt:  day(1),
			wantAllowed: true,
		},
		{
			name:       "T02_quota_4_month_estourada_janela_retroativa",
			j:          activeJourney(quatroMarcadas...),
			rules:      []careline.Rule{quotaMonthly(4)},
			intendedAt: day(28),
			wantTypes:  []string{careline.RuleQuota},
			check: func(t *testing.T, got careline.Eligibility) {
				require.Len(t, got.Blocks, 1)
				requireAvailableFrom(t, findBlock(got, careline.RuleQuota), day(5).Add(careline.MonthWindow))
			},
		},
		{
			name:       "T03_quota_4_month_janela_para_frente_bloqueia",
			j:          activeJourney(quatroMarcadas...),
			rules:      []careline.Rule{quotaMonthly(4)},
			intendedAt: day(1),
			wantTypes:  []string{careline.RuleQuota},
			check: func(t *testing.T, got careline.Eligibility) {
				require.Len(t, got.Blocks, 1)
				requireAvailableFrom(t, findBlock(got, careline.RuleQuota), day(5).Add(careline.MonthWindow))
			},
		},
		{
			name:       "T04_min_interval_7_bloqueia_vizinho_mais_tardio",
			j:          activeJourney(agendada(5), agendada(12), agendada(19)),
			rules:      []careline.Rule{minInterval(7)},
			intendedAt: day(22),
			wantTypes:  []string{careline.RuleMinInterval},
			check: func(t *testing.T, got careline.Eligibility) {
				require.Len(t, got.Blocks, 1)
				requireAvailableFrom(t, findBlock(got, careline.RuleMinInterval), day(26))
			},
		},
		{
			name: "T05_min_interval_7_distancia_exata_permitida",
			// fora de ordem de propósito: o motor não pode depender da ordenação
			j:           activeJourney(agendada(26), agendada(5), agendada(19)),
			rules:       []careline.Rule{minInterval(7)},
			intendedAt:  day(12),
			wantAllowed: true,
		},
		{
			name: "T06a_cancelada_com_antecedencia_libera_o_slot",
			j: activeJourney(agendada(5), canceladaComAntecedencia(12, 48*time.Hour),
				agendada(19), agendada(26)),
			rules:       []careline.Rule{minInterval(7)},
			intendedAt:  day(12),
			wantAllowed: true,
		},
		{
			name: "T06b_min_interval_valida_contra_vizinhos_restantes",
			j: activeJourney(agendada(5), canceladaComAntecedencia(12, 48*time.Hour),
				agendada(19), agendada(26)),
			rules:      []careline.Rule{minInterval(7)},
			intendedAt: day(13),
			wantTypes:  []string{careline.RuleMinInterval},
			check: func(t *testing.T, got careline.Eligibility) {
				require.Len(t, got.Blocks, 1)
				// bloqueado por d19: AvailableFrom = d19 + 7d = d26
				requireAvailableFrom(t, findBlock(got, careline.RuleMinInterval), day(26))
			},
		},
		{
			name:       "T07_cancelamento_tardio_conta_na_quota",
			j:          activeJourney(canceladaComAntecedencia(5, 2*time.Hour)),
			rules:      []careline.Rule{quotaTotal(1)},
			intendedAt: day(10),
			wantTypes:  []string{careline.RuleQuota},
		},
		{
			name:       "T08_falta_conta_na_quota",
			j:          activeJourney(consulta("psicologia", 5, careline.StatusFalta)),
			rules:      []careline.Rule{quotaTotal(1)},
			intendedAt: day(10),
			wantTypes:  []string{careline.RuleQuota},
		},
		{
			name:       "T09_max_advance_14_bloqueia_now_mais_20d",
			j:          activeJourney(),
			rules:      []careline.Rule{maxAdvance(14)},
			intendedAt: day(21), // now + 20d
			wantTypes:  []string{careline.RuleMaxAdvance},
			check: func(t *testing.T, got careline.Eligibility) {
				require.Len(t, got.Blocks, 1)
				b := findBlock(got, careline.RuleMaxAdvance)
				requireAvailableFrom(t, b, day(7)) // intended − 14d
				assert.Contains(t, b.Reason, "14")
			},
		},
		{
			name:       "T10_prerequisite_sem_realizada_bloqueia_com_label",
			j:          activeJourney(),
			rules:      []careline.Rule{prerequisite("psiquiatria", careline.StatusRealizada, 90)},
			intendedAt: day(1),
			wantTypes:  []string{careline.RulePrerequisite},
			check: func(t *testing.T, got careline.Eligibility) {
				require.Len(t, got.Blocks, 1)
				b := findBlock(got, careline.RulePrerequisite)
				require.NotNil(t, b)
				assert.Nil(t, b.AvailableFrom, "depende de ação, não de tempo")
				assert.Contains(t, b.Reason, "Consulta psiquiátrica", "deve usar o Label do item, não o ref")
			},
		},
		{
			name: "T11_prerequisite_satisfeito_por_realizada_ha_30d",
			j: activeJourney(careline.JourneyAppointment{
				ItemRef: "psiquiatria", Status: careline.StatusRealizada,
				ScheduledAt: day(5).AddDate(0, 0, -30),
			}),
			rules:       []careline.Rule{prerequisite("psiquiatria", careline.StatusRealizada, 90)},
			intendedAt:  day(5),
			wantAllowed: true,
		},
		{
			name:       "T12_quota_total_1_realizada_bloqueio_permanente",
			j:          activeJourney(consulta("psicologia", 5, careline.StatusRealizada)),
			rules:      []careline.Rule{quotaTotal(1)},
			intendedAt: day(20),
			wantTypes:  []string{careline.RuleQuota},
			check: func(t *testing.T, got careline.Eligibility) {
				b := findBlock(got, careline.RuleQuota)
				require.NotNil(t, b)
				assert.Nil(t, b.AvailableFrom, "quota total nunca libera com o tempo")
			},
		},
		{
			name:       "T13_quota_e_min_interval_simultaneos",
			j:          activeJourney(quatroMarcadas...),
			rules:      []careline.Rule{quotaMonthly(4), minInterval(7)},
			intendedAt: day(28),
			wantTypes:  []string{careline.RuleQuota, careline.RuleMinInterval},
		},
		{
			name: "T15_vigencia_ate_d30_intended_d31",
			j: func() careline.Journey {
				j := activeJourney()
				j.ValidUntil = day(30)
				return j
			}(),
			intendedAt: day(31),
			wantTypes:  []string{careline.RuleVigencia},
			check: func(t *testing.T, got careline.Eligibility) {
				require.Len(t, got.Blocks, 1)
				b := findBlock(got, careline.RuleVigencia)
				require.NotNil(t, b)
				assert.Nil(t, b.AvailableFrom, "renovar é ação, não espera")
				assert.Equal(t, "Seu plano vai até 30/08/2026. Renove para agendar além dessa data", b.Reason)
			},
		},
		{
			name: "T16_vigencia_nao_curto_circuita_quota",
			j: func() careline.Journey {
				j := activeJourney(quatroMarcadas...)
				j.ValidUntil = day(30)
				return j
			}(),
			rules:      []careline.Rule{quotaMonthly(4)},
			intendedAt: day(31),
			wantTypes:  []string{careline.RuleVigencia, careline.RuleQuota},
			check: func(t *testing.T, got careline.Eligibility) {
				require.NotEmpty(t, got.Blocks)
				assert.Equal(t, careline.RuleVigencia, got.Blocks[0].RuleType, "VIGENCIA vem primeiro")
			},
		},
		{
			name: "T17_renovacao_antecipada_sem_reset_de_janela",
			j: func() careline.Journey {
				j := activeJourney(quatroMarcadas...)
				j.ValidUntil = day(61) // renovou: avançou de d31
				return j
			}(),
			rules:      []careline.Rule{maxAdvance(40), quotaMonthly(4)},
			intendedAt: day(35), // janela retroativa pega só d12,d19,d26 = 3 < 4
			// d05 está a exatamente 30d: fora de qualquer janela de 30d com d35
			wantAllowed: true,
		},
		{
			name: "T18_janela_movel_ignora_fronteira_de_ciclo",
			j: func() careline.Journey {
				j := activeJourney(agendada(12), agendada(19), agendada(26), agendada(33))
				j.ValidUntil = day(61)
				return j
			}(),
			rules:      []careline.Rule{quotaMonthly(4)},
			intendedAt: day(29),
			wantTypes:  []string{careline.RuleQuota},
			check: func(t *testing.T, got careline.Eligibility) {
				// a janela [d12, d42) contém d29 e as 4 consultas
				requireAvailableFrom(t, findBlock(got, careline.RuleQuota), day(12).Add(careline.MonthWindow))
			},
		},
		{
			name: "T19_matricula_expirada_bloqueia_vigencia",
			j: func() careline.Journey {
				j := activeJourney()
				j.Status = careline.EnrollmentExpirada
				return j
			}(),
			intendedAt: day(10),
			wantTypes:  []string{careline.RuleVigencia},
			check: func(t *testing.T, got careline.Eligibility) {
				b := findBlock(got, careline.RuleVigencia)
				require.NotNil(t, b)
				assert.Nil(t, b.AvailableFrom)
				assert.Contains(t, b.Reason, "expirou")
			},
		},

		// --- Casos além da tabela (fronteiras que a tabela não pinça) ---
		{
			name: "X01_vigencia_antes_do_inicio_available_from_valid_from",
			j: func() careline.Journey {
				j := activeJourney()
				j.ValidFrom = day(10)
				return j
			}(),
			intendedAt: day(5),
			wantTypes:  []string{careline.RuleVigencia},
			check: func(t *testing.T, got careline.Eligibility) {
				b := findBlock(got, careline.RuleVigencia)
				requireAvailableFrom(t, b, day(10))
				assert.Contains(t, b.Reason, "10/08/2026")
			},
		},
		{
			name: "X02_params_invalidos_em_runtime_falham_fechado",
			j:    activeJourney(),
			rules: []careline.Rule{{Type: careline.RuleQuota,
				Params: json.RawMessage(`{"max":0,"period":"month"}`)}},
			intendedAt: day(5),
			wantTypes:  []string{careline.RuleQuota},
			check: func(t *testing.T, got careline.Eligibility) {
				b := findBlock(got, careline.RuleQuota)
				require.NotNil(t, b)
				assert.Equal(t, "Regra inválida na configuração da linha de cuidado", b.Reason)
				assert.Nil(t, b.AvailableFrom)
			},
		},
		{
			name: "X03_matricula_pausada_bloqueia_vigencia",
			j: func() careline.Journey {
				j := activeJourney()
				j.Status = careline.EnrollmentPausada
				return j
			}(),
			intendedAt: day(10),
			wantTypes:  []string{careline.RuleVigencia},
			check: func(t *testing.T, got careline.Eligibility) {
				b := findBlock(got, careline.RuleVigencia)
				require.NotNil(t, b)
				assert.Equal(t, "Sua matrícula está pausada", b.Reason)
			},
		},
		{
			name: "X04_prerequisite_futuro_nao_satisfaz",
			// A consulta do pré-requisito está agendada 35d DEPOIS do horário
			// pretendido: "realize primeiro" é uma janela retroativa, então uma
			// consulta futura não pode satisfazê-la.
			j: activeJourney(careline.JourneyAppointment{
				ItemRef: "psiquiatria", Status: careline.StatusRealizada,
				ScheduledAt: day(5).AddDate(0, 0, 35),
			}),
			rules:      []careline.Rule{prerequisite("psiquiatria", careline.StatusRealizada, 90)},
			intendedAt: day(5),
			wantTypes:  []string{careline.RulePrerequisite},
			check: func(t *testing.T, got careline.Eligibility) {
				b := findBlock(got, careline.RulePrerequisite)
				require.NotNil(t, b)
				assert.Nil(t, b.AvailableFrom, "depende de ação, não de tempo")
			},
		},
		{
			name: "X05_quota_janela_super_lotada_available_from_pela_max_esima",
			// count > max na janela (config reduzida/dados migrados): a vaga abre
			// quando a (max)-ésima consulta mais recente envelhece — d06 + 30d —,
			// NÃO a mais antiga (d05). d05+30d ainda estaria bloqueado.
			j:          activeJourney(agendada(5), agendada(6)),
			rules:      []careline.Rule{quotaMonthly(1)},
			intendedAt: day(7),
			wantTypes:  []string{careline.RuleQuota},
			check: func(t *testing.T, got careline.Eligibility) {
				requireAvailableFrom(t, findBlock(got, careline.RuleQuota), day(6).Add(careline.MonthWindow))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := tt.item
			if item.Ref == "" {
				item = psicologia
			}
			now := tt.now
			if now.IsZero() {
				now = day(1)
			}

			got := careline.Evaluate(tt.j, item, tt.rules, tt.intendedAt, now)

			assert.Equalf(t, tt.wantAllowed, got.Allowed, "Allowed; blocks: %+v", got.Blocks)
			if tt.wantAllowed {
				assert.Empty(t, got.Blocks, "Allowed exige Blocks vazio")
			} else {
				assert.NotEmpty(t, got.Blocks, "bloqueado exige pelo menos um Block")
			}
			for _, rt := range tt.wantTypes {
				assert.NotNilf(t, findBlock(got, rt), "esperava Block %s; blocks: %+v", rt, got.Blocks)
			}
			for _, b := range got.Blocks {
				assert.NotEmptyf(t, b.Reason, "todo Block precisa de Reason exibível (%s)", b.RuleType)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

// T14 — cenário-alvo do produto: QUOTA 4/month + MIN_INTERVAL 7, agendando
// semana a semana. Cada avaliação vê as consultas já feitas na journey.
func TestEvaluate_T14_CenarioAlvoSequencial(t *testing.T) {
	rules := []careline.Rule{quotaMonthly(4), minInterval(7)}
	j := activeJourney()
	now := day(1)

	for _, n := range []int{5, 12, 19, 26} {
		got := careline.Evaluate(j, psicologia, rules, day(n), now)
		require.Truef(t, got.Allowed, "dia %d deveria ser permitido; blocks: %+v", n, got.Blocks)
		j.Appointments = append(j.Appointments, agendada(n))
	}

	// d28: a quota do período estourou
	got := careline.Evaluate(j, psicologia, rules, day(28), now)
	require.False(t, got.Allowed)
	assert.NotNilf(t, findBlock(got, careline.RuleQuota), "d28 deveria bloquear por QUOTA; blocks: %+v", got.Blocks)

	// d15: no meio da sequência viola QUOTA e MIN_INTERVAL ao mesmo tempo
	got = careline.Evaluate(j, psicologia, rules, day(15), now)
	require.False(t, got.Allowed)
	assert.NotNilf(t, findBlock(got, careline.RuleQuota), "d15: QUOTA; blocks: %+v", got.Blocks)
	assert.NotNilf(t, findBlock(got, careline.RuleMinInterval), "d15: MIN_INTERVAL; blocks: %+v", got.Blocks)
}
