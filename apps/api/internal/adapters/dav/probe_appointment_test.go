//go:build davprobe

// Sondagem da API de APPOINTMENT da DAV — a metade "agendamento" da bateria.
//
// Arquivo separado do probe_test.go (que sonda /person) porque são dois recursos
// com perguntas independentes; a infra (probeClient, record, do, cleanup) é
// compartilhada.
//
// Por que existe: o spec deles se contradiz de novo, e desta vez em cima do que
// decide a arquitetura.
//
//  1. `AppointmentCreateRequestSchema` NÃO tem a propriedade `id`, mas o exemplo
//     "requiredOnly" do próprio POST diz "Note que o campo ID segue a regra
//     descrita acima". O `POST /professional` tem `id` no schema. Se o appointment
//     aceitar um id NOSSO, o POST vira sondável e ganhamos a mesma idempotência
//     que o cadastro tem (ADR-011b). Se não aceitar, um 504 nos deixa sem saber se
//     a consulta existe E sem id para procurá-la.
//  2. `ParticipantRequestSchema` declara `url` como obrigatório no REQUEST — o que
//     é impossível: a url de atendimento é o que a DAV DEVOLVE. O exemplo deles
//     omite. Um dos dois mente.
//  3. Medido à mão antes desta bateria: `GET /appointment/{id}` devolveu
//     `500 {"message":"Unexpected end of JSON input"}` tanto para um id real do
//     Renovi legado quanto para um UUID inexistente. Se for 500-para-tudo, não há
//     sonda de reconciliação — e a saga de agendamento precisa saber disso ANTES
//     de ser escrita, não depois de um incidente.
//
// Higiene: cria um profissional e um paciente sintéticos (prefixo "RENOVI PROBE",
// CPF com DV válido, e-mail @example.com) e remove tudo no fim.
package dav

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// O que criamos na sondagem de agendamento, para limpar depois.
var (
	createdProfessionalIDs sync.Map
	createdAppointmentIDs  sync.Map
)

// saoPaulo é o fuso do negócio. O legado da Renovi grava `datetime` sem fuso, e
// é hora de parede daqui — por isso a sondagem #15 pergunta o que a DAV faz com
// o offset.
var saoPaulo = mustLoadSaoPaulo()

func mustLoadSaoPaulo() *time.Location {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		panic("carregar America/Sao_Paulo: " + err.Error())
	}
	return loc
}

// ---------------------------------------------------------------------------
// Elenco da sondagem: um médico (MMD) e um paciente (PAT)
// ---------------------------------------------------------------------------

// probeCast é o par mínimo de participantes: a DAV exige no MÍNIMO 2.
type probeCast struct {
	professionalID string // MMD
	patientID      string // PAT
}

// newProbeCast cria os dois participantes na DAV.
//
// Cria em vez de reaproveitar médico do MySQL da Renovi de propósito: a bateria
// não pode depender do estado de outro sistema (a HML do legado está com dados de
// 2025 e nenhum slot futuro). Sondagem que depende de dado alheio quebra sozinha.
func newProbeCast(t *testing.T, c *probeClient) probeCast {
	t.Helper()

	proID := uuid.NewString()
	pro := c.createProfessional(t, map[string]any{
		"id":              proID,
		"name":            probeNamePrefix + " Medico",
		"cpf":             randomCPF(),
		"birth_date":      "1980-03-11",
		"email":           probeEmail(),
		"license_number":  "99999",
		"license_region":  "SP",
		"license_council": "CRM",
	})
	if pro.outcome() != accepted {
		t.Fatalf("não consegui criar o profissional da sondagem (HTTP %d): sem um MMD "+
			"não há appointment para sondar.\nResposta: %s", pro.status, pro.pretty(600))
	}

	pat := c.createPerson(t, minimalPerson())
	if pat.outcome() != accepted {
		t.Fatalf("não consegui criar o paciente da sondagem (HTTP %d): sem um PAT "+
			"não há appointment para sondar.\nResposta: %s", pat.status, pat.pretty(600))
	}
	var patOut struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(pat.body, &patOut); err != nil || patOut.ID == "" {
		t.Fatalf("paciente criado mas sem id utilizável na resposta: %s", pat.pretty(400))
	}

	return probeCast{professionalID: proID, patientID: patOut.ID}
}

