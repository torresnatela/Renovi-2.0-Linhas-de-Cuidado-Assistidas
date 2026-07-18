package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/models"
)

// CareLineAdmin é o que o controller precisa do catálogo (interface no consumidor,
// ADR-012). Os stores dos models a implementam.
type CareLineAdmin interface {
	Create(ctx context.Context, code, name, description string) (models.CareLine, error)
	AddItem(ctx context.Context, careLineID uuid.UUID, in models.AddItemInput) (models.CareLineItem, error)
	AddRule(ctx context.Context, careLineID uuid.UUID, itemRef, ruleType string, params json.RawMessage) (models.CareLineRule, error)
	Publish(ctx context.Context, id uuid.UUID, now time.Time) (models.CareLine, error)
	ListVersions(ctx context.Context, code string) ([]models.CareLine, error)
}

// EnrollmentAdmin é o que o controller precisa da matrícula.
type EnrollmentAdmin interface {
	Enroll(ctx context.Context, patientID uuid.UUID, careLineCode string, months int, now time.Time) (models.Enrollment, error)
	Renew(ctx context.Context, id uuid.UUID, months int, now time.Time) (models.Enrollment, error)
	End(ctx context.Context, id uuid.UUID, status, reason string, now time.Time) (models.Enrollment, error)
}

// CareLineAdminController expõe as rotas /admin/* (catálogo + matrícula).
type CareLineAdminController struct {
	Catalog     CareLineAdmin
	Enrollments EnrollmentAdmin
	// Now é injetável para o teste não depender do relógio da máquina.
	Now func() time.Time
}

func (c CareLineAdminController) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// ---------------------------------------------------------------------------
// Catálogo
// ---------------------------------------------------------------------------

func (c CareLineAdminController) CreateCareLine(w http.ResponseWriter, r *http.Request) {
	var body api.CreateCareLineRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	line, err := c.Catalog.Create(r.Context(), body.Code, body.Name, deref(body.Description))
	if err != nil {
		writeCareLineError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, toAPICareLine(line))
}

func (c CareLineAdminController) ListCareLines(w http.ResponseWriter, r *http.Request) {
	lines, err := c.Catalog.ListVersions(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		writeCareLineError(w, err)
		return
	}
	out := make([]api.CareLine, 0, len(lines))
	for _, l := range lines {
		out = append(out, toAPICareLine(l))
	}
	WriteJSON(w, http.StatusOK, api.CareLineList{Items: out})
}

func (c CareLineAdminController) CreateCareLineItem(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "care_line_id")
	if !ok {
		return
	}
	var body api.CreateCareLineItemRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	item, err := c.Catalog.AddItem(r.Context(), id, models.AddItemInput{
		Ref:           body.Ref,
		Kind:          string(body.Kind),
		SpecialtyCode: body.SpecialtyCode,
		Label:         body.Label,
		Recurrence:    body.Recurrence,
		SortOrder:     body.SortOrder,
	})
	if err != nil {
		writeCareLineError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, toAPICareLineItem(item))
}

func (c CareLineAdminController) CreateCareLineItemRule(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "care_line_id")
	if !ok {
		return
	}
	itemRef := chi.URLParam(r, "item_ref")

	var body api.CreateCareLineRuleRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	// Reserializa os params para o RawMessage que o motor valida. O map já veio
	// decodificado do corpo; remontar o JSON é o formato que o store espera.
	params, err := json.Marshal(body.Params)
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "requisição inválida", "params inválidos")
		return
	}

	rule, err := c.Catalog.AddRule(r.Context(), id, itemRef, string(body.RuleType), params)
	if err != nil {
		writeCareLineError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, toAPICareLineRule(rule))
}

func (c CareLineAdminController) PublishCareLine(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "care_line_id")
	if !ok {
		return
	}
	line, err := c.Catalog.Publish(r.Context(), id, c.now())
	if err != nil {
		writeCareLineError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, toAPICareLine(line))
}

// ---------------------------------------------------------------------------
// Matrícula
// ---------------------------------------------------------------------------

func (c CareLineAdminController) CreateEnrollment(w http.ResponseWriter, r *http.Request) {
	var body api.CreateEnrollmentRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	months := int(body.Months)
	if !monthsValid(months) {
		writeInvalidMonths(w)
		return
	}
	enr, err := c.Enrollments.Enroll(r.Context(), body.PatientId, body.CareLineCode, months, c.now())
	if err != nil {
		writeEnrollmentError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, toAPIEnrollment(enr))
}

func (c CareLineAdminController) RenewEnrollment(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "enrollment_id")
	if !ok {
		return
	}
	var body api.RenewEnrollmentRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	months := int(body.Months)
	if !monthsValid(months) {
		writeInvalidMonths(w)
		return
	}
	enr, err := c.Enrollments.Renew(r.Context(), id, months, c.now())
	if err != nil {
		writeEnrollmentError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, toAPIEnrollment(enr))
}

func (c CareLineAdminController) EndEnrollment(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "enrollment_id")
	if !ok {
		return
	}
	var body api.EndEnrollmentRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	status := string(body.Status)
	if status != "concluida" && status != "encerrada" {
		WriteProblem(w, http.StatusBadRequest, "requisição inválida",
			"status deve ser concluida ou encerrada")
		return
	}
	enr, err := c.Enrollments.End(r.Context(), id, status, body.Reason, c.now())
	if err != nil {
		writeEnrollmentError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, toAPIEnrollment(enr))
}

// ---------------------------------------------------------------------------
// Erros
// ---------------------------------------------------------------------------

