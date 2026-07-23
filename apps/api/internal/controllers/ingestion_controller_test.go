package controllers_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/models"
)

type fakeIngestion struct {
	recordResult  models.RecordResult
	recordErr     error
	gotRecord     models.ContractPush
	resendResult  models.ResendResult
	resendErr     error
	gotResendHmac []byte
}

func (f *fakeIngestion) RecordContract(_ context.Context, in models.ContractPush) (models.RecordResult, error) {
	f.gotRecord = in
	return f.recordResult, f.recordErr
}

func (f *fakeIngestion) ResendInvite(_ context.Context, cpfHmac []byte) (models.ResendResult, error) {
	f.gotResendHmac = cpfHmac
	return f.resendResult, f.resendErr
}

func serveIngestion(f *fakeIngestion) http.Handler {
	c := controllers.IngestionController{Ingestion: f}
	r := chi.NewRouter()
	r.Post("/integration/gestao/contracts", c.RecordContract)
	r.Post("/integration/gestao/employees/{cpf_hmac}/resend-invite", c.ResendInvite)
	return r
}

var cpfHmacHex = hex.EncodeToString(bytes.Repeat([]byte{0x01}, 32))

func contractBody(cpfHmac string) string {
	return `{
		"contract_id": "C-1",
		"status": "ativo",
		"started_at": "2026-07-22T00:00:00Z",
		"employee": {"id": "E-1", "cpf_hmac": "` + cpfHmac + `", "name": "Maria", "email": "m@e.test"},
		"company": {"id": "CO-1", "display_name": "ACME"}
	}`
}

func postIngestion(h http.Handler, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	return w
}

func TestRecordContract_Ok(t *testing.T) {
	exp := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	f := &fakeIngestion{recordResult: models.RecordResult{
		PersonStatus: "pendente", ContractStatus: "ativo", InviteSent: true,
		InviteURL: "https://app.test/onboarding/ABC", InviteExpiresAt: &exp,
	}}
	w := postIngestion(serveIngestion(f), "/integration/gestao/contracts", contractBody(cpfHmacHex))

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out api.GestaoContractPushResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.EqualValues(t, "pendente", out.PersonStatus)
	assert.EqualValues(t, "ativo", out.ContractStatus)
	assert.True(t, out.InviteSent)
	require.NotNil(t, out.InviteUrl)
	assert.Equal(t, "https://app.test/onboarding/ABC", *out.InviteUrl)

	// O controller decodifica o cpf_hmac de hex para 32 bytes antes de chamar o model.
	assert.Len(t, f.gotRecord.Employee.CPFHmac, 32)
	assert.Equal(t, "C-1", f.gotRecord.ContractID)
}

func TestRecordContract_SemConvite_NaoTrazURL(t *testing.T) {
	f := &fakeIngestion{recordResult: models.RecordResult{
		PersonStatus: "pendente", ContractStatus: "ativo", InviteSent: false,
	}}
	w := postIngestion(serveIngestion(f), "/integration/gestao/contracts", contractBody(cpfHmacHex))

	require.Equal(t, http.StatusOK, w.Code)
	var out api.GestaoContractPushResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.False(t, out.InviteSent)
	assert.Nil(t, out.InviteUrl, "sem convite, invite_url é omitido")
}

func TestRecordContract_CorpoInvalido(t *testing.T) {
	w := postIngestion(serveIngestion(&fakeIngestion{}), "/integration/gestao/contracts", "{ nao é json")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRecordContract_CPFHmacInvalido(t *testing.T) {
	for _, bad := range []string{"nao-hex", hex.EncodeToString([]byte{0x01, 0x02})} { // não-hex e curto
		w := postIngestion(serveIngestion(&fakeIngestion{}), "/integration/gestao/contracts", contractBody(bad))
		assert.Equal(t, http.StatusBadRequest, w.Code, "cpf_hmac %q deveria ser 400", bad)
	}
}

func TestRecordContract_ModelInvalido(t *testing.T) {
	f := &fakeIngestion{recordErr: models.ErrInvalidContractPush}
	w := postIngestion(serveIngestion(f), "/integration/gestao/contracts", contractBody(cpfHmacHex))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRecordContract_ErroInterno(t *testing.T) {
	f := &fakeIngestion{recordErr: errors.New("boom no banco")}
	w := postIngestion(serveIngestion(f), "/integration/gestao/contracts", contractBody(cpfHmacHex))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "boom no banco", "erro interno não vaza detalhe")
}

func TestResendInvite_Ok(t *testing.T) {
	exp := time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)
	f := &fakeIngestion{resendResult: models.ResendResult{InviteURL: "https://app.test/onboarding/XYZ", ExpiresAt: exp}}
	w := postIngestion(serveIngestion(f), "/integration/gestao/employees/"+cpfHmacHex+"/resend-invite", "")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out api.ResendInviteResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "https://app.test/onboarding/XYZ", out.InviteUrl)
	assert.Len(t, f.gotResendHmac, 32)
}

func TestResendInvite_CPFHmacInvalido(t *testing.T) {
	w := postIngestion(serveIngestion(&fakeIngestion{}), "/integration/gestao/employees/nao-hex/resend-invite", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestResendInvite_Desconhecido(t *testing.T) {
	f := &fakeIngestion{resendErr: models.ErrEmployeeUnknown}
	w := postIngestion(serveIngestion(f), "/integration/gestao/employees/"+cpfHmacHex+"/resend-invite", "")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestResendInvite_JaTemConta(t *testing.T) {
	f := &fakeIngestion{resendErr: models.ErrAlreadyHasAccount}
	w := postIngestion(serveIngestion(f), "/integration/gestao/employees/"+cpfHmacHex+"/resend-invite", "")
	require.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "ALREADY_HAS_ACCOUNT")
}