// createProfessional cria o médico. Registra o id ANTES do POST, pelo mesmo
// motivo do createPerson: um POST que estoura o gateway pode ter criado.
func (c *probeClient) createProfessional(t *testing.T, payload map[string]any) probeResponse {
	t.Helper()
	if id, ok := payload["id"].(string); ok && id != "" {
		createdProfessionalIDs.Store(id, struct{}{})
	}
	r := c.do(t, http.MethodPost, "/professional", payload)
	if r.outcome() == accepted {
		var out struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(r.body, &out) == nil && out.ID != "" {
			createdProfessionalIDs.Store(out.ID, struct{}{})
		}
	}
	return r
}

// createAppointment cria a consulta, registrando os ids para a limpeza.
//
// Registra o id enviado (se houver) E o devolvido: enquanto não soubermos se a
// DAV honra o id do integrador (sondagem #12), os dois podem existir.
func (c *probeClient) createAppointment(t *testing.T, payload map[string]any) probeResponse {
	t.Helper()
	if id, ok := payload["id"].(string); ok && id != "" {
		createdAppointmentIDs.Store(id, struct{}{})
	}
	r := c.do(t, http.MethodPost, "/appointment", payload)
	if r.outcome() == accepted {
		var out struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(r.body, &out) == nil && out.ID != "" {
			createdAppointmentIDs.Store(out.ID, struct{}{})
		}
	}
	return r
}

// appointmentPayload monta o corpo mínimo aceito pelo spec.
//
// NOTA: `participants[].url` é deliberadamente OMITIDO, embora o
// ParticipantRequestSchema o declare obrigatório. É a sondagem #13: o exemplo
// deles também omite, e a url é o que eles devolvem — não teríamos o que mandar.
func appointmentPayload(cast probeCast, start, end time.Time) map[string]any {
	return map[string]any{
		"title":              probeNamePrefix + " Consulta",
		"start_date_time":    start.Format(time.RFC3339),
		"end_date_time":      end.Format(time.RFC3339),
		"appointment_reason": "elective", // único valor do enum
		"participants": []map[string]any{
			{"id": cast.professionalID, "role": "MMD"},
			{"id": cast.patientID, "role": "PAT"},
		},
	}
}

// probeSlot devolve uma janela futura de 25 minutos (a duração mais comum dos
// slots reais da Renovi), deslocada para não colidir entre sondagens.
func probeSlot(offset time.Duration) (time.Time, time.Time) {
	start := time.Now().In(saoPaulo).Add(24*time.Hour + offset).Truncate(time.Minute)
	return start, start.Add(25 * time.Minute)
}

// ---------------------------------------------------------------------------
// A bateria de agendamento (achados 12+)
// ---------------------------------------------------------------------------

