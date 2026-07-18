package controllers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/models"
)

// fakeCatalog e fakeEnroll implementam as interfaces que o CONTROLLER declara
// (ADR-012). Struct à mão, sem framework de mock — é o padrão da casa.
type fakeCatalog struct {
	careLine models.CareLine
	item     models.CareLineItem
	rule     models.CareLineRule
	versions []models.CareLine
	err      error

	lastCode   string
	lastItemIn models.AddItemInput
	lastRuleT  string
	lastParams string
}

func (f *fakeCatalog) Create(_ context.Context, code, _, _ string) (models.CareLine, error) {
	f.lastCode = code
	return f.careLine, f.err
}
func (f *fakeCatalog) AddItem(_ context.Context, _ uuid.UUID, in models.AddItemInput) (models.CareLineItem, error) {
	f.lastItemIn = in
	return f.item, f.err
}
func (f *fakeCatalog) AddRule(_ context.Context, _ uuid.UUID, _, ruleType string, params json.RawMessage) (models.CareLineRule, error) {
	f.lastRuleT = ruleType
	f.lastParams = string(params)
	return f.rule, f.err
}
func (f *fakeCatalog) Publish(_ context.Context, _ uuid.UUID, _ time.Time) (models.CareLine, error) {
	return f.careLine, f.err
}
func (f *fakeCatalog) ListVersions(_ context.Context, code string) ([]models.CareLine, error) {
	f.lastCode = code
	return f.versions, f.err
}

type fakeEnroll struct {
	enr        models.Enrollment
	err        error
	lastMonths int
	lastStatus string
}

func (f *fakeEnroll) Enroll(_ context.Context, _ uuid.UUID, _ string, months int, _ time.Time) (models.Enrollment, error) {
	f.lastMonths = months
	return f.enr, f.err
}
func (f *fakeEnroll) Renew(_ context.Context, _ uuid.UUID, months int, _ time.Time) (models.Enrollment, error) {
	f.lastMonths = months
	return f.enr, f.err
}
func (f *fakeEnroll) End(_ context.Context, _ uuid.UUID, status, _ string, _ time.Time) (models.Enrollment, error) {
	f.lastStatus = status
	return f.enr, f.err
}

func adminCtrl(cat controllers.CareLineAdmin, enr controllers.EnrollmentAdmin) controllers.CareLineAdminController {
	return controllers.CareLineAdminController{
		Catalog:     cat,
		Enrollments: enr,
		Now:         func() time.Time { return time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC) },
	}
}

// serveAdmin monta as rotas /admin (sem o middleware de token, que tem teste
// próprio) e devolve a resposta.
func serveAdmin(t *testing.T, c controllers.CareLineAdminController, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Post("/admin/care-lines", c.CreateCareLine)
	r.Get("/admin/care-lines", c.ListCareLines)
	r.Post("/admin/care-lines/{care_line_id}/items", c.CreateCareLineItem)
	r.Post("/admin/care-lines/{care_line_id}/items/{item_ref}/rules", c.CreateCareLineItemRule)
	r.Post("/admin/care-lines/{care_line_id}/publish", c.PublishCareLine)
	r.Post("/admin/enrollments", c.CreateEnrollment)
	r.Post("/admin/enrollments/{enrollment_id}/renew", c.RenewEnrollment)
	r.Post("/admin/enrollments/{enrollment_id}/end", c.EndEnrollment)

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func draftFixture() models.CareLine {
	return models.CareLine{
		ID: uuid.New(), Code: "saude-mental", Version: 1,
		Name: "Saúde Mental", Description: "linha piloto", Status: "draft",
	}
}

func enrollmentFixture() models.Enrollment {
	return models.Enrollment{
		ID: uuid.New(), PatientID: uuid.New(), CareLineCode: "saude-mental",
		CareLineVersion: 1, Status: "ativa",
		ValidFrom:  time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC),
		ValidUntil: time.Date(2026, 9, 18, 8, 0, 0, 0, time.UTC),
		Periods: []models.EnrollmentPeriod{{
			ID:       uuid.New(),
			StartsAt: time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC),
			EndsAt:   time.Date(2026, 9, 18, 8, 0, 0, 0, time.UTC),
			Source:   "admin",
		}},
	}
}

