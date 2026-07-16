package dav

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// CreateAppointmentInput são os dados da consulta a criar na DAV.
//
// Repare no que NÃO tem aqui: um `id` nosso. A DAV aceita id do integrador em
// /person e em /professional, mas RECUSA no appointment — 400 "property id should
// not exist" (achado #12). É a diferença que muda toda a saga: o cadastro é
// idempotente e sondável; o agendamento não é nenhum dos dois.
type CreateAppointmentInput struct {
	Title    string
	StartsAt time.Time
	EndsAt   time.Time
	// Specialty vira `appointment_specialty`, texto livre. É o NOME da
	// especialidade no legado — o slot não a determina (o vínculo
	// profissional-especialidade é muitos-para-muitos), então quem escolheu foi o
	// paciente.
	Specialty string
	// ProfessionalID é o participante MMD. É o `tb_professionals.id` do legado,
	// que é TAMBÉM o id do profissional na DAV — no recurso /professional, não
	// /person (sondado: GET /professional/{id} devolve 200; /person/{id} devolve
	// 204). Por isso não existe tabela de mapeamento em lugar nenhum.
	ProfessionalID string
	// PatientID é o participante PAT: o `patient_account.dav_person_id`, que é o
	// nosso UUIDv7 aceito por eles no cadastro (achado #4).
	PatientID string
}

// Appointment é o que a DAV devolve ao criar.
type Appointment struct {
	// ID é o id DELES. Guardamos porque é a única forma de voltar a falar sobre
	// esta consulta — e porque, se o POST não responder, ele se perde para sempre:
	// a DAV não tem rota de busca de appointment.
	ID string
	// PatientJoinURL é o link que o paciente clica. É CREDENCIAL: nunca logar,
	// nunca devolver em listagem (ver JoinTicket no openapi.yaml).
	PatientJoinURL string
}

// CreateAppointment cria a consulta e devolve o link do paciente.
//
// NUNCA repete, como o CreatePerson (ADR-011b) — mas aqui a consequência é bem
// pior. Lá, um POST que estourou pode ser reconciliado: o id é nosso, então
// GET /person/{nosso-id} responde se pegou. Aqui o id é DELES e só chega na
// resposta: se a resposta não chegar, a consulta pode existir sem que exista
// qualquer forma de encontrá-la (não há listagem, não há busca — sondado).
//
// Por isso o erro é ErrMaybeApplied e quem chama NÃO PODE:
//   - repetir (criaria uma segunda consulta de verdade: a DAV aceita duas no
//     mesmo horário para o mesmo profissional — achado #17); nem
//   - concluir que falhou e devolver o horário (se a consulta fantasma existir,
//     outro paciente marca por cima e o profissional recebe dois).
//
// O único caminho correto é segurar o horário e mandar para revisão humana.
func (c *Client) CreateAppointment(ctx context.Context, in CreateAppointmentInput) (Appointment, error) {
	const route = "POST /appointment"

	body := createAppointmentBody{
		Title: in.Title,
		// RFC3339 com offset. A sondagem provou que a DAV respeita o `-03:00` que
		// mandamos (achado #16), então não convertemos para UTC: o horário viaja
		// como o negócio o enxerga.
		StartDateTime: in.StartsAt.Format(time.RFC3339),
		EndDateTime:   in.EndsAt.Format(time.RFC3339),
		// Único valor do enum deles.
		AppointmentReason:    "elective",
		AppointmentSpecialty: in.Specialty,
		Participants: []participantBody{
			{ID: in.ProfessionalID, Role: "MMD"},
			{ID: in.PatientID, Role: "PAT"},
		},
	}

	// UMA tentativa, nunca mais. Ver o comentário da função.
	res, err := c.doWithRetry(ctx, http.MethodPost, "/appointment", route, body, 1)
	if err != nil {
		return Appointment{}, fmt.Errorf("%w: %w", ErrMaybeApplied, err)
	}
	if res.status != http.StatusOK && res.status != http.StatusCreated {
		return Appointment{}, c.classifyWrite(res, route)
	}

	var out struct {
		ID           string `json:"id"`
		Participants []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"participants"`
	}
	if err := json.Unmarshal(res.body, &out); err != nil || out.ID == "" {
		// 2xx significa que a DAV ACEITOU: a consulta existe lá. ErrUnavailable
		// faria o chamador concluir "falhou" e devolver o horário — e aí outro
		// paciente marcaria por cima de uma consulta real.
		return Appointment{}, fmt.Errorf("%w: %s respondeu %d sem id utilizável", ErrMaybeApplied, route, res.status)
	}

	// A url do paciente sai casando pelo ID que enviamos, e não pelo `role`.
	// A resposta real traz `role` — mas o ParticipantResponseSchema deles declara
	// só {id, url}. Depender de um campo que o próprio spec diz não existir é
	// construir sobre algo que eles podem remover sem avisar; o id é o que NÓS
	// controlamos.
	var joinURL string
	for _, p := range out.Participants {
		if p.ID == in.PatientID {
			joinURL = p.URL
			break
		}
	}
	if joinURL == "" {
		// A consulta foi criada (201) mas não achamos o link do paciente. NÃO é
		// ErrUnavailable: aquilo significaria "não aconteceu", e aconteceu. É o
		// mesmo desconhecido de sempre — segura o horário, chama gente.
		return Appointment{}, fmt.Errorf("%w: %s criou a consulta %s mas não devolveu url para o paciente",
			ErrMaybeApplied, route, out.ID)
	}

	return Appointment{ID: out.ID, PatientJoinURL: joinURL}, nil
}

type participantBody struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

type createAppointmentBody struct {
	Title                string            `json:"title"`
	StartDateTime        string            `json:"start_date_time"`
	EndDateTime          string            `json:"end_date_time"`
	AppointmentReason    string            `json:"appointment_reason"`
	AppointmentSpecialty string            `json:"appointment_specialty,omitempty"`
	Participants         []participantBody `json:"participants"`
	// `url` NÃO vai no participante, apesar de o ParticipantRequestSchema deles
	// declarar `required: [id, role, url]`. Era impossível mesmo — a url é o que
	// eles geram — e a sondagem confirmou que o payload sem ela é aceito
	// (achado #13).
}