// 12. A DAV aceita um `id` NOSSO no POST /appointment?
//
// É a pergunta que decide a arquitetura da saga. Com id nosso, um 504 é sondável
// (GET /appointment/{nosso-id}) e reaproveitamos o padrão do ADR-011b. Sem ele,
// um timeout deixa a consulta possivelmente criada e inalcançável — e o slot
// travado sem ninguém para reconciliar.
func probeAppointmentIntegratorID(t *testing.T, c *probeClient, cast probeCast) {
	const q = "A DAV aceita um `id` nosso no POST /appointment?"

	ours := uuid.NewString()
	start, end := probeSlot(0)
	payload := appointmentPayload(cast, start, end)
	payload["id"] = ours

	r := c.createAppointment(t, payload)
	if r.outcome() == inconclusive {
		record(12, q, r.inconclusiveNote(), codeBlock("json", r.pretty(600)))
		return
	}
	if r.outcome() == rejected {
		record(12, q, fmt.Sprintf("HTTP %d — a DAV RECUSOU o id do integrador. "+
			"**Sem idempotência**: um 504 no POST deixa a consulta órfã e sem id para sondar. "+
			"Ver Risco 1 do plano.", r.status),
			codeBlock("json", jsonOf(payload))+"\n\n"+codeBlock("json", r.pretty(600)))
		return
	}

	var out struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(r.body, &out)

	// Um 201 sozinho NÃO prova nada, e é aqui que mora a armadilha. O validador
	// deles é class-validator (NestJS): com `whitelist: true` sem
	// `forbidNonWhitelisted`, um `id` desconhecido é DESCARTADO EM SILÊNCIO e a
	// DAV gera o dela. Se lêssemos só o 201 e assumíssemos que o id é nosso,
	// a reconciliação sondaria `GET /appointment/{nosso-id}`, não acharia,
	// concluiria "não criou" e criaria DE NOVO — duas consultas reais para o
	// mesmo paciente. Só o GET fecha a pergunta.
	confirm := c.do(t, http.MethodGet, "/appointment/"+ours, nil)

	verdict := fmt.Sprintf("HTTP %d — a DAV devolveu o id `%s`. ", r.status, out.ID)
	switch {
	case out.ID == ours && confirm.outcome() == accepted:
		verdict += "✅ **Aceita e honra o id do integrador**, e o `GET /appointment/{nosso-id}` " +
			"confirma. Igual ao `/person` (achado #4): o POST fica sondável, a escrita nunca " +
			"repete e a reconciliação do ADR-011b se aplica inteira ao agendamento."
	case out.ID == ours:
		verdict += fmt.Sprintf("⚠️ **Ecoou o nosso id, mas o `GET /appointment/{nosso-id}` respondeu %d** "+
			"(ver achado #15). Ecoar não é guardar: enquanto o GET não confirmar, NÃO dá para "+
			"tratar o id como sondável.", confirm.status)
	default:
		verdict += "🔴 **Ela IGNOROU o nosso id e gerou outro.** Gravar sempre o id DEVOLVIDO, " +
			"nunca o enviado. E a consequência é dura: **não existe reconciliação** — um POST que " +
			"estoura os 29s do gateway pode ter criado uma consulta cujo id nunca saberemos, e a " +
			"DAV **não tem rota de busca/listagem de appointment**. Órfã para sempre. " +
			"É o Risco 1 do plano se confirmando: a saga tem que falhar FECHADA (segurar o slot)."
	}
	record(12, q, verdict,
		"Enviado:\n\n"+codeBlock("json", jsonOf(payload))+
			"\n\nResposta do POST:\n\n"+codeBlock("json", r.pretty(600))+
			"\n\n`GET /appointment/{nosso-id}` (a prova):\n\n"+codeBlock("json", confirm.pretty(400)))
}

// 13. `participants[].url` é mesmo obrigatório no REQUEST?
//
// O ParticipantRequestSchema diz `required: [id, role, url]`. Isso não pode estar
// certo: a url de atendimento é o que a DAV gera. O exemplo "requiredOnly" deles
// manda só {id, role}. Todo payload desta bateria omite `url` — se algum for
// aceito, o spec mente (de novo).
func probeParticipantURLRequired(t *testing.T, c *probeClient, cast probeCast) {
	const q = "`participants[].url` é obrigatório no request, como diz o spec?"

	start, end := probeSlot(1 * time.Hour)
	payload := appointmentPayload(cast, start, end) // sem `url`

	r := c.createAppointment(t, payload)
	switch r.outcome() {
	case inconclusive:
		record(13, q, r.inconclusiveNote(), codeBlock("json", r.pretty(600)))
	case rejected:
		record(13, q, fmt.Sprintf("HTTP %d — a DAV recusou o participante sem `url`. "+
			"O spec estaria certo, o que é estranho: teríamos que inventar a url que eles geram. "+
			"Ler a mensagem antes de concluir.", r.status),
			codeBlock("json", jsonOf(payload))+"\n\n"+codeBlock("json", r.pretty(600)))
	default:
		record(13, q, fmt.Sprintf("HTTP %d — **não é obrigatório; o spec mente.** "+
			"Mandar `{id, role}` e ler a `url` da RESPOSTA.", r.status),
			codeBlock("json", jsonOf(payload))+"\n\n"+codeBlock("json", r.pretty(800)))
	}
}

