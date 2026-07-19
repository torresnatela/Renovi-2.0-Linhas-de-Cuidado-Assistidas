//go:build integration

package e2e

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/testsupport"
)

// TestE2E_C_SaudeMentalSemanal percorre os casos de uso (UC) do marco "linha
// semanal": psicologia sob QUOTA {max:1, period:week} — 1 por semana, em janela
// MÓVEL de 7 dias (vizinhos a EXATOS 7d são permitidos, a janela é semiaberta)
// — e psiquiatria sob QUOTA {max:1, period:month}, com matrícula de 1 mês (30d).
//
// Mapa dos UC pedidos no marco:
//
//	UC1 = C01–C03: linha publicada, paciente matriculado e ATIVO (jornada liberada)
//	UC2 = C04: as 4 de psicologia espaçadas em exatos 7d + 1 de psiquiatria, tudo 201
//	UC4 = C05: segunda psico na MESMA semana → 422 QUOTA (em qualquer horário da semana)
//	UC3 = C06: com tudo agendado, NADA mais é agendável — QUOTA dentro da vigência,
//	      VIGENCIA além dela; a disponibilidade anotada confirma slot a slot
//	UC5 = C07: cancela a psico de uma semana específica e reagenda NAQUELA semana
//	UC6 = C08: psiq desmarca e remarca livremente — para DEPOIS e para ANTES
//
// Extras além do marco: idempotência do reagendamento (C07), cota mensal da
// psiquiatria nas duas direções (C08), cancelamentos inválidos e rotas sem
// sessão (C09) e a auditoria contando a história inteira (C10).
//
// Horários (09:00/09:30 de hoje+D, America/Sao_Paulo), todos DISJUNTOS dos
// cenários A/B: psico +4/+11/+18/+25 (as quatro semanas da vigência), +12 e +26
// para os bloqueios, +32 para "além da vigência"; psiq +6, +13 (remarcar para
// antes) e +27 (remarcar para depois).
func TestE2E_C_SaudeMentalSemanal(t *testing.T) {
	seedSlots(t)
	step := steps(t)

	const (
		lineCode = "saude-mental-semanal"
		lineName = "Saúde Mental Semanal de teste"
	)

	patient := newPatientClient(t)

	var (
		accountID string
		items     = map[string]api.CareLineItem{}    // por ref: psico, psiq
		appts     = map[string]api.CareAppointment{} // por apelido: psico+4, ...
		keys      = map[string]string{}
	)

	book := func(t *testing.T, item api.CareLineItem, slotID, key string) (int, []byte) {
		t.Helper()
		return doReq(t, patient, "POST", "/me/appointments",
			map[string]string{"Idempotency-Key": key},
			map[string]any{"item_id": item.Id, "slot_id": slotID})
	}

	// requireQuotaBlock afirma o 422 canônico da cota: exatamente [QUOTA], com
	// o available_from EXATO — a aritmética da janela móvel é o que está em teste.
	requireQuotaBlock := func(t *testing.T, raw []byte, wantAvailableFrom time.Time) {
		t.Helper()
		p := problemOf(t, raw)
		require.Equal(t, "ELIGIBILITY_BLOCKED", reasonCode(p))
		blocks := blocksOf(t, p)
		require.Len(t, blocks, 1, "esperava exatamente [QUOTA]: %+v", blocks)
		require.Equal(t, "QUOTA", string(blocks[0].RuleType))
		require.NotNil(t, blocks[0].AvailableFrom)
		requireInstant(t, wantAvailableFrom, *blocks[0].AvailableFrom, "available_from da QUOTA")
	}

	step("C01_uc1_admin_publica_linha_semanal", func(t *testing.T) {
		status, raw := adminDo(t, "POST", "/admin/care-lines", map[string]any{
			"code": lineCode, "name": lineName, "description": "linha semanal do E2E (UC1–UC6)",
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		draft := decodeAs[api.CareLine](t, raw)
		base := "/admin/care-lines/" + draft.Id.String()

		for _, item := range []map[string]any{
			{"ref": "psico", "kind": "CONSULTA", "label": "Psicoterapia semanal", "specialty_code": "Psicologia"},
			{"ref": "psiq", "kind": "CONSULTA", "label": "Consulta psiquiátrica mensal", "specialty_code": "Psiquiatria"},
		} {
			status, raw := adminDo(t, "POST", base+"/items", item)
			require.Equal(t, http.StatusCreated, status, "item %v: %s", item["ref"], raw)
		}
		for _, rule := range []struct {
			ref  string
			body map[string]any
		}{
			{"psico", map[string]any{"rule_type": "QUOTA", "params": map[string]any{"max": 1, "period": "week"}}},
			{"psiq", map[string]any{"rule_type": "QUOTA", "params": map[string]any{"max": 1, "period": "month"}}},
		} {
			status, raw := adminDo(t, "POST", base+"/items/"+rule.ref+"/rules", rule.body)
			require.Equal(t, http.StatusCreated, status, "regra em %s: %s", rule.ref, raw)
		}

		status, raw = adminDo(t, "POST", base+"/publish", nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		line := decodeAs[api.CareLine](t, raw)
		require.Equal(t, "published", string(line.Status))
		require.Len(t, line.Items, 2)
		for _, it := range line.Items {
			items[it.Ref] = it
		}
	})

	step("C02_paciente3_registra_e_loga", func(t *testing.T) {
		status, raw := doReq(t, patient, "POST", "/auth/register", nil, map[string]any{
			"full_name":  "Paciente Três E2E",
			"cpf":        "111.444.777-35", // CPF válido pelo DV, distinto dos pacientes 1 e 2
			"birth_date": "1992-07-21",
			"email":      "paciente3.e2e@example.com",
			"phone":      "11999990003",
			"password":   "cavalo-bateria-grampo-3",
			"address": map[string]any{
				"zip_code": "01310-100", "street": "Av. Paulista", "number": "3000",
				"neighborhood": "Bela Vista", "city": "São Paulo", "state": "SP",
			},
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		accountID = decodeAs[api.Account](t, raw).Id.String()

		status, raw = doReq(t, patient, "POST", "/auth/login", nil, map[string]any{
			"cpf": "11144477735", "password": "cavalo-bateria-grampo-3",
		})
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
	})

	step("C03_uc1_matricula_ativa_e_jornada_liberada", func(t *testing.T) {
		status, raw := adminDo(t, "POST", "/admin/enrollments", map[string]any{
			"patient_id": accountID, "care_line_code": lineCode, "months": 1,
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		enr := decodeAs[api.Enrollment](t, raw)
		require.Equal(t, "ativa", string(enr.Status), "UC1: a matrícula nasce ATIVA")
		require.Equal(t, 30*24*time.Hour, enr.ValidUntil.Sub(enr.ValidFrom),
			"months=1 => vigência de exatamente 30 dias")

		status, raw = doReq(t, patient, "GET", "/me/journey", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		j := decodeAs[api.Journey](t, raw)
		require.Len(t, j.Enrollments, 1)
		je := j.Enrollments[0]
		require.Equal(t, lineName, je.CareLineName)
		require.Len(t, je.Items, 2)
		for _, it := range je.Items {
			require.Truef(t, it.Eligibility.Allowed, "UC1: item %s deveria nascer liberado: %+v",
				it.Item.Ref, it.Eligibility.Blocks)
		}
	})

	step("C04_uc2_agenda_4_psico_semanais_e_1_psiq", func(t *testing.T) {
		for _, b := range []struct {
			name string
			item api.CareLineItem
			slot testsupport.SeededSlot
		}{
			{"psico+4", items["psico"], anaSlotsC[4][0]},   // 09:00
			{"psico+11", items["psico"], anaSlotsC[11][0]}, // 09:00 (exatos 7d da +4 — permitido)
			{"psico+18", items["psico"], anaSlotsC[18][0]}, // 09:00 (exatos 7d da +11)
			{"psico+25", items["psico"], anaSlotsC[25][0]}, // 09:00 (exatos 7d da +18)
			{"psiq+6", items["psiq"], brunoSlotsC[6][0]},   // 09:00
		} {
			key := idemKey(t)
			keys[b.name] = key
			status, raw := book(t, b.item, b.slot.ID, key)
			require.Equal(t, http.StatusCreated, status, "UC2: agendar %s: %s", b.name, raw)
			appt := decodeAs[api.CareAppointment](t, raw)
			require.Equal(t, "agendada", string(appt.Status))
			requireInstant(t, b.slot.StartsAt, appt.ScheduledAt, b.name)
			appts[b.name] = appt
		}

		status, raw := doReq(t, patient, "GET", "/me/appointments", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		list := decodeAs[api.CareAppointmentList](t, raw)
		require.Len(t, list.Items, 5, "UC2: as 4 de psicologia + 1 de psiquiatria")
		for _, a := range list.Items {
			require.Equal(t, "agendada", string(a.Status))
		}
	})

	step("C05_uc4_segunda_psico_na_mesma_semana_422", func(t *testing.T) {
		// Nas DUAS formas de cair na mesma semana: outro dia (a +12, 1d depois da
		// +11) e o MESMO dia em outro horário (09:30 da +11). A janela móvel de 7d
		// bloqueia ambas; a vaga só abre quando a consulta mais antiga da janela
		// sai dela — +11 09:00 + 7d = +18 09:00, nos dois casos.
		wantAF := anaSlotsC[11][0].StartsAt.Add(7 * 24 * time.Hour)
		for _, attempt := range []struct {
			name string
			slot testsupport.SeededSlot
		}{
			{"psico+12 09:00 (dia seguinte ao da +11)", anaSlotsC[12][0]},
			{"psico+11 09:30 (mesmo dia da +11)", anaSlotsC[11][1]},
		} {
			status, raw := book(t, items["psico"], attempt.slot.ID, idemKey(t))
			require.Equal(t, http.StatusUnprocessableEntity, status, "UC4 %s: %s", attempt.name, raw)
			requireQuotaBlock(t, raw, wantAF)
		}
	})

	step("C06_uc3_com_tudo_agendado_nada_mais_e_agendavel", func(t *testing.T) {
		// (a) Psico DENTRO da vigência, semana ocupada (+26, 1d depois da +25):
		// QUOTA, liberando quando a +25 sai da janela (+25 09:00 + 7d).
		status, raw := book(t, items["psico"], anaSlotsC[26][0].ID, idemKey(t))
		require.Equal(t, http.StatusUnprocessableEntity, status, "corpo: %s", raw)
		requireQuotaBlock(t, raw, anaSlotsC[25][0].StartsAt.Add(7*24*time.Hour))

		// (b) Psico na semana LIVRE seguinte (+32, exatos 7d da +25 — a QUOTA
		// permite): é a VIGENCIA que barra, e só ela. Renovar é ação do admin,
		// não espera — sem available_from.
		status, raw = book(t, items["psico"], anaSlotsC[32][0].ID, idemKey(t))
		require.Equal(t, http.StatusUnprocessableEntity, status, "corpo: %s", raw)
		blocks := blocksOf(t, problemOf(t, raw))
		require.Len(t, blocks, 1, "esperava exatamente [VIGENCIA]: %+v", blocks)
		require.Equal(t, "VIGENCIA", string(blocks[0].RuleType))
		require.Contains(t, blocks[0].Reason, "Renove")
		require.Nil(t, blocks[0].AvailableFrom)

		// (c) Psiq em qualquer outro dia do mês (+27): QUOTA 1/mês, liberando
		// quando a +6 sai da janela de 30d.
		status, raw = book(t, items["psiq"], brunoSlotsC[27][0].ID, idemKey(t))
		require.Equal(t, http.StatusUnprocessableEntity, status, "corpo: %s", raw)
		requireQuotaBlock(t, raw, brunoSlotsC[6][0].StartsAt.Add(30*24*time.Hour))

		// (d) A jornada reflete: os DOIS itens bloqueados para agora, por QUOTA.
		status, raw = doReq(t, patient, "GET", "/me/journey", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		j := decodeAs[api.Journey](t, raw)
		require.Len(t, j.Enrollments, 1)
		for _, it := range j.Enrollments[0].Items {
			require.Falsef(t, it.Eligibility.Allowed, "UC3: item %s não deveria ter vaga agora", it.Item.Ref)
			requireBlock(t, it.Eligibility.Blocks, "QUOTA")
		}

		// (e) O simulador de elegibilidade concorda com o (b): além da vigência,
		// bloqueado por VIGENCIA.
		q := url.Values{}
		q.Set("item_id", items["psico"].Id.String())
		q.Set("date", anaSlotsC[32][0].StartsAt.Format(time.RFC3339))
		status, raw = doReq(t, patient, "GET", "/me/eligibility?"+q.Encode(), nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		elig := decodeAs[api.Eligibility](t, raw)
		require.False(t, elig.Allowed)
		require.Len(t, elig.Blocks, 1)
		require.Equal(t, "VIGENCIA", string(elig.Blocks[0].RuleType))

		// (f) A prova exaustiva do UC3: TODO slot livre anotado na disponibilidade
		// (de hoje até +32, para os dois itens) vem allowed=false — não há UM
		// horário agendável no sistema para este paciente.
		until := anaSlotsC[32][0].StartsAt.Format("2006-01-02")
		for _, item := range []string{"psico", "psiq"} {
			status, raw := doReq(t, patient, "GET",
				"/me/availability?item_id="+items[item].Id.String()+"&to="+until, nil, nil)
			require.Equal(t, http.StatusOK, status, "disponibilidade de %s: %s", item, raw)
			page := decodeAs[api.AvailabilityPage](t, raw)
			require.NotEmpty(t, page.Items, "a agenda tem horários livres — só não para este paciente")
			for _, s := range page.Items {
				require.Falsef(t, s.Eligibility.Allowed,
					"UC3: slot %s (%s) de %s não deveria estar agendável", s.Id, s.StartsAt, item)
			}
		}
	})

	step("C07_uc5_cancela_uma_semana_e_reagenda_nela", func(t *testing.T) {
		// Cancela a psico da semana da +11, com ~11 dias de antecedência (>24h:
		// a vaga volta para a cota E o horário volta para o legado).
		status, raw := doReq(t, patient, "POST",
			"/me/appointments/"+appts["psico+11"].Id.String()+"/cancel", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		cancelled := decodeAs[api.CareAppointment](t, raw)
		require.Equal(t, "cancelada", string(cancelled.Status))
		require.NotNil(t, cancelled.CancelledAt)

		// A semana reabriu, mas a janela é MÓVEL, não de calendário: só o 09:00
		// da +11 fica a >= 7d dos dois vizinhos (+4 09:00 e +18 09:00). A
		// disponibilidade dos dias +11/+12 anota exatamente isso, slot a slot.
		from := anaSlotsC[11][0].StartsAt.Format("2006-01-02")
		to := anaSlotsC[12][0].StartsAt.Format("2006-01-02")
		status, raw = doReq(t, patient, "GET",
			"/me/availability?item_id="+items["psico"].Id.String()+"&from="+from+"&to="+to, nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		page := decodeAs[api.AvailabilityPage](t, raw)
		var sawReleased bool
		for _, s := range page.Items {
			if s.Id == anaSlotsC[11][0].ID {
				sawReleased = true
				require.True(t, s.Eligibility.Allowed,
					"UC5: o horário cancelado deveria estar agendável de novo: %+v", s.Eligibility.Blocks)
				continue
			}
			require.Falsef(t, s.Eligibility.Allowed,
				"slot %s (%s): a <7d de um vizinho, deveria seguir bloqueado", s.Id, s.StartsAt)
		}
		require.True(t, sawReleased,
			"o slot da +11 09:00 não voltou à disponibilidade — o cancel não devolveu o horário ao legado?")

		// Reagendar às 09:30 do mesmo dia: 422 — fica a 6d23h30m da +18. A vaga
		// dessa janela abre quando a +18 sai dela (+18 09:00 + 7d).
		status, raw = book(t, items["psico"], anaSlotsC[11][1].ID, idemKey(t))
		require.Equal(t, http.StatusUnprocessableEntity, status, "corpo: %s", raw)
		requireQuotaBlock(t, raw, anaSlotsC[18][0].StartsAt.Add(7*24*time.Hour))

		// Reagendar NO horário liberado: 201 — a cancelada com antecedência não
		// conta na cota e o slot foi devolvido ao legado pelo cancelamento.
		key := idemKey(t)
		keys["psico+11b"] = key
		status, raw = book(t, items["psico"], anaSlotsC[11][0].ID, key)
		require.Equal(t, http.StatusCreated, status, "UC5: reagendar na semana liberada: %s", raw)
		rebooked := decodeAs[api.CareAppointment](t, raw)
		require.Equal(t, "agendada", string(rebooked.Status))
		requireInstant(t, anaSlotsC[11][0].StartsAt, rebooked.ScheduledAt, "psico+11 reagendada")
		appts["psico+11b"] = rebooked

		// Replay da mesma key: a MESMA consulta, 200.
		status, raw = book(t, items["psico"], anaSlotsC[11][0].ID, key)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		require.Equal(t, rebooked.Id, decodeAs[api.CareAppointment](t, raw).Id)
	})

	step("C08_uc6_psiquiatria_desmarca_e_remarca_livre", func(t *testing.T) {
		// Desmarca a psiq da +6 e remarca para DEPOIS (+27): com a cancelada fora
		// da cota, o mês está livre.
		status, raw := doReq(t, patient, "POST",
			"/me/appointments/"+appts["psiq+6"].Id.String()+"/cancel", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		require.Equal(t, "cancelada", string(decodeAs[api.CareAppointment](t, raw).Status))

		status, raw = book(t, items["psiq"], brunoSlotsC[27][0].ID, idemKey(t))
		require.Equal(t, http.StatusCreated, status, "UC6: remarcar para depois: %s", raw)
		later := decodeAs[api.CareAppointment](t, raw)
		requireInstant(t, brunoSlotsC[27][0].StartsAt, later.ScheduledAt, "psiq+27")
		appts["psiq+27"] = later

		// Desmarca de novo e remarca para ANTES (+13).
		status, raw = doReq(t, patient, "POST",
			"/me/appointments/"+later.Id.String()+"/cancel", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)

		status, raw = book(t, items["psiq"], brunoSlotsC[13][0].ID, idemKey(t))
		require.Equal(t, http.StatusCreated, status, "UC6: remarcar para antes: %s", raw)
		earlier := decodeAs[api.CareAppointment](t, raw)
		requireInstant(t, brunoSlotsC[13][0].StartsAt, earlier.ScheduledAt, "psiq+13")
		appts["psiq+13"] = earlier

		// A cota mensal segue valendo em VOLTA da remarcada: o slot da +6 voltou
		// ao legado, mas está a 7d da +13 — janela de 30d cheia. Libera quando a
		// +13 sair dela (+13 09:00 + 30d).
		status, raw = book(t, items["psiq"], brunoSlotsC[6][0].ID, idemKey(t))
		require.Equal(t, http.StatusUnprocessableEntity, status, "corpo: %s", raw)
		requireQuotaBlock(t, raw, brunoSlotsC[13][0].StartsAt.Add(30*24*time.Hour))
	})

	step("C09_extras_cancelamentos_invalidos_e_sessao", func(t *testing.T) {
		// Cancelar consulta JÁ cancelada → 409.
		status, raw := doReq(t, patient, "POST",
			"/me/appointments/"+appts["psiq+27"].Id.String()+"/cancel", nil, nil)
		require.Equal(t, http.StatusConflict, status, "corpo: %s", raw)
		require.Equal(t, "CANCEL_NOT_ALLOWED", reasonCode(problemOf(t, raw)))

		// Cancelar id inexistente → 404.
		status, raw = doReq(t, patient, "POST",
			"/me/appointments/"+uuid.NewString()+"/cancel", nil, nil)
		require.Equal(t, http.StatusNotFound, status, "corpo: %s", raw)

		// Rotas /me sem sessão → 401.
		status, raw = doReq(t, plainClient, "GET", "/me/journey", nil, nil)
		require.Equal(t, http.StatusUnauthorized, status, "corpo: %s", raw)
		status, raw = doReq(t, plainClient, "POST", "/me/appointments",
			map[string]string{"Idempotency-Key": idemKey(t)},
			map[string]any{"item_id": items["psico"].Id, "slot_id": anaSlotsC[26][0].ID})
		require.Equal(t, http.StatusUnauthorized, status, "corpo: %s", raw)
	})

	step("C10_estado_final_e_auditoria", func(t *testing.T) {
		// Estado final: 5 ativas (psico +4/+11/+18/+25, psiq +13) e 3 canceladas
		// (psico +11 original, psiq +6, psiq +27) — comparado por INSTANTE.
		status, raw := doReq(t, patient, "GET", "/me/appointments?status=agendada", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		active := decodeAs[api.CareAppointmentList](t, raw)
		require.Len(t, active.Items, 5)
		wantActive := map[string]bool{
			anaSlotsC[4][0].StartsAt.Format(time.RFC3339):    false,
			anaSlotsC[11][0].StartsAt.Format(time.RFC3339):   false,
			anaSlotsC[18][0].StartsAt.Format(time.RFC3339):   false,
			anaSlotsC[25][0].StartsAt.Format(time.RFC3339):   false,
			brunoSlotsC[13][0].StartsAt.Format(time.RFC3339): false,
		}
		for _, a := range active.Items {
			k := a.ScheduledAt.Format(time.RFC3339)
			seen, ok := wantActive[k]
			require.Truef(t, ok, "consulta ativa num instante inesperado: %s", k)
			require.Falsef(t, seen, "instante ativo repetido: %s", k)
			wantActive[k] = true
		}

		status, raw = doReq(t, patient, "GET", "/me/appointments?status=cancelada", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		require.Len(t, decodeAs[api.CareAppointmentList](t, raw).Items, 3)

		// A auditoria conta a história inteira, em ordem DESC.
		status, raw = doReq(t, patient, "GET", "/me/audit?limit=50", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		full := decodeAs[api.AuditPage](t, raw)
		want := []string{
			"consulta_agendada",  // C08 psiq +13 (remarcada para antes)
			"consulta_cancelada", // C08 psiq +27
			"consulta_agendada",  // C08 psiq +27 (remarcada para depois)
			"consulta_cancelada", // C08 psiq +6
			"consulta_agendada",  // C07 psico +11 (reagendada)
			"consulta_cancelada", // C07 psico +11
			"consulta_agendada",  // C04 psiq +6
			"consulta_agendada",  // C04 psico +25
			"consulta_agendada",  // C04 psico +18
			"consulta_agendada",  // C04 psico +11
			"consulta_agendada",  // C04 psico +4
			"matricula_criada",   // C03
		}
		require.Len(t, full.Items, len(want), "total de eventos da jornada do paciente 3")
		for i, ev := range full.Items {
			require.Equalf(t, want[i], string(ev.EventType), "evento [%d]", i)
		}

		// Todos os cancelamentos foram com antecedência: nenhum consome cota, e a
		// falha do cancel na DAV (o fake responde 500 sempre) fica registrada.
		for _, i := range []int{1, 3, 5} {
			ev := full.Items[i]
			require.Equal(t, "paciente", string(ev.Actor))
			require.Equal(t, false, ev.Payload["counts_for_quota"],
				"cancelamento com antecedência devolve a vaga (evento [%d])", i)
			hours, ok := ev.Payload["hours_before"].(float64)
			require.True(t, ok, "hours_before: %v", ev.Payload["hours_before"])
			require.Greater(t, hours, 24.0)
			require.Equal(t, false, ev.Payload["dav_cancelled"])
		}
	})
}
