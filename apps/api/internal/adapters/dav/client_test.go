package dav_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/renovisaude/renovi-care/internal/adapters/dav"
)

// Corpos reais capturados da HML pela sondagem (`make dav-probe`).
// Ver docs/DAV-API-NOTAS.md — não invente payloads aqui.
const (
	bodyDuplicateCPF = `{
	  "code": 422, "message": "Person invalid", "trace": "955dc48",
	  "i18n": {"phrase": "entity.validation.exception", "mustache": {"entity": "Person"}},
	  "detail": [{"message": "A person with same CPF already exists",
	    "i18n": {"phrase": "entity.unique.attribute.already.exists", "mustache": {"field": "cpf"}}}]
	}`
	bodyInvalidCPF = `{
	  "code": 422, "message": "Person invalid", "trace": "6313a6a",
	  "i18n": {"phrase": "entity.validation.exception", "mustache": {"entity": "Person"}},
	  "detail": [{"message": "Invalid CPF",
	    "i18n": {"phrase": "attribute.is.not.valid", "mustache": {"field": "cpf"}}}]
	}`
	// Note a forma diferente: sem i18n, sem detail, mensagem cravada em português.
	bodyDuplicateEmail = `{"code": 422, "message": "Email já cadastrado.", "trace": "219372e"}`
	bodyGatewayTimeout = `{"message": "Endpoint request timed out"}`
)

func validInput() dav.CreatePersonInput {
	return dav.CreatePersonInput{
		ID:        "019f6c3e-7659-7e86-8c93-b0009ced58d7",
		Name:      "Roberval Juvencio Lazaroti",
		CPF:       "94819089846",
		BirthDate: "1976-01-23",
		Email:     "roberval@example.com",
	}
}

// newClient monta um client apontado para o servidor falso, com backoff curto
// para o teste não gastar segundos de verdade.
func newClient(t *testing.T, srv *httptest.Server, logger *slog.Logger) *dav.Client {
	t.Helper()
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	c, err := dav.New(dav.Config{
		BaseURL:     srv.URL,
		APIKey:      "chave-de-teste",
		Timeout:     2 * time.Second,
		MaxAttempts: 3,
		BaseBackoff: time.Millisecond,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("dav.New: %v", err)
	}
	return c
}

// ---------------------------------------------------------------------------
// FindPersonByCPF
// ---------------------------------------------------------------------------

// 204 é "não achei", não "sucesso vazio". É o erro mais fácil de cometer aqui:
// a DAV não usa 404 (confirmado na sondagem, achado #1).
func TestFindPersonByCPF_204NaoEncontrado(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p, err := newClient(t, srv, nil).FindPersonByCPF(context.Background(), "94819089846")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if p != nil {
		t.Errorf("204 devolveu pessoa %+v; queria nil", p)
	}
}

func TestFindPersonByCPF_200Encontrado(t *testing.T) {
	var gotPath, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey = r.URL.Path, r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"dav-123","name":"Maria","cpf":"94819089846","birth_date":"1990-01-02","email":"m@x.com"}`)
	}))
	defer srv.Close()

	p, err := newClient(t, srv, nil).FindPersonByCPF(context.Background(), "94819089846")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if p == nil {
		t.Fatal("200 devolveu nil; queria a pessoa")
	}
	if p.ID != "dav-123" {
		t.Errorf("ID = %q; quero dav-123", p.ID)
	}
	if gotPath != "/person/cpf/94819089846" {
		t.Errorf("path = %q; quero /person/cpf/94819089846", gotPath)
	}
	if gotKey != "chave-de-teste" {
		t.Errorf("x-api-key = %q; a chave não foi enviada", gotKey)
	}
}

// ---------------------------------------------------------------------------
// CreatePerson — sucesso e mapeamento de erro
// ---------------------------------------------------------------------------

// A DAV aceita nosso id (achado #4) — é o que torna o POST sondável.
// E mandamos status sempre: a sondagem diz que é opcional (#2), mas o spec o
// declara obrigatório; enviar satisfaz os dois.
func TestCreatePerson_EnviaNossoIDEStatus(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"019f6c3e-7659-7e86-8c93-b0009ced58d7"}`)
	}))
	defer srv.Close()

	id, err := newClient(t, srv, nil).CreatePerson(context.Background(), validInput())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if id != "019f6c3e-7659-7e86-8c93-b0009ced58d7" {
		t.Errorf("id = %q; quero o nosso UUIDv7 de volta", id)
	}
	if body["id"] != "019f6c3e-7659-7e86-8c93-b0009ced58d7" {
		t.Errorf("body[id] = %v; o nosso id não foi enviado", body["id"])
	}
	if body["status"] != true {
		t.Errorf("body[status] = %v; quero true sempre", body["status"])
	}
}