// 14. Onde vem a url do paciente, e dá para saber de quem é cada uma?
//
// O ParticipantResponseSchema é {id, url} — SEM `role`. Então a única forma de
// achar a url do paciente é casar pelo id que enviamos como PAT. Se a resposta
// vier sem os ids, ou com url só de um participante, a feature inteira (o link
// que o paciente clica) não tem de onde sair.
func probeAttendanceURL(t *testing.T, c *probeClient, cast probeCast) {
	const q = "De onde sai a url de atendimento do paciente?"

	start, end := probeSlot(2 * time.Hour)
	r := c.createAppointment(t, appointmentPayload(cast, start, end))
	if r.outcome() != accepted {
		record(14, q, fmt.Sprintf("Inconclusivo — o POST respondeu %d.", r.status),
			codeBlock("json", r.pretty(600)))
		return
	}

	var out struct {
		ID           string `json:"id"`
		Participants []struct {
			ID   string `json:"id"`
			Role string `json:"role"`
			URL  string `json:"url"`
		} `json:"participants"`
	}
	_ = json.Unmarshal(r.body, &out)

	var patURL, mmdURL string
	temRole := false
	for _, p := range out.Participants {
		if p.Role != "" {
			temRole = true
		}
		switch p.ID {
		case cast.patientID:
			patURL = p.URL
		case cast.professionalID:
			mmdURL = p.URL
		}
	}

	verdict := fmt.Sprintf("HTTP %d — a resposta traz %d participante(s). ", r.status, len(out.Participants))
	switch {
	case patURL != "" && mmdURL != "" && patURL != mmdURL:
		verdict += "**Cada participante tem a SUA url.** A do PAT é o link que o paciente clica."
	case patURL != "" && patURL == mmdURL:
		verdict += "⚠️ **A url é a MESMA para médico e paciente** — é link de sala, não por pessoa."
	case patURL == "":
		verdict += "⚠️ **Não achei url para o id do PAT.** Sem isso não há link para o paciente e a " +
			"feature inteira não existe. Conferir se os ids voltam como enviados."
	}
	// O ParticipantResponseSchema deles declara só {id, url}. Se vier `role`, é
	// mais uma contradição do spec — e muda como o adapter acha a url do paciente.
	if temRole {
		verdict += " O spec declara a resposta como `{id, url}`, mas ela **também traz `role`**. " +
			"Mesmo assim casamos pelo ID que enviamos, e não pelo role: o id é o que NÓS " +
			"controlamos, e depender de um campo que o spec diz não existir é construir sobre " +
			"algo que eles podem remover sem avisar."
	} else {
		verdict += " A resposta não traz `role` (como o spec diz), então a única forma de achar a " +
			"url do paciente é casar pelo id que enviamos."
	}
	record(14, q, verdict, codeBlock("json", r.pretty(1200)))
}

