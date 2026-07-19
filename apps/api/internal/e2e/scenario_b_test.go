//go:build integration

package e2e

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/http/api"
)

// TestE2E_B_ApoioPsicologico é o cenário B: a linha "Apoio Psicológico" com UM
// item de psicologia sob QUOTA {max:1, period:total} + MAX_ADVANCE 30d, para o
// paciente 2. A cota total não tem janela que "passe": o bloqueio é permanente
// (available_from AUSENTE) — só cancelamento com antecedência devolve a vaga.
//
// Os horários usados (Ana: +2 09:30, +16 09:00, +23 09:00) são disjuntos dos
// que o cenário A reservou, então os dois cenários dividem o mesmo legado sem
// se pisar.
func TestE2E_B_ApoioPsicologico(t *testing.T) {
	seedSlots(t)
	step := steps(t)

	const lineCode = "apoio-psicologico"

	patient := newPatientClient(t)

	var (
		accountID string
		item      api.CareLineItem
		first     api.CareAppointment // a consulta de +2 (cancelada no passo 4)
	)

	book := func(t *testing.T, slotID, key string) (int, []byte) {
		t.Helper()
		return doReq(t, patient, "POST", "/me/appointments",
			map[string]string{"Idempotency-Key": key},
			map[string]any{"item_id": item.Id, "slot_id": slotID})
	}

	step("B01_linha_paciente_e_matricula", func(t *testing.T) {
		status, raw := adminDo(t, "POST", "/admin/care-lines", map[string]any{
			"code": lineCode, "name": "Apoio Psicológico",
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		draft := decodeAs[api.CareLine](t, raw)
		base := "/admin/care-lines/" + draft.Id.String()

		status, raw = adminDo(t, "POST", base+"/items", map[string]any{
			"ref": "apoio", "kind": "CONSULTA", "label": "Sessão de apoio", "specialty_code": "Psicologia",
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		for _, rule := range []map[string]any{
			{"rule_type": "QUOTA", "params": map[string]any{"max": 1, "period": "total"}},
			{"rule_type": "MAX_ADVANCE", "params": map[string]any{"days": 30}},
		} {
			status, raw := adminDo(t, "POST", base+"/items/apoio/rules", rule)
			require.Equal(t, http.StatusCreated, status, "regra %v: %s", rule["rule_type"], raw)
		}
		status, raw = adminDo(t, "POST", base+"/publish", nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		published := decodeAs[api.CareLine](t, raw)
		require.Len(t, published.Items, 1)
		item = published.Items[0]

		// Paciente 2: CPF válido DISTINTO (o "zeros à esquerda" dos testes de cpf).
		status, raw = doReq(t, patient, "POST", "/auth/register", nil, map[string]any{
			"full_name":  "Paciente Dois E2E",
			"cpf":        "00000003700",
			"birth_date": "1988-11-05",
			"email":      "paciente2.e2e@example.com",
			"phone":      "11999990002",
			"password":   "cavalo-bateria-grampo-2",
			"address": map[string]any{
				"zip_code": "01310-100", "street": "Av. Paulista", "number": "2000",
				"neighborhood": "Bela Vista", "city": "São Paulo", "state": "SP",
			},
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		accountID = decodeAs[api.Account](t, raw).Id.String()

		status, raw = doReq(t, patient, "POST", "/auth/login", nil, map[string]any{
			"cpf": "00000003700", "password": "cavalo-bateria-grampo-2",
		})
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)

		status, raw = adminDo(t, "POST", "/admin/enrollments", map[string]any{
			"patient_id": accountID, "care_line_code": lineCode, "months": 1,
		})
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		require.Equal(t, "ativa", string(decodeAs[api.Enrollment](t, raw).Status))
	})

	step("B02_agenda_a_unica_liberada", func(t *testing.T) {
		status, raw := book(t, anaSlots[2][1].ID, idemKey(t)) // +2, 09:30
		require.Equal(t, http.StatusCreated, status, "corpo: %s", raw)
		first = decodeAs[api.CareAppointment](t, raw)
		require.Equal(t, "agendada", string(first.Status))
		require.Equal(t, "apoio", first.ItemRef)
		requireInstant(t, anaSlots[2][1].StartsAt, first.ScheduledAt, "apoio+2")
	})

	step("B03_quota_total_bloqueio_permanente", func(t *testing.T) {
		status, raw := book(t, anaSlots[16][0].ID, idemKey(t)) // +16, 09:00 (livre)
		require.Equal(t, http.StatusUnprocessableEntity, status, "corpo: %s", raw)
		p := problemOf(t, raw)
		require.Equal(t, "ELIGIBILITY_BLOCKED", reasonCode(p))
		blocks := blocksOf(t, p)
		require.Len(t, blocks, 1, "esperava exatamente [QUOTA]: %+v", blocks)
		require.Equal(t, "QUOTA", string(blocks[0].RuleType))
		require.Nil(t, blocks[0].AvailableFrom,
			"period=total: a janela é a vida da matrícula — não há data em que destrave")
	})

	step("B04_cancela_e_a_vaga_volta", func(t *testing.T) {
		status, raw := doReq(t, patient, "POST",
			"/me/appointments/"+first.Id.String()+"/cancel", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		require.Equal(t, "cancelada", string(decodeAs[api.CareAppointment](t, raw).Status))

		status, raw = book(t, anaSlots[16][0].ID, idemKey(t))
		require.Equal(t, http.StatusCreated, status,
			"cancelada com >24h de antecedência não conta na cota total: %s", raw)
		again := decodeAs[api.CareAppointment](t, raw)
		require.Equal(t, "agendada", string(again.Status))
		requireInstant(t, anaSlots[16][0].StartsAt, again.ScheduledAt, "apoio+16")
	})

	step("B05_quota_total_de_novo_permanente", func(t *testing.T) {
		status, raw := book(t, anaSlots[23][0].ID, idemKey(t)) // +23, 09:00 (livre)
		require.Equal(t, http.StatusUnprocessableEntity, status, "corpo: %s", raw)
		p := problemOf(t, raw)
		blocks := blocksOf(t, p)
		require.Len(t, blocks, 1, "esperava exatamente [QUOTA]: %+v", blocks)
		require.Equal(t, "QUOTA", string(blocks[0].RuleType))
		require.Nil(t, blocks[0].AvailableFrom)
	})

	// Garantia de isolamento entre cenários: a jornada do paciente 2 só tem a
	// matrícula dele, e as consultas dele não vazam para o paciente 1 (e
	// vice-versa) — as rotas /me são escopadas pela sessão.
	step("B06_jornadas_nao_vazam_entre_pacientes", func(t *testing.T) {
		status, raw := doReq(t, patient, "GET", "/me/journey", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		j := decodeAs[api.Journey](t, raw)
		require.Len(t, j.Enrollments, 1)
		require.Equal(t, lineCode, j.Enrollments[0].Enrollment.CareLineCode)

		status, raw = doReq(t, patient, "GET", "/me/appointments", nil, nil)
		require.Equal(t, http.StatusOK, status, "corpo: %s", raw)
		list := decodeAs[api.CareAppointmentList](t, raw)
		require.Len(t, list.Items, 2, "só as consultas do paciente 2 (a cancelada e a ativa)")
		for _, a := range list.Items {
			require.Equal(t, "apoio", a.ItemRef)
		}
	})
}
