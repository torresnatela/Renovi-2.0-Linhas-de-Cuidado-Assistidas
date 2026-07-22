//go:build integration

// Package e2e é o teste de aceite do Slice 1: sobe a API REAL (o router de
// produção, com a fiação do cmd/api/main.go feita à mão) contra Postgres e
// MySQL reais (testcontainers) e uma DAV fake (httptest), e percorre o
// cenário-alvo inteiro por HTTP puro — admin monta a linha, paciente agenda,
// o motor barra, o admin renova, a auditoria conta a história.
//
// A API conecta no Postgres como o role RESTRITO renovi_app (appDSN): o slice
// inteiro precisa rodar sob os grants de produção, inclusive o append-only de
// journey_event. O superDSN fica só para montar cenário e para asserts diretos.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/adapters/dav"
	"github.com/renovisaude/renovi-care/internal/controllers"
	"github.com/renovisaude/renovi-care/internal/db"
	apihttp "github.com/renovisaude/renovi-care/internal/http"
	"github.com/renovisaude/renovi-care/internal/http/api"
	"github.com/renovisaude/renovi-care/internal/models"
	"github.com/renovisaude/renovi-care/internal/models/scheduling"
	"github.com/renovisaude/renovi-care/internal/testsupport"
)

const (
	adminToken = "test-admin-token"
	davAPIKey  = "e2e-dav-key"

	// Ids do deploy/mysql-legacy/init.sql (o mesmo mock do `make up`).
	anaID   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" // Psicologia
	brunoID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" // Psiquiatria
)

// env é o ambiente compartilhado por TODOS os testes do pacote: os containers
// levam dezenas de segundos para subir, então sobem UMA vez, no TestMain.
var env struct {
	superDSN string
	appDSN   string
	legacy   testsupport.LegacyMySQL
	baseURL  string        // http://.../api/v1
	super    *pgxpool.Pool // superusuário: montar cenário e asserts diretos
	loc      *time.Location
}