// 15. `GET /appointment/{id}` serve como sonda de reconciliação?
//
// A pergunta mais importante da bateria. O ADR-011b inteiro depende de conseguir
// perguntar "a escrita pegou?" depois de um 504. Medição manual anterior: 500
// para id real do legado E para UUID inexistente — mesmo corpo, "Unexpected end
// of JSON input". Se aqui, com um appointment que ACABAMOS de criar, o GET também
// falhar, então não existe reconciliação e o Risco 1 do plano se confirma.
func probeAppointmentGet(t *testing.T, c *probeClient, cast probeCast) {
	const q = "`GET /appointment/{id}` serve de sonda de reconciliação?"

	start, end := probeSlot(3 * time.Hour)
	created := c.createAppointment(t, appointmentPayload(cast, start, end))
	if created.outcome() != accepted {
		record(15, q, fmt.Sprintf("Inconclusivo — não consegui criar o appointment para sondar (HTTP %d).", created.status),
			codeBlock("json", created.pretty(600)))
		return
	}
	var out struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(created.body, &out)

	existente := c.do(t, http.MethodGet, "/appointment/"+out.ID, nil)
	inexistente := c.do(t, http.MethodGet, "/appointment/"+uuid.NewString(), nil)

	var verdict string
	switch {
	case existente.outcome() == accepted && inexistente.status == http.StatusNoContent:
		verdict = "✅ **Serve.** 200 para o que existe, 204 para o que não existe — mesma " +
			"semântica do `/person` (achado #1). A reconciliação do ADR-011b funciona: " +
			"depois de um 504, sondar antes de concluir."
	case existente.outcome() == accepted:
		verdict = fmt.Sprintf("⚠️ **Serve pela metade.** 200 para o que existe, mas o inexistente "+
			"devolveu %d (esperado 204). Dá para reconciliar (200 = existe), mas o negativo é "+
			"ambíguo: não distinguir 'não existe' de 'a DAV caiu' obriga a errar para o lado seguro "+
			"(deixar PENDING_DAV, não liberar o slot).", inexistente.status)
	default:
		verdict = fmt.Sprintf("🔴 **NÃO serve.** Um appointment que acabamos de criar devolveu "+
			"HTTP %d no GET (o inexistente: %d). **Não há sonda de reconciliação.** "+
			"Depois de um 504 no POST ficamos sem saber se a consulta existe. "+
			"Abrir chamado na DAV com o `trace` da evidência antes de confiar na saga.",
			existente.status, inexistente.status)
	}

	record(15, q, verdict,
		"Appointment recém-criado (`"+out.ID+"`):\n\n"+codeBlock("json", existente.pretty(900))+
			"\n\nUUID inexistente (controle):\n\n"+codeBlock("json", inexistente.pretty(400)))
}

// 16. A DAV respeita o offset -03:00 que mandamos?
//
// O legado grava `datetime` sem fuso (hora de parede de São Paulo) e o exemplo do
// spec deles usa `Z`. Se a DAV reinterpretar o offset, a consulta cai 3 horas
// fora e o paciente perde a hora. É o bug mais provável desta feature.
func probeTimezone(t *testing.T, c *probeClient, cast probeCast) {
	const q = "A DAV respeita o offset `-03:00` ou reinterpreta como UTC?"

	start, end := probeSlot(5 * time.Hour)
	payload := appointmentPayload(cast, start, end) // RFC3339 em America/Sao_Paulo => -03:00

	r := c.createAppointment(t, payload)
	if r.outcome() != accepted {
		record(16, q, fmt.Sprintf("Inconclusivo — POST respondeu %d.", r.status), codeBlock("json", r.pretty(600)))
		return
	}
	var out struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(r.body, &out)

	got := c.do(t, http.MethodGet, "/appointment/"+out.ID, nil)
	if got.outcome() != accepted {
		record(16, q, fmt.Sprintf("**Inconclusivo** — o GET respondeu %d (ver achado #15), "+
			"então não dá para conferir o que foi gravado. Enquanto isso, tratar o fuso como "+
			"risco aberto: enviar sempre com offset explícito e conferir na tela.", got.status),
			"Enviado:\n\n"+codeBlock("json", jsonOf(payload))+"\n\nGET:\n\n"+codeBlock("json", got.pretty(400)))
		return
	}

	var back struct {
		Start string `json:"start_date_time"`
	}
	_ = json.Unmarshal(got.body, &back)

	verdict := fmt.Sprintf("Enviamos `%s`; a DAV devolveu `%s`. ", start.Format(time.RFC3339), back.Start)
	if parsed, err := time.Parse(time.RFC3339, back.Start); err == nil && parsed.Equal(start) {
		verdict += "✅ **Mesmo instante** — o offset é respeitado. Mandar sempre RFC3339 com offset."
	} else {
		verdict += "🔴 **Instante DIFERENTE** — a DAV reinterpretou o horário. " +
			"Converter para UTC antes de enviar e conferir de novo."
	}
	record(16, q, verdict, codeBlock("json", got.pretty(700)))
}

