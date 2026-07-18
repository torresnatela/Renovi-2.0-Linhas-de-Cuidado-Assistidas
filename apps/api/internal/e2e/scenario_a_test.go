//go:build integration

package e2e

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/testsupport"
)

// TestE2E_A_SaudeMentalBasica percorre o cenário A do marco: a linha "Saúde
// Mental Básica de teste" (psicologia QUOTA 4/mês + MIN_INTERVAL 7d +
// MAX_ADVANCE 30d; psiquiatria QUOTA 1/mês), com o paciente 1 esgotando a cota,
// esbarrando em cada regra, cancelando, renovando e auditando tudo no fim.
//
// A sequência importa: cada passo depende do estado deixado pelo anterior, e o
// runner aborta a sequência no primeiro passo que falhar.
//
// Horários (todos 09:00/09:30 de hoje+D, America/Sao_Paulo): as 4 de psicologia
// entram em +2 09:00, +9 09:00, +16 09:30 e +23 09:30 — adjacentes a EXATOS 7d
// ou mais (MIN_INTERVAL permite a distância exata), e o reagendamento do +9 às
// 09:30 (passo 15) continua a >= 7d dos vizinhos (+2 09:00 e +16 09:30).
func TestE2E_A_SaudeMentalBasica(t *testing.T) {
	seedSlots(t)
	step := steps(t)
	ctx := context.Background()

	const (
		lineCode = "saude-mental-basica"
		lineName = "Saúde Mental Básica de teste"
	)

	patient := newPatientClient(t)

	var (
		accountID string
		line      api.CareLine
		items     = map[string]api.CareLineItem{}    // por ref: psico, psiq
		appts     = map[string]api.CareAppointment{} // por apelido: psico+2, ...
		keys      = map[string]string{}
		enr       api.Enrollment // como criada (passo 5)
		enr18     api.Enrollment // depois da renovação contígua (passo 18)
	)

	book := func(t *testing.T, item api.CareLineItem, slotID, key string) (int, []byte) {
		t.Helper()
		return doReq(t, patient, "POST", "/me/appointments",
			map[string]string{"Idempotency-Key": key},
			map[string]any{"item_id": item.Id, "slot_id": slotID})
	}

	step("A01_admin_sem_token_401", func(t *testing.T) {
		status, raw := doReq(t, plainClient, "GET", "/admin/care-lines", nil, nil)
		require.Equal(t, http.StatusUnauthorized, status, "corpo: %s", raw)
		require.Equal(t, "ADMIN_TOKEN_INVALID", reasonCode(problemOf(t, raw)))

		status, raw = doReq(t, plainClient, "GET", "/admin/care-lines",
			map[string]string{"X-Admin-Token": "token-errado"}, nil)
		require.Equal(t, http.StatusUnauthorized, status, "corpo: %s", raw)
		require.Equal(t, "ADMIN_TOKEN_INVALID", reasonCode(problemOf(t, raw)))
	})

	step("A02_admin_monta_e_publica_linha", func(t *testing.T) {
		status, raw := adminDo(t, "POST", "/admin/care-lines", map[string]any{
			"code": lineCode, "name": lineName, "description": "linha do E2E do slice 1",
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		draft := decodeAs[api.CareLine](t, raw)
		require.Equal(t, "draft", string(draft.Status))

		base := "/admin/care-lines/" + draft.Id.String()
		for _, item := range []map[string]any{
			{"ref": "psico", "kind": "CONSULTA", "label": "Psicoterapia", "specialty_code": "Psicologia"},
			{"ref": "psiq", "kind": "CONSULTA", "label": "Consulta psiquiátrica", "specialty_code": "Psiquiatria"},
		} {
			status, raw := adminDo(t, "POST", base+"/items", item)
			require.Equal(t, http.StatusCreated, status, "item %v: %s", item["ref"], raw)
		}
		for _, rule := range []struct {
			ref  string
			body map[string]any
		}{
			{"psico", map[string]any{"rule_type": "QUOTA", "params": map[string]any{"max": 4, "period": "month", "window": "rolling"}}},
			{"psico", map[string]any{"rule_type": "MIN_INTERVAL", "params": map[string]any{"days": 7}}},
			{"psico", map[string]any{"rule_type": "MAX_ADVANCE", "params": map[string]any{"days": 30}}},
			{"psiq", map[string]any{"rule_type": "QUOTA", "params": map[string]any{"max": 1, "period": "month"}}},
		} {
			status, raw := adminDo(t, "POST", base+"/items/"+rule.ref+"/rules", rule.body)
			require.Equal(t, http.StatusCreated, status, "regra em %s: %s", rule.ref, raw)
		}

		status, raw = adminDo(t, "POST", base+"/publish", nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		line = decodeAs[api.CareLine](t, raw)
		require.Equal(t, "published", string(line.Status))
		require.NotNil(t, line.PublishedAt)
		require.Len(t, line.Items, 2)
		for _, it := range line.Items {
			items[it.Ref] = it
		}

		// Publicada é imutável: item novo → 409.
		status, raw = adminDo(t, "POST", base+"/items", map[string]any{
			"ref": "extra", "kind": "CONSULTA", "label": "Extra", "specialty_code": "Psicologia",
		})
		require.Equal(t, http.StatusConflict, status, "corpo: %s", raw)
		require.Equal(t, "CARE_LINE_PUBLISHED", reasonCode(problemOf(t, raw)))
	})

	step("A03_publish_reprova_com_todos_os_erros", func(t *testing.T) {
		status, raw := adminDo(t, "POST", "/admin/care-lines", map[string]any{
			"code": "linha-invalida", "name": "Linha inválida de teste",
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		bad := decodeAs[api.CareLine](t, raw)
		base := "/admin/care-lines/" + bad.Id.String()

		status, raw = adminDo(t, "POST", base+"/items", map[string]any{
			"ref": "a", "kind": "CONSULTA", "label": "Item A", "specialty_code": "INEXISTENTE",
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		status, raw = adminDo(t, "POST", base+"/items", map[string]any{
			"ref": "b", "kind": "CONSULTA", "label": "Item B", "specialty_code": "Psicologia",
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		itemB := decodeAs[api.CareLineItem](t, raw)

		// Ciclo a -> b -> a.
		status, raw = adminDo(t, "POST", base+"/items/a/rules", map[string]any{
			"rule_type": "PREREQUISITE",
			"params":    map[string]any{"item_ref": "b", "status": "realizada", "within_days": 30},
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		status, raw = adminDo(t, "POST", base+"/items/b/rules", map[string]any{
			"rule_type": "PREREQUISITE",
			"params":    map[string]any{"item_ref": "a", "status": "realizada", "within_days": 30},
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)

		// QUOTA window=calendar: a API já barra na INSERÇÃO (validação em
		// profundidade — o AddRule usa o MESMO ParseRuleParams do motor)...
		status, raw = adminDo(t, "POST", base+"/items/b/rules", map[string]any{
			"rule_type": "QUOTA",
			"params":    map[string]any{"max": 1, "period": "month", "window": "calendar"},
		})
		require.Equal(t, http.StatusBadRequest, status, "corpo: %s", raw)
		pAdd := problemOf(t, raw)
		require.NotNil(t, pAdd.Errors)
		require.Contains(t, strings.Join(*pAdd.Errors, "\n"), "calendar")

		// ...então, para provar que o PUBLISH revalida o CONJUNTO (a segunda
		// linha de defesa), a regra inválida entra por baixo, direto no banco
		// (superDSN) — simulando config que envelheceu por fora da API.
		ruleID, err := uuid.NewV7()
		require.NoError(t, err)
		_, err = env.super.Exec(ctx, `
			INSERT INTO care_line_rule (id, care_line_item_id, rule_type, params)
			VALUES ($1, $2, 'QUOTA', '{"max":1,"period":"month","window":"calendar"}')`,
			ruleID, itemB.Id)
		require.NoError(t, err, "injetar regra inválida via superusuário")

		status, raw = adminDo(t, "POST", base+"/publish", nil)
		require.Equal(t, http.StatusBadRequest, status, "corpo: %s", raw)
		p := problemOf(t, raw)
		require.NotNil(t, p.Errors, "o 400 do publish precisa da lista errors[]")
		all := strings.Join(*p.Errors, "\n")
		require.Contains(t, all, "INEXISTENTE")
		require.Contains(t, all, "não encontrada no legado")
		require.Contains(t, all, "calendar")
		require.Contains(t, all, "ciclo de pré-requisitos")
	})

	step("A04_paciente1_registra_e_loga", func(t *testing.T) {
		status, raw := doReq(t, patient, "POST", "/auth/register", nil, map[string]any{
			"full_name":  "Paciente Um E2E",
			"cpf":        "948.190.898-46", // CPF válido dos testes de auth
			"birth_date": "1990-03-10",
			"email":      "paciente1.e2e@example.com",
			"phone":      "11999990001",
			"password":   "cavalo-bateria-grampo-1",
			"address": map[string]any{
				"zip_code": "01310-100", "street": "Av. Paulista", "number": "1000",
				"neighborhood": "Bela Vista", "city": "São Paulo", "state": "SP",
			},
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		acc := decodeAs[api.Account](t, raw)
		accountID = acc.Id.String()
		require.NotEmpty(t, accountID)

		status, raw = doReq(t, patient, "POST", "/auth/login", nil, map[string]any{
			"cpf": "94819089846", "password": "cavalo-bateria-grampo-1",
		})
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)

		// O cookie de sessão vale: /me responde 200 com a conta.
		status, raw = doReq(t, patient, "GET", "/me", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		require.Equal(t, accountID, decodeAs[api.Account](t, raw).Id.String())
	})

	step("A05_admin_matricula_meses1_e_409_na_segunda", func(t *testing.T) {
		before := time.Now()
		status, raw := adminDo(t, "POST", "/admin/enrollments", map[string]any{
			"patient_id": accountID, "care_line_code": lineCode, "months": 1,
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		enr = decodeAs[api.Enrollment](t, raw)
		require.Equal(t, "ativa", string(enr.Status))
		require.Equal(t, lineCode, enr.CareLineCode)

		// Vigência: valid_from = agora (tolerância p/ relógio do servidor);
		// valid_until = valid_from + 30d EXATOS (relação entre si, sem tolerância).
		require.WithinDuration(t, before, enr.ValidFrom, 5*time.Second)
		require.Equal(t, 30*24*time.Hour, enr.ValidUntil.Sub(enr.ValidFrom),
			"months=1 => vigência de exatamente 30 dias")
		require.Len(t, enr.Periods, 1)
		requireInstant(t, enr.ValidFrom, enr.Periods[0].StartsAt, "período 1 começa no valid_from")
		requireInstant(t, enr.ValidUntil, enr.Periods[0].EndsAt, "período 1 termina no valid_until")

		// Matricular DE NOVO na mesma linha → 409.
		status, raw = adminDo(t, "POST", "/admin/enrollments", map[string]any{
			"patient_id": accountID, "care_line_code": lineCode, "months": 1,
		})
		require.Equal(t, http.StatusConflict, status, "corpo: %s", raw)
		require.Equal(t, "ENROLLMENT_ALIVE", reasonCode(problemOf(t, raw)))
	})

	step("A06_jornada_dois_itens_liberados", func(t *testing.T) {
		status, raw := doReq(t, patient, "GET", "/me/journey", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		j := decodeAs[api.Journey](t, raw)
		require.Len(t, j.Enrollments, 1)
		je := j.Enrollments[0]
		require.Equal(t, "ativa", string(je.Enrollment.Status))
		require.Equal(t, lineName, je.CareLineName)
		require.Len(t, je.Items, 2)
		for _, it := range je.Items {
			require.Truef(t, it.Eligibility.Allowed, "item %s deveria estar liberado: %+v",
				it.Item.Ref, it.Eligibility.Blocks)
		}
	})

	step("A07_disponibilidade_anotada_slot2_allowed", func(t *testing.T) {
		status, raw := doReq(t, patient, "GET",
			"/me/availability?item_id="+items["psico"].Id.String(), nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		page := decodeAs[api.AvailabilityPage](t, raw)
		require.NotEmpty(t, page.Items)
		for _, s := range page.Items {
			require.Equal(t, anaID, s.Professional.Id,
				"só a Ana tem horário futuro de Psicologia no mock")
		}

		target := anaSlots[2][0] // hoje+2, 09:00
		var found *api.AnnotatedSlot
		for i := range page.Items {
			if page.Items[i].Id == target.ID {
				found = &page.Items[i]
				break
			}
		}
		require.NotNil(t, found, "o slot semeado de hoje+2 não apareceu na disponibilidade")
		requireInstant(t, target.StartsAt, found.StartsAt,
			"o instante do slot round-tripa exato pelo fuso do legado")
		require.Equal(t, "America/Sao_Paulo", found.TimeZone)
		require.True(t, found.Eligibility.Allowed, "blocks: %+v", found.Eligibility.Blocks)
	})

	step("A08_agenda_as_4_de_psicologia", func(t *testing.T) {
		for _, b := range []struct {
			name string
			slot testsupport.SeededSlot
		}{
			{"psico+2", anaSlots[2][0]},   // 09:00
			{"psico+9", anaSlots[9][0]},   // 09:00 (exatos 7d do +2 — permitido)
			{"psico+16", anaSlots[16][1]}, // 09:30
			{"psico+23", anaSlots[23][1]}, // 09:30 (exatos 7d do +16 — permitido)
		} {
			key := idemKey(t)
			keys[b.name] = key
			status, raw := book(t, items["psico"], b.slot.ID, key)
			require.Equal(t, http.StatusCreated, status, "agendar %s: %s", b.name, raw)
			appt := decodeAs[api.CareAppointment](t, raw)
			require.Equal(t, "agendada", string(appt.Status))
			require.Equal(t, "psico", appt.ItemRef)
			requireInstant(t, b.slot.StartsAt, appt.ScheduledAt, b.name)
			require.Equal(t, "America/Sao_Paulo", appt.TimeZone)
			appts[b.name] = appt
		}
	})

	step("A09_replay_mesma_key_200_mesmo_id", func(t *testing.T) {
		status, raw := book(t, items["psico"], anaSlots[23][1].ID, keys["psico+23"])
		require.Equal(t, http.StatusOK, status, "replay devolve 200, não 201: %s", raw)
		replay := decodeAs[api.CareAppointment](t, raw)
		require.Equal(t, appts["psico+23"].Id, replay.Id, "a MESMA consulta, sem criar outra")

		status, raw = doReq(t, patient, "GET", "/me/appointments", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		list := decodeAs[api.CareAppointmentList](t, raw)
		require.Len(t, list.Items, 4, "o replay não pode ter criado uma quinta consulta")
		for _, a := range list.Items {
			require.Equal(t, "psico", a.ItemRef)
			require.Equal(t, "agendada", string(a.Status))
		}
	})

	step("A10_sem_idempotency_key_400", func(t *testing.T) {
		status, raw := doReq(t, patient, "POST", "/me/appointments", nil,
			map[string]any{"item_id": items["psico"].Id, "slot_id": anaSlots[30][0].ID})
		require.Equal(t, http.StatusBadRequest, status, "corpo: %s", raw)
		require.Equal(t, "IDEMPOTENCY_KEY_REQUIRED", reasonCode(problemOf(t, raw)))
	})

	step("A11_quinta_no_periodo_quota_422", func(t *testing.T) {
		status, raw := book(t, items["psico"], anaSlots[30][0].ID, idemKey(t))
		require.Equal(t, http.StatusUnprocessableEntity, status, "corpo: %s", raw)
		p := problemOf(t, raw)
		require.Equal(t, "ELIGIBILITY_BLOCKED", reasonCode(p))
		quota := requireBlock(t, blocksOf(t, p), "QUOTA")
		require.NotNil(t, quota.AvailableFrom)
		// A vaga abre quando a consulta mais antiga da janela (a de +2) sai dela.
		requireInstant(t, anaSlots[2][0].StartsAt.Add(30*24*time.Hour), *quota.AvailableFrom,
			"available_from da QUOTA = scheduled(+2) + 30d")
	})

	step("A12_vizinho_a_menos_de_7d_min_interval_422", func(t *testing.T) {
		status, raw := book(t, items["psico"], anaSlots[10][0].ID, idemKey(t))
		require.Equal(t, http.StatusUnprocessableEntity, status, "corpo: %s", raw)
		p := problemOf(t, raw)
		require.Equal(t, "ELIGIBILITY_BLOCKED", reasonCode(p))
		mi := requireBlock(t, blocksOf(t, p), "MIN_INTERVAL")
		require.NotNil(t, mi.AvailableFrom)
		// O vizinho conflitante MAIS TARDIO é o de +16 09:30 → libera 7d depois.
		requireInstant(t, anaSlots[16][1].StartsAt.Add(7*24*time.Hour), *mi.AvailableFrom,
			"available_from do MIN_INTERVAL = vizinho mais tardio + 7d")
	})

	step("A13_slot_tomado_409", func(t *testing.T) {
		status, raw := book(t, items["psico"], anaSlots[2][0].ID, idemKey(t))
		require.Equal(t, http.StatusConflict, status, "corpo: %s", raw)
		require.Equal(t, "SLOT_TAKEN", reasonCode(problemOf(t, raw)))
	})

	step("A14_cancela_a_de_mais9_com_antecedencia", func(t *testing.T) {
		status, raw := doReq(t, patient, "POST",
			"/me/appointments/"+appts["psico+9"].Id.String()+"/cancel", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		cancelled := decodeAs[api.CareAppointment](t, raw)
		require.Equal(t, "cancelada", string(cancelled.Status))
		require.NotNil(t, cancelled.CancelledAt)
		// O payload do evento (counts_for_quota=false + falha do cancel na DAV,
		// que o fake responde 500 sempre) é conferido no A22, via /me/audit.
	})

	step("A15_reagenda_no_mesmo_dia_201", func(t *testing.T) {
		key := idemKey(t)
		status, raw := book(t, items["psico"], anaSlots[9][1].ID, key) // +9, 09:30
		require.Equal(t, http.StatusCreated, status,
			"a cancelada com antecedência não conta (T6a): %s", raw)
		appt := decodeAs[api.CareAppointment](t, raw)
		require.Equal(t, "agendada", string(appt.Status))
		requireInstant(t, anaSlots[9][1].StartsAt, appt.ScheduledAt, "psico+9 reagendada")
		appts["psico+9b"] = appt
	})

	step("A16_psiquiatria_mais5_ok_mais37_so_vigencia", func(t *testing.T) {
		status, raw := book(t, items["psiq"], brunoSlots[5][0].ID, idemKey(t))
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		appts["psiq+5"] = decodeAs[api.CareAppointment](t, raw)

		status, raw = book(t, items["psiq"], brunoSlots[37][0].ID, idemKey(t))
		require.Equal(t, http.StatusUnprocessableEntity, status, "corpo: %s", raw)
		blocks := blocksOf(t, problemOf(t, raw))
		// SÓ vigência: psiquiatria não tem MAX_ADVANCE, e a QUOTA 1/mês não pega
		// (nenhuma janela de 30d contém +5 e +37 ao mesmo tempo).
		require.Len(t, blocks, 1, "esperava exatamente [VIGENCIA]: %+v", blocks)
		require.Equal(t, "VIGENCIA", string(blocks[0].RuleType))
		require.Nil(t, blocks[0].AvailableFrom, "renovar é AÇÃO do admin, não espera — sem available_from")
	})

	step("A17_mais44_max_advance_e_vigencia", func(t *testing.T) {
		status, raw := book(t, items["psico"], anaSlots[44][0].ID, idemKey(t))
		require.Equal(t, http.StatusUnprocessableEntity, status, "corpo: %s", raw)
		blocks := blocksOf(t, problemOf(t, raw))
		ma := requireBlock(t, blocks, "MAX_ADVANCE")
		require.NotNil(t, ma.AvailableFrom)
		requireInstant(t, anaSlots[44][0].StartsAt.Add(-30*24*time.Hour), *ma.AvailableFrom,
			"available_from do MAX_ADVANCE = intended - 30d")
		requireBlock(t, blocks, "VIGENCIA") // +44 também passa do valid_until
	})

	step("A18_renova_contiguo_mais30d", func(t *testing.T) {
		status, raw := adminDo(t, "POST", "/admin/enrollments/"+enr.Id.String()+"/renew",
			map[string]any{"months": 1})
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		enr18 = decodeAs[api.Enrollment](t, raw)
		require.Equal(t, "ativa", string(enr18.Status))
		requireInstant(t, enr.ValidUntil.Add(30*24*time.Hour), enr18.ValidUntil,
			"valid_until avança exatamente 30d")
		require.Len(t, enr18.Periods, 2)

		// O período novo é CONTÍGUO: começa EXATAMENTE no valid_until antigo.
		var novo *api.EnrollmentPeriod
		for i := range enr18.Periods {
			if enr18.Periods[i].StartsAt.Equal(enr.ValidUntil) {
				novo = &enr18.Periods[i]
				break
			}
		}
		require.NotNil(t, novo, "esperava um período começando no valid_until antigo: %+v", enr18.Periods)
		requireInstant(t, enr18.ValidUntil, novo.EndsAt, "o período novo termina no novo valid_until")
	})

	step("A19_mais37_agora_201_e_mais44_segue_max_advance", func(t *testing.T) {
		// A primeira consulta do ciclo 2, agendada ainda no ciclo 1.
		status, raw := book(t, items["psiq"], brunoSlots[37][0].ID, idemKey(t))
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		appts["psiq+37"] = decodeAs[api.CareAppointment](t, raw)
		requireInstant(t, brunoSlots[37][0].StartsAt, appts["psiq+37"].ScheduledAt, "psiq+37")

		// Sem "reset": +44 continua barrado por MAX_ADVANCE (mas VIGENCIA sumiu).
		status, raw = book(t, items["psico"], anaSlots[44][0].ID, idemKey(t))
		require.Equal(t, http.StatusUnprocessableEntity, status, "corpo: %s", raw)
		blocks := blocksOf(t, problemOf(t, raw))
		require.Len(t, blocks, 1, "esperava exatamente [MAX_ADVANCE]: %+v", blocks)
		require.Equal(t, "MAX_ADVANCE", string(blocks[0].RuleType))
		require.NotNil(t, blocks[0].AvailableFrom)
		requireInstant(t, anaSlots[44][0].StartsAt.Add(-30*24*time.Hour), *blocks[0].AvailableFrom,
			"available_from do MAX_ADVANCE não muda com a renovação")
	})

	step("A20_force_status_realizada", func(t *testing.T) {
		status, raw := doReq(t, plainClient, "POST",
			"/internal/appointments/"+appts["psico+2"].Id.String()+"/force-status",
			nil, map[string]any{"status": "realizada"})
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		forced := decodeAs[api.CareAppointment](t, raw)
		require.Equal(t, "realizada", string(forced.Status))

		status, raw = doReq(t, patient, "GET", "/me/appointments?status=realizada", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		list := decodeAs[api.CareAppointmentList](t, raw)
		require.Len(t, list.Items, 1)
		require.Equal(t, appts["psico+2"].Id, list.Items[0].Id)
	})

	step("A21_expiracao_lazy_e_reativacao", func(t *testing.T) {
		// (a) Vence a vigência POR BAIXO (superDSN), status segue 'ativa'. O CHECK
		// vigencia_valida exige valid_from < valid_until, então recua os dois —
		// o mesmo truque do teste do JourneyRepo.Expire.
		tag, err := env.super.Exec(ctx, `
			UPDATE enrollment
			   SET valid_from = now() - interval '60 days',
			       valid_until = now() - interval '1 hour',
			       updated_at = now()
			 WHERE id = $1`, enr.Id)
		require.NoError(t, err)
		require.EqualValues(t, 1, tag.RowsAffected())
		var st string
		require.NoError(t, env.super.QueryRow(ctx,
			`SELECT status FROM enrollment WHERE id = $1`, enr.Id).Scan(&st))
		require.Equal(t, "ativa", st, "o UPDATE não transiciona: quem expira é a leitura lazy")

		// (b) A leitura da jornada expira lazy.
		status, raw := doReq(t, patient, "GET", "/me/journey", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		j := decodeAs[api.Journey](t, raw)
		require.Len(t, j.Enrollments, 1)
		require.Equal(t, "expirada", string(j.Enrollments[0].Enrollment.Status))

		// (c) ...e grava o evento com actor=sistema.
		status, raw = doReq(t, patient, "GET", "/me/audit?limit=1", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		page := decodeAs[api.AuditPage](t, raw)
		require.Len(t, page.Items, 1)
		require.Equal(t, "matricula_expirada", string(page.Items[0].EventType))
		require.Equal(t, "sistema", string(page.Items[0].Actor))

		// (d) Agendar com a matrícula expirada → 422 com VIGENCIA (motivo: expirou).
		status, raw = book(t, items["psico"], anaSlots[10][0].ID, idemKey(t))
		require.Equal(t, http.StatusUnprocessableEntity, status, "corpo: %s", raw)
		vig := requireBlock(t, blocksOf(t, problemOf(t, raw)), "VIGENCIA")
		require.Contains(t, vig.Reason, "expirou")
		require.Nil(t, vig.AvailableFrom)

		// (e) Renovar REATIVA: status ativa e o período novo começa AGORA (não é
		// contíguo ao valid_until vencido). months=3 para a checagem (g) caber
		// na vigência.
		before := time.Now()
		status, raw = adminDo(t, "POST", "/admin/enrollments/"+enr.Id.String()+"/renew",
			map[string]any{"months": 3})
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		renewed := decodeAs[api.Enrollment](t, raw)
		require.Equal(t, "ativa", string(renewed.Status))
		require.Len(t, renewed.Periods, 3)
		// O período da reativação é o que termina no NOVO valid_until (não é o de
		// StartsAt mais tardio: o período contíguo do A18 começa em +30d, DEPOIS
		// de "agora").
		var novo *api.EnrollmentPeriod
		for i := range renewed.Periods {
			if renewed.Periods[i].EndsAt.Equal(renewed.ValidUntil) {
				novo = &renewed.Periods[i]
				break
			}
		}
		require.NotNil(t, novo, "esperava um período terminando no novo valid_until: %+v", renewed.Periods)
		require.WithinDuration(t, before, novo.StartsAt, 5*time.Second,
			"reativação: o período começa agora, não no valid_until vencido")
		requireInstant(t, novo.StartsAt.Add(3*30*24*time.Hour), renewed.ValidUntil,
			"valid_until = início da reativação + 90d")

		// (f) Agendar voltou a funcionar NO QUE DEPENDE DA MATRÍCULA: a
		// elegibilidade de amanhã não tem mais VIGENCIA — sobram só as regras de
		// cadência (QUOTA/MIN_INTERVAL), que seguem cheias de verdade (as 4 de
		// psicologia dos últimos 21 dias). Ver o relatório: allowed=true para
		// psicologia é INATINGÍVEL aqui — a QUOTA só libera em scheduled(+2)+30d,
		// e o MAX_ADVANCE corta em now+30d, que vem antes.
		tomorrow := time.Now().In(env.loc).Add(24 * time.Hour)
		q := url.Values{}
		q.Set("item_id", items["psico"].Id.String())
		q.Set("date", tomorrow.Format(time.RFC3339))
		status, raw = doReq(t, patient, "GET", "/me/eligibility?"+q.Encode(), nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		elig := decodeAs[api.Eligibility](t, raw)
		require.False(t, elig.Allowed)
		require.NotEmpty(t, elig.Blocks)
		for _, b := range elig.Blocks {
			require.Contains(t, []string{"QUOTA", "MIN_INTERVAL"}, string(b.RuleType),
				"depois de renovar, só as regras de cadência podem barrar")
		}

		// (g) E uma data DE FATO livre volta a ser allowed=true: psiquiatria a
		// 31d da consulta de +37 (fora de qualquer janela de cota) e dentro da
		// vigência renovada.
		free := brunoSlots[37][0].StartsAt.Add(31 * 24 * time.Hour)
		q = url.Values{}
		q.Set("item_id", items["psiq"].Id.String())
		q.Set("date", free.Format(time.RFC3339))
		status, raw = doReq(t, patient, "GET", "/me/eligibility?"+q.Encode(), nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		elig = decodeAs[api.Eligibility](t, raw)
		require.True(t, elig.Allowed, "blocks: %+v", elig.Blocks)
		require.Empty(t, elig.Blocks)
	})

	step("A22_auditoria_integra_e_paginada", func(t *testing.T) {
		status, raw := doReq(t, patient, "GET", "/me/audit?limit=50", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		full := decodeAs[api.AuditPage](t, raw)
		require.Nil(t, full.NextCursor, "13 eventos cabem numa página de 50")

		// A história inteira, em ordem DESC (occurred_at, id).
		want := []string{
			"matricula_renovada",      // A21e (reativação)
			"matricula_expirada",      // A21b (lazy, actor=sistema)
			"consulta_status_forcado", // A20
			"consulta_agendada",       // A19 psiq +37
			"matricula_renovada",      // A18 (contígua)
			"consulta_agendada",       // A16 psiq +5
			"consulta_agendada",       // A15 psico +9 (09:30)
			"consulta_cancelada",      // A14
			"consulta_agendada",       // A08 psico +23
			"consulta_agendada",       // A08 psico +16
			"consulta_agendada",       // A08 psico +9
			"consulta_agendada",       // A08 psico +2
			"matricula_criada",        // A05
		}
		require.Len(t, full.Items, len(want), "total de eventos da jornada")
		for i, ev := range full.Items {
			require.Equalf(t, want[i], string(ev.EventType), "evento [%d]", i)
		}

		// Ordem DESC estável por (occurred_at, id).
		for i := 1; i < len(full.Items); i++ {
			prev, cur := full.Items[i-1], full.Items[i]
			require.False(t, cur.OccurredAt.After(prev.OccurredAt), "occurred_at fora de ordem em [%d]", i)
			if cur.OccurredAt.Equal(prev.OccurredAt) {
				require.Less(t, cur.Id.String(), prev.Id.String(), "empate sem desempate por id em [%d]", i)
			}
		}

		// Payloads que são contrato de auditoria.
		reactivation := full.Items[0].Payload
		require.Equal(t, true, reactivation["reactivated"])

		expired := full.Items[1]
		require.Equal(t, "sistema", string(expired.Actor))

		forced := full.Items[2]
		require.Equal(t, "admin", string(forced.Actor))
		require.Equal(t, "agendada", forced.Payload["from"])
		require.Equal(t, "realizada", forced.Payload["to"])

		renew1 := full.Items[4].Payload
		require.Equal(t, false, renew1["reactivated"])
		require.NotEmpty(t, renew1["period_id"])
		renewUntil, err := time.Parse(time.RFC3339Nano, renew1["valid_until"].(string))
		require.NoError(t, err)
		requireInstant(t, enr18.ValidUntil, renewUntil, "valid_until do evento de renovação")

		cancelled := full.Items[7]
		require.Equal(t, "paciente", string(cancelled.Actor))
		require.Equal(t, "paciente", cancelled.Payload["cancelled_by"])
		require.Equal(t, false, cancelled.Payload["counts_for_quota"],
			"cancelamento com antecedência devolve a vaga")
		hours, ok := cancelled.Payload["hours_before"].(float64)
		require.True(t, ok, "hours_before: %v", cancelled.Payload["hours_before"])
		require.Greater(t, hours, 24.0)
		require.Equal(t, false, cancelled.Payload["dav_cancelled"],
			"o cancel na DAV falhou (o fake responde 500 sempre) e foi TOLERADO")
		davErr, _ := cancelled.Payload["dav_error"].(string)
		require.Contains(t, davErr, "500", "a falha do cancel fica registrada no evento")

		created := full.Items[12]
		require.Equal(t, "admin", string(created.Actor))
		require.Equal(t, float64(1), created.Payload["months"])
		createdFrom, err := time.Parse(time.RFC3339Nano, created.Payload["valid_from"].(string))
		require.NoError(t, err)
		createdUntil, err := time.Parse(time.RFC3339Nano, created.Payload["valid_until"].(string))
		require.NoError(t, err)
		requireInstant(t, enr.ValidFrom, createdFrom, "valid_from do matricula_criada")
		requireInstant(t, enr.ValidUntil, createdUntil, "valid_until do matricula_criada")

		// Pagina com limit=5 seguindo next_cursor até o fim: sem gaps, sem
		// repetidos, mesmo total.
		var collected []api.JourneyEvent
		cursor := ""
		pages := 0
		for {
			path := "/me/audit?limit=5"
			if cursor != "" {
				path += "&cursor=" + url.QueryEscape(cursor)
			}
			status, raw := doReq(t, patient, "GET", path, nil, nil)
			require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
			page := decodeAs[api.AuditPage](t, raw)
			collected = append(collected, page.Items...)
			pages++
			require.LessOrEqual(t, pages, 10, "a paginação precisa terminar")
			if page.NextCursor == nil {
				break
			}
			require.Len(t, page.Items, 5, "página cheia sempre traz next_cursor")
			cursor = *page.NextCursor
		}
		require.Equal(t, 3, pages, "13 eventos com limit=5 = páginas 5+5+3")
		require.Len(t, collected, len(full.Items))
		seen := map[string]bool{}
		for i, ev := range collected {
			require.Equal(t, full.Items[i].Id, ev.Id, "a paginação diverge da listagem em [%d]", i)
			require.False(t, seen[ev.Id.String()], "evento repetido: %s", ev.Id)
			seen[ev.Id.String()] = true
		}
	})

	step("A23_journey_event_append_only_sob_role_restrito", func(t *testing.T) {
		// O mesmo role da API (renovi_app): UPDATE/DELETE em journey_event são
		// recusados PELO BANCO (42501), não por disciplina de código.
		appPool, err := pgxpool.New(ctx, env.appDSN)
		require.NoError(t, err)
		defer appPool.Close()

		var pgErr *pgconn.PgError
		_, err = appPool.Exec(ctx, `UPDATE journey_event SET actor = 'paciente'`)
		require.Error(t, err)
		require.True(t, errors.As(err, &pgErr), "erro: %v", err)
		require.Equal(t, "42501", pgErr.Code)

		_, err = appPool.Exec(ctx, `DELETE FROM journey_event`)
		require.Error(t, err)
		require.True(t, errors.As(err, &pgErr), "erro: %v", err)
		require.Equal(t, "42501", pgErr.Code)
	})
}