// ---------------------------------------------------------------------------
// Catálogo — casos felizes
// ---------------------------------------------------------------------------

func TestAdmin_CreateCareLine_201(t *testing.T) {
	cat := &fakeCatalog{careLine: draftFixture()}
	w := serveAdmin(t, adminCtrl(cat, &fakeEnroll{}), http.MethodPost, "/admin/care-lines",
		`{"code":"saude-mental","name":"Saúde Mental","description":"linha piloto"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, quero 201 (corpo: %s)", w.Code, w.Body.String())
	}
	if cat.lastCode != "saude-mental" {
		t.Errorf("code passado ao model = %q", cat.lastCode)
	}
	var got struct {
		Code   string `json:"code"`
		Status string `json:"status"`
		Items  []any  `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Code != "saude-mental" || got.Status != "draft" {
		t.Errorf("corpo = %s", w.Body.String())
	}
	if got.Items == nil {
		t.Error("items precisa ser [] e não null (contrato exige o array)")
	}
}

func TestAdmin_ListCareLines_FiltraPorCode(t *testing.T) {
	cat := &fakeCatalog{versions: []models.CareLine{draftFixture()}}
	w := serveAdmin(t, adminCtrl(cat, &fakeEnroll{}), http.MethodGet, "/admin/care-lines?code=saude-mental", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200", w.Code)
	}
	if cat.lastCode != "saude-mental" {
		t.Errorf("code do filtro = %q", cat.lastCode)
	}
	if !strings.Contains(w.Body.String(), `"items"`) {
		t.Errorf("resposta precisa ser CareLineList: %s", w.Body.String())
	}
}

func TestAdmin_CreateItem_201_KindDefault(t *testing.T) {
	cat := &fakeCatalog{item: models.CareLineItem{
		ID: uuid.New(), Ref: "aval", Kind: "CONSULTA", SpecialtyCode: "Psicologia", Label: "Avaliação",
	}}
	w := serveAdmin(t, adminCtrl(cat, &fakeEnroll{}), http.MethodPost,
		"/admin/care-lines/"+uuid.NewString()+"/items",
		`{"ref":"aval","kind":"CONSULTA","specialty_code":"Psicologia","label":"Avaliação"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, quero 201 (corpo: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"rules":[]`) {
		t.Errorf("CareLineItem precisa trazer rules como array vazio: %s", w.Body.String())
	}
}

func TestAdmin_CreateRule_201(t *testing.T) {
	cat := &fakeCatalog{rule: models.CareLineRule{RuleType: "QUOTA", Params: json.RawMessage(`{"max":4,"period":"month"}`)}}
	w := serveAdmin(t, adminCtrl(cat, &fakeEnroll{}), http.MethodPost,
		"/admin/care-lines/"+uuid.NewString()+"/items/aval/rules",
		`{"rule_type":"QUOTA","params":{"max":4,"period":"month"}}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, quero 201 (corpo: %s)", w.Code, w.Body.String())
	}
	if cat.lastRuleT != "QUOTA" {
		t.Errorf("rule_type passado = %q", cat.lastRuleT)
	}
}

func TestAdmin_Publish_200(t *testing.T) {
	pub := draftFixture()
	pub.Status = "published"
	cat := &fakeCatalog{careLine: pub}
	w := serveAdmin(t, adminCtrl(cat, &fakeEnroll{}), http.MethodPost,
		"/admin/care-lines/"+uuid.NewString()+"/publish", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"published"`) {
		t.Errorf("corpo = %s", w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Catálogo — mapeamento de erros
// ---------------------------------------------------------------------------

func TestAdmin_CreateCareLine_DraftDuplicado_409(t *testing.T) {
	cat := &fakeCatalog{err: models.ErrCareLineDraftExists}
	w := serveAdmin(t, adminCtrl(cat, &fakeEnroll{}), http.MethodPost, "/admin/care-lines",
		`{"code":"saude-mental","name":"x"}`)

	assertReason(t, w, http.StatusConflict, "CARE_LINE_DRAFT_EXISTS")
}

func TestAdmin_CreateItem_LinhaPublicada_409(t *testing.T) {
	cat := &fakeCatalog{err: models.ErrCareLinePublished}
	w := serveAdmin(t, adminCtrl(cat, &fakeEnroll{}), http.MethodPost,
		"/admin/care-lines/"+uuid.NewString()+"/items",
		`{"ref":"aval","kind":"CONSULTA","specialty_code":"Psicologia","label":"Avaliação"}`)

	assertReason(t, w, http.StatusConflict, "CARE_LINE_PUBLISHED")
}

func TestAdmin_Publish_Invalido_400ComErrors(t *testing.T) {
	cat := &fakeCatalog{err: models.ErrCareLineInvalid{Errors: []string{
		"item \"aval\": especialidade \"Xpto\" não encontrada no legado",
		"ciclo de pré-requisitos: a -> b -> a",
	}}}
	w := serveAdmin(t, adminCtrl(cat, &fakeEnroll{}), http.MethodPost,
		"/admin/care-lines/"+uuid.NewString()+"/publish", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (corpo: %s)", w.Code, w.Body.String())
	}
	var p struct {
		Errors []string `json:"errors"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if len(p.Errors) != 2 {
		t.Errorf("errors[] devia trazer os 2 problemas de uma vez, veio: %s", w.Body.String())
	}
}

func TestAdmin_Publish_LegadoForaDoAr_503(t *testing.T) {
	cat := &fakeCatalog{err: agenda.ErrUnavailable}
	w := serveAdmin(t, adminCtrl(cat, &fakeEnroll{}), http.MethodPost,
		"/admin/care-lines/"+uuid.NewString()+"/publish", "")

	assertReason(t, w, http.StatusServiceUnavailable, "LEGACY_UNAVAILABLE")
}

func TestAdmin_Publish_NaoEncontrada_404(t *testing.T) {
	cat := &fakeCatalog{err: models.ErrCareLineNotFound}
	w := serveAdmin(t, adminCtrl(cat, &fakeEnroll{}), http.MethodPost,
		"/admin/care-lines/"+uuid.NewString()+"/publish", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, quero 404", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Matrícula
// ---------------------------------------------------------------------------

func TestAdmin_CreateEnrollment_201(t *testing.T) {
	enr := &fakeEnroll{enr: enrollmentFixture()}
	w := serveAdmin(t, adminCtrl(&fakeCatalog{}, enr), http.MethodPost, "/admin/enrollments",
		`{"patient_id":"`+uuid.NewString()+`","care_line_code":"saude-mental","months":2}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, quero 201 (corpo: %s)", w.Code, w.Body.String())
	}
	if enr.lastMonths != 2 {
		t.Errorf("months passado = %d", enr.lastMonths)
	}
	if !strings.Contains(w.Body.String(), `"periods"`) {
		t.Errorf("Enrollment precisa trazer periods: %s", w.Body.String())
	}
}

func TestAdmin_CreateEnrollment_MatriculaViva_409(t *testing.T) {
	enr := &fakeEnroll{err: models.ErrEnrollmentAlive}
	w := serveAdmin(t, adminCtrl(&fakeCatalog{}, enr), http.MethodPost, "/admin/enrollments",
		`{"patient_id":"`+uuid.NewString()+`","care_line_code":"saude-mental","months":1}`)

	assertReason(t, w, http.StatusConflict, "ENROLLMENT_ALIVE")
}

func TestAdmin_CreateEnrollment_PacienteInexistente_404(t *testing.T) {
	enr := &fakeEnroll{err: models.ErrPatientNotFound}
	w := serveAdmin(t, adminCtrl(&fakeCatalog{}, enr), http.MethodPost, "/admin/enrollments",
		`{"patient_id":"`+uuid.NewString()+`","care_line_code":"saude-mental","months":1}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, quero 404", w.Code)
	}
}

func TestAdmin_CreateEnrollment_LinhaSemVersaoPublicada_404(t *testing.T) {
	enr := &fakeEnroll{err: models.ErrCareLineNotPublished}
	w := serveAdmin(t, adminCtrl(&fakeCatalog{}, enr), http.MethodPost, "/admin/enrollments",
		`{"patient_id":"`+uuid.NewString()+`","care_line_code":"saude-mental","months":1}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, quero 404", w.Code)
	}
}

func TestAdmin_CreateEnrollment_MesesInvalidos_400(t *testing.T) {
	for _, body := range []string{
		`{"patient_id":"` + uuid.NewString() + `","care_line_code":"x","months":0}`,
		`{"patient_id":"` + uuid.NewString() + `","care_line_code":"x","months":4}`,
	} {
		enr := &fakeEnroll{enr: enrollmentFixture()}
		w := serveAdmin(t, adminCtrl(&fakeCatalog{}, enr), http.MethodPost, "/admin/enrollments", body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, quero 400", body, w.Code)
		}
		if enr.lastMonths != 0 {
			t.Errorf("months fora de {1,2,3} não pode chegar ao model")
		}
	}
}

func TestAdmin_RenewEnrollment_200(t *testing.T) {
	enr := &fakeEnroll{enr: enrollmentFixture()}
	w := serveAdmin(t, adminCtrl(&fakeCatalog{}, enr), http.MethodPost,
		"/admin/enrollments/"+uuid.NewString()+"/renew", `{"months":3}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", w.Code, w.Body.String())
	}
	if enr.lastMonths != 3 {
		t.Errorf("months passado = %d", enr.lastMonths)
	}
}

func TestAdmin_RenewEnrollment_Fechada_409(t *testing.T) {
	enr := &fakeEnroll{err: models.ErrEnrollmentClosed}
	w := serveAdmin(t, adminCtrl(&fakeCatalog{}, enr), http.MethodPost,
		"/admin/enrollments/"+uuid.NewString()+"/renew", `{"months":1}`)
	assertReason(t, w, http.StatusConflict, "ENROLLMENT_CLOSED")
}

func TestAdmin_EndEnrollment_200(t *testing.T) {
	enr := &fakeEnroll{enr: enrollmentFixture()}
	w := serveAdmin(t, adminCtrl(&fakeCatalog{}, enr), http.MethodPost,
		"/admin/enrollments/"+uuid.NewString()+"/end", `{"status":"encerrada","reason":"pediu"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, quero 200 (corpo: %s)", w.Code, w.Body.String())
	}
	if enr.lastStatus != "encerrada" {
		t.Errorf("status passado = %q", enr.lastStatus)
	}
}

func TestAdmin_EndEnrollment_Fechada_409(t *testing.T) {
	enr := &fakeEnroll{err: models.ErrEnrollmentClosed}
	w := serveAdmin(t, adminCtrl(&fakeCatalog{}, enr), http.MethodPost,
		"/admin/enrollments/"+uuid.NewString()+"/end", `{"status":"concluida","reason":"terminou"}`)
	assertReason(t, w, http.StatusConflict, "ENROLLMENT_CLOSED")
}

func TestAdmin_EndEnrollment_StatusInvalido_400(t *testing.T) {
	enr := &fakeEnroll{enr: enrollmentFixture()}
	w := serveAdmin(t, adminCtrl(&fakeCatalog{}, enr), http.MethodPost,
		"/admin/enrollments/"+uuid.NewString()+"/end", `{"status":"pausada","reason":"x"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quero 400 (só concluida|encerrada)", w.Code)
	}
	if enr.lastStatus != "" {
		t.Errorf("status inválido não pode chegar ao model")
	}
}

func assertReason(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("status = %d, quero %d (corpo: %s)", w.Code, wantStatus, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Errorf("content-type = %q, quero problem+json", ct)
	}
	var p struct {
		Reason struct {
			Code string `json:"code"`
		} `json:"reason"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if p.Reason.Code != wantCode {
		t.Errorf("reason.code = %q, quero %q (corpo: %s)", p.Reason.Code, wantCode, w.Body.String())
	}
}