// 17. A DAV impede dois appointments no mesmo horário para o MESMO médico?
//
// Importa porque a trava de double-booking do lado da Renovi é fraca: o MySQL
// real não tem constraint nenhuma (só o flag `booked`), então nosso lock é um
// SELECT..FOR UPDATE. Se a DAV também recusar, ganhamos uma segunda rede de
// proteção de graça. Se aceitar, o SELECT..FOR UPDATE é a ÚNICA que existe.
func probeDoubleBooking(t *testing.T, c *probeClient, cast probeCast) {
	const q = "A DAV recusa dois appointments no mesmo horário para o mesmo médico?"

	start, end := probeSlot(7 * time.Hour)
	first := c.createAppointment(t, appointmentPayload(cast, start, end))
	if first.outcome() != accepted {
		record(17, q, fmt.Sprintf("Inconclusivo — o primeiro POST respondeu %d.", first.status),
			codeBlock("json", first.pretty(600)))
		return
	}
	second := c.createAppointment(t, appointmentPayload(cast, start, end))

	var verdict string
	switch second.outcome() {
	case rejected:
		verdict = fmt.Sprintf("HTTP %d — **a DAV recusa.** Segunda rede contra double-booking, "+
			"além do nosso SELECT..FOR UPDATE no slot.", second.status)
	case accepted:
		verdict = fmt.Sprintf("HTTP %d — ⚠️ **a DAV ACEITA sobreposição.** Não há segunda rede: o CAS "+
			"do adapter da agenda (`UPDATE tb_slots SET booked=1 WHERE id=? AND booked=0`) é a ÚNICA "+
			"trava de double-booking do sistema — o MySQL real não tem constraint alguma ligando "+
			"consulta a horário. O teste de concorrência do adapter não é opcional.", second.status)
	default:
		verdict = second.inconclusiveNote()
	}
	record(17, q, verdict, "Segundo POST no mesmo horário/MMD:\n\n"+codeBlock("json", second.pretty(600)))
}

// 18. A DAV recusa horário no passado, como o spec promete?
//
// Barato de sondar e define se precisamos validar antes de gastar um round-trip
// lento — e se dá para confiar nisso como rede contra slot velho no legado.
func probePastAppointment(t *testing.T, c *probeClient, cast probeCast) {
	const q = "A DAV recusa `start_date_time` no passado?"

	start := time.Now().In(saoPaulo).Add(-48 * time.Hour).Truncate(time.Minute)
	r := c.createAppointment(t, appointmentPayload(cast, start, start.Add(25*time.Minute)))

	var verdict string
	switch r.outcome() {
	case rejected:
		verdict = fmt.Sprintf("HTTP %d — **recusa, como o spec diz.** Ainda assim validamos antes: "+
			"falha rápido, sem gastar um POST de ~2s.", r.status)
	case accepted:
		verdict = fmt.Sprintf("HTTP %d — ⚠️ **ACEITA passado**, ao contrário do que o spec afirma. "+
			"A validação é nossa: filtrar slot com `startsAt > agora` na query do legado.", r.status)
	default:
		verdict = r.inconclusiveNote()
	}
	record(18, q, verdict, codeBlock("json", r.pretty(600)))
}

// 19. Qual a latência real do POST /appointment?
//
// O cadastro já ensinou que a DAV é lenta e que o teto de 29s é do AWS API
// Gateway. O agendamento herda o problema: é uma chamada síncrona dentro de um
// clique. Precisamos do número para dimensionar o timeout da rota.
func probeAppointmentLatency(t *testing.T, c *probeClient, cast probeCast) {
	const q = "Qual a latência do POST /appointment?"

	var samples []time.Duration
	for i := 0; i < 3; i++ {
		start, end := probeSlot(time.Duration(10+i) * time.Hour)
		r := c.createAppointment(t, appointmentPayload(cast, start, end))
		if r.outcome() == accepted {
			samples = append(samples, r.elapsed)
		}
	}
	if len(samples) == 0 {
		record(19, q, "Inconclusivo — nenhum POST foi aceito.", "(sem amostras)")
		return
	}

	var total, max time.Duration
	for _, s := range samples {
		total += s
		if s > max {
			max = s
		}
	}
	avg := total / time.Duration(len(samples))

	record(19, q, fmt.Sprintf("Média **%s**, máx **%s** (n=%d) NESTA execução. ⚠️ **Varia muito entre "+
		"execuções**: sondagens do mesmo dia deram média de 3,3s e de 10,5s, com máximo de 17,2s — "+
		"ou seja, o número de uma rodada só não dimensiona nada. Tratar como "+
		"\"alguns segundos, às vezes mais de 15\". O teto do gateway (29s) continua valendo: "+
		"`RENOVI_DAV_TIMEOUT` acima disso é inútil, e com essa variância o 504 não é hipótese "+
		"remota. A rota de POST /appointments precisa do mesmo tratamento do register: timeout "+
		"próprio derivado do orçamento da DAV + SetWriteDeadline.",
		avg.Round(time.Millisecond), max.Round(time.Millisecond), len(samples)),
		codeBlock("", formatSamples(samples)))
}