// A DAV pode devolver um id diferente do que mandamos. Confiar cegamente no
// nosso deixaria o vínculo apontando para o lugar errado.
func TestCreatePerson_UsaOIDQueADAVDevolve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"id-escolhido-pela-dav"}`)
	}))
	defer srv.Close()

	id, err := newClient(t, srv, nil).CreatePerson(context.Background(), validInput())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if id != "id-escolhido-pela-dav" {
		t.Errorf("id = %q; quero o id devolvido pela DAV, não o nosso", id)
	}
}

// CPF duplicado e DV inválido têm a MESMA message ("Person invalid"). Só o
// detail[].i18n.phrase os separa — e o cadastro precisa distinguir, porque um é
// "essa pessoa já existe" e o outro é "seu CPF está errado".
func TestCreatePerson_MapeiaOs422(t *testing.T) {
	tests := []struct {
		nome string
		body string
		want error
	}{
		{"CPF duplicado", bodyDuplicateCPF, dav.ErrDuplicateCPF},
		{"e-mail duplicado", bodyDuplicateEmail, dav.ErrDuplicateEmail},
		{"CPF inválido", bodyInvalidCPF, dav.ErrValidation},
	}

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()

			_, err := newClient(t, srv, nil).CreatePerson(context.Background(), validInput())
			if !errors.Is(err, tt.want) {
				t.Errorf("erro = %v; quero %v", err, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Retry
// ---------------------------------------------------------------------------

// O lookup é um GET: seguro de repetir, e a DAV oscila muito (medimos o mesmo
// GET entre 0,5s e 7,4s em homologação).
func TestFindPersonByCPF_RetentaEmFalhaTransitoria(t *testing.T) {
	tests := []struct {
		nome   string
		status int
		body   string
	}{
		{"500", http.StatusInternalServerError, `{"code":500,"message":"boom"}`},
		{"429", http.StatusTooManyRequests, `{"code":429,"message":"slow down"}`},
		{"504 do gateway", http.StatusGatewayTimeout, bodyGatewayTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			var attempts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if attempts.Add(1) == 1 {
					w.WriteHeader(tt.status)
					_, _ = io.WriteString(w, tt.body)
					return
				}
				_, _ = io.WriteString(w, `{"id":"dav-123","cpf":"94819089846"}`)
			}))
			defer srv.Close()

			p, err := newClient(t, srv, nil).FindPersonByCPF(context.Background(), "94819089846")
			if err != nil {
				t.Fatalf("erro inesperado após retry: %v", err)
			}
			if p == nil || p.ID != "dav-123" {
				t.Errorf("pessoa = %+v; quero dav-123", p)
			}
			if got := attempts.Load(); got != 2 {
				t.Errorf("%d tentativas; quero 2 (1 falha + 1 sucesso)", got)
			}
		})
	}
}

// CreatePerson NUNCA repete — nem em 5xx, nem em timeout.
//
// Descoberto no primeiro cadastro real pelo browser: o POST estourou o teto de
// 29s do gateway (504) mas TINHA criado a pessoa; a segunda tentativa levou 409
// "id already exists", e o cadastro foi reprovado com a pessoa existindo lá.
// Repetir um POST que pode ter funcionado transforma sucesso em conflito. A
// reconciliação é do model, que sabe consultar por CPF/id — ver ErrMaybeApplied.
func TestCreatePerson_NuncaRetenta(t *testing.T) {
	tests := []struct {
		nome   string
		status int
		body   string
	}{
		{"504 do gateway", http.StatusGatewayTimeout, bodyGatewayTimeout},
		{"500", http.StatusInternalServerError, `{"code":500}`},
	}

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			var attempts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()

			_, err := newClient(t, srv, nil).CreatePerson(context.Background(), validInput())
			if !errors.Is(err, dav.ErrMaybeApplied) {
				t.Errorf("erro = %v; quero ErrMaybeApplied (o resultado é DESCONHECIDO)", err)
			}
			if got := attempts.Load(); got != 1 {
				t.Errorf("%d tentativas; quero 1 — repetir POST vira 409", got)
			}
		})
	}
}

// 409 "id already exists." é a prova de que um POST anterior nosso funcionou.
// Formato capturado da HML — é uma QUARTA forma de erro deles, sem i18n nem detail.
func TestCreatePerson_409SignificaQueJaFoiAplicado(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"code":409,"message":"id already exists.","trace":"d42acd5"}`)
	}))
	defer srv.Close()

	_, err := newClient(t, srv, nil).CreatePerson(context.Background(), validInput())
	if !errors.Is(err, dav.ErrMaybeApplied) {
		t.Errorf("erro = %v; quero ErrMaybeApplied", err)
	}
}