func TestMain(m *testing.M) {
	code, err := run(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// run existe para os defer rodarem (os.Exit no TestMain os pularia).
func run(m *testing.M) (int, error) {
	ctx := context.Background()

	superDSN, appDSN, stopPG, err := testsupport.StartPostgresSharedDSNs(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = stopPG() }()

	legacy, stopMySQL, err := testsupport.StartMySQLShared(ctx)
	if err != nil {
		return 0, fmt.Errorf("subir mysql legado: %w", err)
	}
	defer func() { _ = stopMySQL() }()

	davSrv := httptest.NewServer(newFakeDAV())
	defer davSrv.Close()

	// Só erro no log: o fluxo feliz do E2E atravessa caminhos que WARNam de
	// propósito (o cancel da DAV devolve 500 sempre), e o ruído esconderia uma
	// falha real.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// A API roda como o role RESTRITO (appDSN) — como em produção.
	pool, err := db.Connect(ctx, appDSN)
	if err != nil {
		return 0, fmt.Errorf("conectar como renovi_app: %w", err)
	}
	defer pool.Close()

	superPool, err := pgxpool.New(ctx, superDSN)
	if err != nil {
		return 0, fmt.Errorf("conectar como superusuário: %w", err)
	}
	defer superPool.Close()

	agendaClient, err := agenda.New(agenda.Config{DSN: legacy.AppDSN, Logger: logger})
	if err != nil {
		return 0, fmt.Errorf("abrir adapter da agenda: %w", err)
	}
	defer func() { _ = agendaClient.Close() }()

	davClient, err := dav.New(dav.Config{
		BaseURL:     davSrv.URL,
		APIKey:      davAPIKey,
		Timeout:     5 * time.Second,
		MaxAttempts: 2,
		BaseBackoff: 10 * time.Millisecond,
		Logger:      logger,
	})
	if err != nil {
		return 0, fmt.Errorf("abrir adapter da dav: %w", err)
	}

	// ------------------------------------------------------------------
	// Fiação REAL, espelhando cmd/api/main.go (que não dá para chamar: ele
	// carrega config de env e sobe um servidor de verdade).
	// ------------------------------------------------------------------
	const sessionTTL = 12 * time.Hour
	auth := &controllers.AuthController{
		Accounts:         models.NewAccountStore(pool, davClient, []byte("pepper-de-teste-e2e")),
		Sessions:         models.NewSessionStore(pool, sessionTTL),
		CookieSecure:     false,
		SessionTTL:       sessionTTL,
		RegisterDeadline: 40 * time.Second,
	}

	bookingStore := models.NewBookingStore(pool, agendaClient, davClient,
		scheduling.Policy{OpensBefore: 30 * time.Minute}, logger)
	schedulingCtrl := &controllers.SchedulingController{
		Bookings:     bookingStore,
		BookDeadline: 35 * time.Second,
	}

	careAdmin := &controllers.CareLineAdminController{
		Catalog:     models.NewCareLineStore(pool, agendaClient),
		Enrollments: models.NewEnrollmentStore(pool),
	}

	// A jornada agenda PELO booking: o MESMO BookingStore, como no main.
	journeyStore := models.NewJourneyStore(models.NewJourneyRepo(pool), bookingStore, 24*time.Hour, logger)
	journeyCtrl := &controllers.JourneyController{
		Journeys:     journeyStore,
		BookDeadline: 35 * time.Second,
	}
	internalCtrl := &controllers.InternalController{Journeys: journeyStore} // RENOVI_TEST_ENDPOINTS "ligado"

	router := apihttp.NewRouter(apihttp.Deps{
		Logger:  logger,
		Version: "e2e",
		Ready: func(ctx context.Context) error {
			if err := db.Ping(ctx, pool); err != nil {
				return err
			}
			return agendaClient.Ping(ctx)
		},
		Auth:            auth,
		Scheduling:      schedulingCtrl,
		CareAdmin:       careAdmin,
		Journey:         journeyCtrl,
		Internal:        internalCtrl,
		AdminToken:      adminToken,
		RegisterTimeout: 40 * time.Second,
		BookTimeout:     35 * time.Second,
	})
	srv := httptest.NewServer(router)
	defer srv.Close()

	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return 0, err
	}

	env.superDSN = superDSN
	env.appDSN = appDSN
	env.legacy = legacy
	env.baseURL = srv.URL + "/api/v1"
	env.super = superPool
	env.loc = loc

	return m.Run(), nil
}

// ---------------------------------------------------------------------------
// DAV fake
// ---------------------------------------------------------------------------

// newFakeDAV cobre exatamente o que o fluxo usa, com os shapes REAIS mapeados
// em adapters/dav (docs/DAV-API-NOTAS.md):
//
//   - GET /person/cpf/{cpf} → 204 (não encontrado é 204, nunca 404)
//   - POST /person          → 201 ecoando o id que NÓS mandamos (achado #4)
//   - GET /person/{id}      → 204 (a sonda; o fluxo feliz não chega nela)
//   - POST /appointment     → 201 com id DELES e participants[] com url
//   - PUT /appointment/{id}/cancel → 500 SEMPRE (achado #20) — é isso que
//     prova a tolerância best-effort do cancelamento do slice.
func newFakeDAV() http.Handler {
	writeJSON := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	withKey := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-api-key") != davAPIKey {
				writeJSON(w, http.StatusUnauthorized, map[string]any{
					"code": 401, "message": "Unauthorized", "trace": "e2e-auth",
				})
				return
			}
			next(w, r)
		}
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /person/cpf/{cpf}", withKey(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /person/{id}", withKey(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /person", withKey(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"code": 400, "message": "Person invalid", "trace": "e2e-person",
			})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": body.ID})
	}))

	mux.HandleFunc("POST /appointment", withKey(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Participants []struct {
				ID   string `json:"id"`
				Role string `json:"role"`
			} `json:"participants"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Participants) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"code": 400, "message": "Appointment invalid", "trace": "e2e-appt",
			})
			return
		}
		id := uuid.NewString()
		participants := make([]map[string]string, 0, len(body.Participants))
		for _, p := range body.Participants {
			participants = append(participants, map[string]string{
				"id":  p.ID,
				"url": "https://dav.example.test/a/" + id + "/" + p.Role,
			})
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id, "participants": participants})
	}))

	mux.HandleFunc("PUT /appointment/{id}/cancel", withKey(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"code": 500, "message": "Internal server error", "trace": "e2e-cancel-trace",
		})
	}))

	return mux
}

// ---------------------------------------------------------------------------
// Seed compartilhado (uma vez por pacote)
// ---------------------------------------------------------------------------

var (
	seedOnce   sync.Once
	anaSlots   map[int][]testsupport.SeededSlot
	brunoSlots map[int][]testsupport.SeededSlot
	// Cenário C (linha semanal): offsets DISJUNTOS dos cenários A/B — os três
	// cenários dividem o mesmo legado sem se pisar.
	anaSlotsC   map[int][]testsupport.SeededSlot
	brunoSlotsC map[int][]testsupport.SeededSlot
)

// seedSlots semeia os horários futuros dos cenários UMA vez. Os offsets
// realizam a aritmética de cada cenário — A/B: QUOTA 4/mês, MIN_INTERVAL 7d,
// MAX_ADVANCE 30d, vigência de 30d; C: QUOTA 1/semana (psico +4/+11/+18/+25,
// com +12/+26 para os bloqueios e +32 para além da vigência) e QUOTA 1/mês
// (psiq +6, com +13/+27 para remarcar). Mudar um número aqui muda vereditos lá.
func seedSlots(t *testing.T) {
	t.Helper()
	seedOnce.Do(func() {
		anaSlots = testsupport.SeedFutureSlots(t, env.legacy.RootDSN, anaID, []int{2, 9, 10, 16, 23, 30, 44})
		brunoSlots = testsupport.SeedFutureSlots(t, env.legacy.RootDSN, brunoID, []int{5, 37})
		anaSlotsC = testsupport.SeedFutureSlots(t, env.legacy.RootDSN, anaID, []int{4, 11, 12, 18, 25, 26, 32})
		brunoSlotsC = testsupport.SeedFutureSlots(t, env.legacy.RootDSN, brunoID, []int{6, 13, 27})
	})
	require.NotEmpty(t, anaSlots, "seed dos slots falhou num teste anterior")
	require.NotEmpty(t, brunoSlots, "seed dos slots falhou num teste anterior")
	require.NotEmpty(t, anaSlotsC, "seed dos slots do cenário C falhou num teste anterior")
	require.NotEmpty(t, brunoSlotsC, "seed dos slots do cenário C falhou num teste anterior")
}

// ---------------------------------------------------------------------------
// Helpers HTTP
// ---------------------------------------------------------------------------

// steps devolve um runner de subtests SEQUENCIAIS: se um passo falha, os
// seguintes não fazem sentido (o estado é cumulativo) e a sequência aborta.
func steps(t *testing.T) func(name string, fn func(t *testing.T)) {
	return func(name string, fn func(t *testing.T)) {
		t.Helper()
		if !t.Run(name, fn) {
			t.Fatalf("sequência interrompida: o passo %s falhou", name)
		}
	}
}

// newPatientClient cria um client com cookie jar próprio — a sessão do paciente
// vive nele, como num browser.
func newPatientClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return &http.Client{Jar: jar, Timeout: 60 * time.Second}
}

// plainClient é para rotas sem sessão (admin, internal, tentativas sem token).
var plainClient = &http.Client{Timeout: 60 * time.Second}

// doReq executa uma chamada JSON e devolve status + corpo cru. As asserções
// ficam com o chamador — cada passo do cenário afirma status E corpo.
func doReq(t *testing.T, c *http.Client, method, path string, headers map[string]string, body any) (int, []byte) {
	t.Helper()

	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err, "serializar corpo")
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, env.baseURL+path, rd)
	require.NoError(t, err, "montar request")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := c.Do(req)
	require.NoError(t, err, "%s %s", method, path)
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err, "ler resposta de %s %s", method, path)
	return res.StatusCode, raw
}

// adminDo chama uma rota /admin com o token correto.
func adminDo(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	return doReq(t, plainClient, method, path, map[string]string{"X-Admin-Token": adminToken}, body)
}

// decodeAs decodifica o corpo nos tipos GERADOS do contrato (internal/http/api)
// — o teste fala a língua do openapi.yaml, não structs paralelas.
func decodeAs[T any](t *testing.T, raw []byte) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(raw, &v), "decodificar corpo: %s", raw)
	return v
}

func problemOf(t *testing.T, raw []byte) api.Problem {
	t.Helper()
	return decodeAs[api.Problem](t, raw)
}

func reasonCode(p api.Problem) string {
	if p.Reason == nil {
		return ""
	}
	return p.Reason.Code
}

func blocksOf(t *testing.T, p api.Problem) []api.EligibilityBlock {
	t.Helper()
	require.NotNil(t, p.Blocks, "o problem deveria trazer blocks[]")
	return *p.Blocks
}

// findBlock acha o bloqueio de uma regra na lista (nil se ausente).
func findBlock(blocks []api.EligibilityBlock, ruleType string) *api.EligibilityBlock {
	for i := range blocks {
		if string(blocks[i].RuleType) == ruleType {
			return &blocks[i]
		}
	}
	return nil
}

// requireBlock exige o bloqueio de uma regra e o devolve.
func requireBlock(t *testing.T, blocks []api.EligibilityBlock, ruleType string) api.EligibilityBlock {
	t.Helper()
	b := findBlock(blocks, ruleType)
	require.NotNil(t, b, "esperava um block %s; blocks: %+v", ruleType, blocks)
	return *b
}

// requireInstant compara instantes na resolução do banco (microssegundo). O
// timestamptz do Postgres NÃO representa sub-microssegundo, então um instante que
// passou pela coluna vem truncado a micros, enquanto o mesmo instante num payload
// de evento (RFC3339Nano) pode trazer nanossegundos. No Linux o time.Now() tem
// resolução de nanossegundo, e comparar por igualdade exata falharia por uma
// diferença que o próprio banco não distingue. Truncar a micros é a tolerância
// ZERO correta para instantes ancorados no banco.
func requireInstant(t *testing.T, want, got time.Time, msg string) {
	t.Helper()
	require.Truef(t, want.Truncate(time.Microsecond).Equal(got.Truncate(time.Microsecond)),
		"%s: esperava %s, veio %s", msg, want, got)
}

func idemKey(t *testing.T) string {
	t.Helper()
	return uuid.NewString()
}