// 20. O cancelamento funciona e vira status CAN?
//
// Precisamos para o compensating action: se a DAV criou a consulta mas o slot não
// pôde ser confirmado, é o cancel que evita deixar consulta fantasma na agenda do
// médico.
func probeCancel(t *testing.T, c *probeClient, cast probeCast) {
	const q = "`PUT /appointment/{id}/cancel` funciona?"

	start, end := probeSlot(20 * time.Hour)
	created := c.createAppointment(t, appointmentPayload(cast, start, end))
	if created.outcome() != accepted {
		record(20, q, fmt.Sprintf("Inconclusivo — o POST respondeu %d.", created.status),
			codeBlock("json", created.pretty(400)))
		return
	}
	var out struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(created.body, &out)

	r := c.do(t, http.MethodPut, "/appointment/"+out.ID+"/cancel", map[string]any{})

	evidence := "Cancel:\n\n" + codeBlock("json", r.pretty(600))
	verdict := fmt.Sprintf("HTTP %d.", r.status)
	if r.outcome() == accepted {
		if got := c.do(t, http.MethodGet, "/appointment/"+out.ID, nil); got.outcome() == accepted {
			var back struct {
				Status string `json:"status_appointment"`
			}
			_ = json.Unmarshal(got.body, &back)
			verdict = fmt.Sprintf("HTTP %d — **funciona**; o status virou `%s` (esperado `CAN`). "+
				"É o compensating action do agendamento.", r.status, back.Status)
			evidence += "\n\nGET depois do cancel:\n\n" + codeBlock("json", got.pretty(500))
		} else {
			verdict = fmt.Sprintf("HTTP %d — o cancel foi aceito, mas o GET (%d) não deixa conferir "+
				"o status resultante (ver achado #15).", r.status, got.status)
		}
	}
	record(20, q, verdict, evidence)
}

func formatSamples(ss []time.Duration) string {
	var b strings.Builder
	for i, s := range ss {
		fmt.Fprintf(&b, "POST /appointment #%d: %s\n", i+1, s.Round(time.Millisecond))
	}
	return strings.TrimRight(b.String(), "\n")
}

// ---------------------------------------------------------------------------
// Limpeza
// ---------------------------------------------------------------------------

// cleanupAppointments remove as consultas criadas. Roda ANTES da limpeza de
// pessoas/profissionais: apagar o participante primeiro poderia deixar a consulta
// impossível de resolver.
func cleanupAppointments(t *testing.T, c *probeClient) {
	t.Helper()
	removed, failed := 0, 0
	createdAppointmentIDs.Range(func(k, _ any) bool {
		id := k.(string)
		if r := c.do(t, http.MethodDelete, "/appointment/"+id, nil); r.outcome() == accepted {
			removed++
		} else {
			failed++
			t.Logf("limpeza: DELETE /appointment/%s devolveu %d", id, r.status)
		}
		return true
	})
	t.Logf("limpeza de appointments: %d removidos, %d não removidos", removed, failed)
}

// cleanupProfessionals remove os médicos sintéticos.
func cleanupProfessionals(t *testing.T, c *probeClient) {
	t.Helper()
	removed, failed := 0, 0
	createdProfessionalIDs.Range(func(k, _ any) bool {
		id := k.(string)
		if r := c.do(t, http.MethodDelete, "/professional/"+id+"?soft=true", nil); r.outcome() == accepted {
			removed++
		} else {
			failed++
			t.Logf("limpeza: DELETE /professional/%s devolveu %d", id, r.status)
		}
		return true
	})
	t.Logf("limpeza de profissionais: %d removidos, %d não removidos", removed, failed)
}