// Repetir um 422 é inútil e, pior, gasta ~2s de latência da DAV por tentativa
// dentro de um cadastro que o usuário está esperando.
func TestCreatePerson_NaoRetentaEm4xx(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, bodyDuplicateCPF)
	}))
	defer srv.Close()

	_, _ = newClient(t, srv, nil).CreatePerson(context.Background(), validInput())
	if got := attempts.Load(); got != 1 {
		t.Errorf("%d tentativas em 422; quero 1 (4xx nunca se repete)", got)
	}
}

func TestFindPersonByCPF_EsgotaTentativasEDevolveErrUnavailable(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := newClient(t, srv, nil).FindPersonByCPF(context.Background(), "94819089846")
	if !errors.Is(err, dav.ErrUnavailable) {
		t.Errorf("erro = %v; quero ErrUnavailable", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("%d tentativas; quero 3 (MaxAttempts)", got)
	}
}

func TestCreatePerson_TimeoutViraErrMaybeApplied(t *testing.T) {
	// O handler precisa de saída PRÓPRIA: httptest.Server.Close() só retorna
	// quando todo handler pendente terminou, e um `<-r.Context().Done()` seco
	// travaria o teste para sempre — o cancelamento NÃO chega aqui quando o
	// http.Client.Timeout dispara (verificado: sem este time.After o teste pendura).
	// 500ms > os 50ms de timeout do client, então quem desiste primeiro é o client,
	// que é o que este teste mede.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
		}
	}))
	defer srv.Close()

	c, err := dav.New(dav.Config{
		BaseURL: srv.URL, APIKey: "k",
		Timeout: 50 * time.Millisecond, MaxAttempts: 2, BaseBackoff: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("dav.New: %v", err)
	}

	// Timeout num POST é o caso ambíguo por excelência: pode ter sido aplicado.
	if _, err := c.CreatePerson(context.Background(), validInput()); !errors.Is(err, dav.ErrMaybeApplied) {
		t.Errorf("erro = %v; quero ErrMaybeApplied", err)
	}
}

// Cancelamento do chamador precisa parar na hora, sem consumir as tentativas
// restantes: se o usuário fechou a aba, não há motivo para insistir.
func TestCreatePerson_RespeitaCancelamentoDoContexto(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newClient(t, srv, nil).CreatePerson(ctx, validInput())
	if err == nil {
		t.Fatal("contexto cancelado não devolveu erro")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("erro = %v; quero que case com context.Canceled", err)
	}
}

// ---------------------------------------------------------------------------
// LGPD
// ---------------------------------------------------------------------------

// O corpo do cadastro carrega CPF, nome, e-mail e endereço. O CLAUDE.md proíbe
// logar isso. Este teste é o que trava a regra — no espírito do
// TestHealthController_Readyz_DependencyDown, que trava vazamento de topologia.
func TestClient_NuncaLogaDadosPessoais(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, bodyDuplicateCPF) // erro: o caminho que mais tenta logar
	}))
	defer srv.Close()

	in := validInput()
	_, _ = newClient(t, srv, logger).CreatePerson(context.Background(), in)
	_, _ = newClient(t, srv, logger).FindPersonByCPF(context.Background(), in.CPF)

	logs := buf.String()
	proibido := map[string]string{
		"CPF":    in.CPF,
		"nome":   in.Name,
		"e-mail": in.Email,
		"chave":  "chave-de-teste",
	}
	for que, valor := range proibido {
		if strings.Contains(logs, valor) {
			t.Errorf("o log vazou %s (%q).\nLog:\n%s", que, valor, logs)
		}
	}
	// O trace da DAV é o que permite acionar o suporte deles — tem que estar lá.
	if !strings.Contains(logs, "955dc48") {
		t.Errorf("o log não trouxe o trace da DAV, necessário para o suporte.\nLog:\n%s", logs)
	}
}

func TestNew_ExigeBaseURLEChave(t *testing.T) {
	tests := []struct {
		nome string
		cfg  dav.Config
	}{
		{"sem base URL", dav.Config{APIKey: "k"}},
		{"sem chave", dav.Config{BaseURL: "https://x"}},
	}
	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			if _, err := dav.New(tt.cfg); err == nil {
				t.Error("New aceitou config incompleta; queria erro")
			}
		})
	}
}