func writeCareLineError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrCareLineDraftExists):
		WriteProblemReason(w, http.StatusConflict, "rascunho já existe",
			"já existe uma linha em rascunho com este code.",
			Reason{Code: "CARE_LINE_DRAFT_EXISTS"})
	case errors.Is(err, models.ErrCareLinePublished):
		WriteProblemReason(w, http.StatusConflict, "linha publicada",
			"esta linha já está publicada e é imutável.",
			Reason{Code: "CARE_LINE_PUBLISHED"})
	case errors.Is(err, models.ErrCareLineNotFound):
		WriteProblem(w, http.StatusNotFound, "não encontrado", "linha de cuidado não encontrada.")
	case errors.Is(err, agenda.ErrUnavailable):
		WriteProblemReason(w, http.StatusServiceUnavailable, "agenda indisponível",
			"não conseguimos validar as especialidades no legado agora. Tente novamente em instantes.",
			Reason{Code: "LEGACY_UNAVAILABLE"})
	default:
		var inv models.ErrCareLineInvalid
		if errors.As(err, &inv) {
			WriteProblemFull(w, Problem{
				Title:  "linha de cuidado inválida",
				Status: http.StatusBadRequest,
				Detail: "corrija os problemas apontados e tente de novo.",
				Errors: inv.Errors,
			})
			return
		}
		WriteProblem(w, http.StatusInternalServerError, "erro interno", "não foi possível concluir a operação.")
	}
}

func writeEnrollmentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrEnrollmentAlive):
		WriteProblemReason(w, http.StatusConflict, "matrícula já existe",
			"o paciente já tem uma matrícula viva nesta linha.",
			Reason{Code: "ENROLLMENT_ALIVE"})
	case errors.Is(err, models.ErrEnrollmentClosed):
		WriteProblemReason(w, http.StatusConflict, "matrícula finalizada",
			"esta matrícula está em desfecho final e não pode mais ser alterada.",
			Reason{Code: "ENROLLMENT_CLOSED"})
	case errors.Is(err, models.ErrEnrollmentNotFound):
		WriteProblem(w, http.StatusNotFound, "não encontrado", "matrícula não encontrada.")
	case errors.Is(err, models.ErrPatientNotFound):
		WriteProblem(w, http.StatusNotFound, "não encontrado", "paciente não encontrado.")
	case errors.Is(err, models.ErrCareLineNotPublished):
		WriteProblem(w, http.StatusNotFound, "não encontrado", "esta linha não tem versão publicada.")
	case errors.Is(err, models.ErrInvalidMonths):
		writeInvalidMonths(w)
	case errors.Is(err, models.ErrInvalidEndStatus):
		WriteProblem(w, http.StatusBadRequest, "requisição inválida",
			"status deve ser concluida ou encerrada")
	default:
		WriteProblem(w, http.StatusInternalServerError, "erro interno", "não foi possível concluir a operação.")
	}
}

func writeInvalidMonths(w http.ResponseWriter) {
	WriteProblem(w, http.StatusBadRequest, "requisição inválida", "months deve ser 1, 2 ou 3.")
}

func monthsValid(m int) bool { return m >= 1 && m <= 3 }

// parseUUIDParam lê um UUID do path. Malformado é 400 (rota autenticada por token,
// não há oráculo de id a proteger como nas rotas de paciente).
func parseUUIDParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		WriteProblem(w, http.StatusBadRequest, "requisição inválida", "identificador inválido.")
		return uuid.UUID{}, false
	}
	return id, true
}

// ---------------------------------------------------------------------------
// Mapeamento domínio -> api (tipos gerados, fiéis ao contrato)
// ---------------------------------------------------------------------------

func toAPICareLine(l models.CareLine) api.CareLine {
	items := make([]api.CareLineItem, 0, len(l.Items))
	for _, it := range l.Items {
		items = append(items, toAPICareLineItem(it))
	}
	return api.CareLine{
		Id:          l.ID,
		Code:        l.Code,
		Version:     l.Version,
		Name:        l.Name,
		Description: l.Description,
		Status:      api.CareLineStatus(l.Status),
		PublishedAt: l.PublishedAt,
		Items:       items,
	}
}

func toAPICareLineItem(it models.CareLineItem) api.CareLineItem {
	rules := make([]api.CareLineRule, 0, len(it.Rules))
	for _, ru := range it.Rules {
		rules = append(rules, toAPICareLineRule(ru))
	}
	return api.CareLineItem{
		Id:            it.ID,
		Ref:           it.Ref,
		Kind:          api.CareLineItemKind(it.Kind),
		SpecialtyCode: it.SpecialtyCode,
		Label:         it.Label,
		Recurrence:    it.Recurrence,
		SortOrder:     it.SortOrder,
		Rules:         rules,
	}
}

func toAPICareLineRule(ru models.CareLineRule) api.CareLineRule {
	params := map[string]interface{}{}
	if len(ru.Params) > 0 {
		_ = json.Unmarshal(ru.Params, &params)
	}
	return api.CareLineRule{
		RuleType: api.CareLineRuleRuleType(ru.RuleType),
		Params:   params,
	}
}

func toAPIEnrollment(e models.Enrollment) api.Enrollment {
	periods := make([]api.EnrollmentPeriod, 0, len(e.Periods))
	for _, p := range e.Periods {
		periods = append(periods, api.EnrollmentPeriod{
			Id:       p.ID,
			StartsAt: p.StartsAt,
			EndsAt:   p.EndsAt,
			Source:   api.EnrollmentPeriodSource(p.Source),
		})
	}
	return api.Enrollment{
		Id:              e.ID,
		PatientId:       e.PatientID,
		CareLineCode:    e.CareLineCode,
		CareLineVersion: e.CareLineVersion,
		Status:          api.EnrollmentStatus(e.Status),
		ValidFrom:       e.ValidFrom,
		ValidUntil:      e.ValidUntil,
		Periods:         periods,
	}
}
