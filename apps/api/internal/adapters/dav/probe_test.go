//go:build davprobe

// Sondagem da API Doutor ao Vivo (DAV) — NÃO é teste de unidade.
//
// Roda só sob demanda (`make dav-probe`) e NUNCA no CI: bate na API real de
// homologação e CRIA pessoas de teste lá.
//
// Por que existe: o spec publicado da DAV se contradiz. Ele declara
// `required: [name, birth_date, status]` — mas todo exemplo omite `status` e
// manda `cpf`, que o spec diz ser opcional. E o lookup por CPF responde 204,
// não 404. Escrever o adapter só a partir do spec produziria um cliente errado
// em três pontos. Esta bateria descobre o comportamento REAL antes disso.
//
// Ela REPORTA em vez de asserir: um probe que aborta no primeiro achado
// inesperado não sonda nada. Só falha no que impede a sondagem (host fora, chave
// inválida). O relatório sai em docs/DAV-API-NOTAS.md.
//
// Higiene: CPFs sintéticos com DV válido, nome prefixado "RENOVI PROBE",
// e-mail @example.com (domínio reservado pela RFC 2606, não entrega), e todas as
// pessoas criadas são removidas no fim (DELETE ?soft=true).
package dav

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Caminhos relativos ao diretório do pacote, que é a CWD do `go test`.
// São 5 níveis até a raiz: apps/api/internal/adapters/dav.
const (
	probeNamePrefix = "RENOVI PROBE"
	repoRoot        = "../../../../../"
	reportPath      = repoRoot + "docs/DAV-API-NOTAS.md"
	dotEnvPath      = repoRoot + ".env"
)

// loadDotEnv popula o ambiente a partir do .env da raiz, sem passar pelo shell.
//
// Sourcing (`set -a; . ./.env`) não serve: o .env tem um DSN de MySQL com
// parênteses (`...@tcp(localhost:3306)/...`) que o sh trata como sintaxe e
// aborta o arquivo inteiro. Passar a chave por `env K=V go test` também não —
// argv aparece no `ps aux`.
//
// Variável já exportada vence o .env (precedência 12-factor).
func loadDotEnv(t *testing.T) {
	t.Helper()

	raw, err := os.ReadFile(dotEnvPath)
	if err != nil {
		return // sem .env é legítimo: as vars podem vir exportadas
	}

	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		val = strings.TrimSpace(val)
		if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = val[1 : len(val)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			t.Setenv(key, val) // t.Setenv restaura o ambiente no fim do teste
		}
	}
}

// ---------------------------------------------------------------------------
// Infra da sondagem
// ---------------------------------------------------------------------------

type probeClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// newProbeClient lê as credenciais do ambiente. Sem elas, pula com instrução
// acionável em vez de falhar — quem roda `make test` não deve tropeçar nisso.
func newProbeClient(t *testing.T) *probeClient {
	t.Helper()
	loadDotEnv(t)

	base := strings.TrimSpace(os.Getenv("RENOVI_DAV_BASE_URL"))
	key := strings.TrimSpace(os.Getenv("RENOVI_DAV_API_KEY"))
	if base == "" || key == "" {
		t.Skip("sondagem pulada: defina RENOVI_DAV_BASE_URL e RENOVI_DAV_API_KEY " +
			"(as linhas já existem comentadas no .env). Ex.: " +
			"RENOVI_DAV_BASE_URL=https://api.v2.hom.doutoraovivo.com.br")
	}

	c := &probeClient{
		baseURL: strings.TrimRight(base, "/"),
		apiKey:  key,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
	c.preflight(t)
	return c
}

// preflight confere que a chave é aceita ANTES de sondar qualquer coisa.
//
// Sem isto, um 401 global é lido por cada sondagem como se fosse resposta de
// domínio, e o relatório sai cheio de vereditos confiantes e errados ("`status`
// é obrigatório", "a DAV valida o DV") — todos deduzidos de um "Unauthorized".
// Um probe que mente é pior que probe nenhum.
func (c *probeClient) preflight(t *testing.T) {
	t.Helper()

	r := c.do(t, http.MethodGet, "/person/cpf/"+randomCPF(), nil)
	if r.status == http.StatusUnauthorized || r.status == http.StatusForbidden {
		t.Fatalf("a DAV recusou a credencial (HTTP %d) — sondagem abortada para não "+
			"produzir achados falsos.\nResposta: %s\nConfira RENOVI_DAV_API_KEY no .env: "+
			"a chave precisa ser de HOMOLOGAÇÃO e estar ativa.", r.status, r.pretty(400))
	}
}

// probeResponse é o tipo do PROBE. Nome distinto do `response` do client.go de
// propósito: os dois vivem no mesmo pacote e colidiriam sob a tag davprobe.
type probeResponse struct {
	status  int
	body    []byte
	elapsed time.Duration
}

// outcome classifica uma resposta em "a DAV decidiu" vs "não deu para saber".
//
// Existe porque a sondagem já se enganou duas vezes lendo falha de transporte
// como resposta de domínio: um 401 global virou "`status` é obrigatório", e um
// 504 do gateway virou a mesma conclusão. Só 2xx e 4xx são opinião da DAV; 5xx
// e timeout são silêncio, e silêncio não é veredito.
type outcome int

const (
	accepted     outcome = iota // 2xx — a DAV aceitou
	rejected                    // 4xx — a DAV recusou (opinião dela sobre o payload)
	inconclusive                // 5xx/timeout — não sabemos
)

func (r probeResponse) outcome() outcome {
	switch {
	case r.status >= 200 && r.status < 300:
		return accepted
	case r.status >= 400 && r.status < 500:
		return rejected
	default:
		return inconclusive
	}
}

// inconclusiveNote explica, no relatório, por que não há veredito.
func (r probeResponse) inconclusiveNote() string {
	return fmt.Sprintf("**Inconclusivo** — HTTP %d não é resposta de domínio, é falha de "+
		"transporte (%s). Rode `make dav-probe` de novo.", r.status, r.elapsed.Round(time.Millisecond))
}

// pretty devolve o corpo formatado para o relatório. O corpo da DAV em erro é
// um DavHttpExceptionDto {code, message, trace, i18n?, detail?} — sem PII.
func (r probeResponse) pretty(limit int) string {
	s := strings.TrimSpace(string(r.body))
	if s == "" {
		return "(corpo vazio)"
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, r.body, "", "  "); err == nil {
		s = buf.String()
	}
	if len(s) > limit {
		s = s[:limit] + "\n… (truncado)"
	}
	return s
}

func (c *probeClient) do(t *testing.T, method, path string, body any) probeResponse {
	t.Helper()

	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("serializar corpo: %v", err)
		}
		rdr = bytes.NewReader(raw)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		t.Fatalf("montar request: %v", err)
	}
	req.Header.Set("x-api-key", c.apiKey) // nunca logado
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("chamar %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ler corpo de %s %s: %v", method, path, err)
	}
	return probeResponse{status: resp.StatusCode, body: raw, elapsed: elapsed}
}

// createdIDs guarda o que criamos para limpar no fim.
var createdIDs sync.Map

func (c *probeClient) createPerson(t *testing.T, payload map[string]any) probeResponse {
	t.Helper()
	r := c.do(t, http.MethodPost, "/person", payload)
	if r.status >= 200 && r.status < 300 {
		var out struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(r.body, &out) == nil && out.ID != "" {
			createdIDs.Store(out.ID, struct{}{})
		}
	}
	return r
}

// ---------------------------------------------------------------------------
// Dados sintéticos
// ---------------------------------------------------------------------------

// randomCPF gera um CPF sintético com dígitos verificadores válidos. Não é um
// CPF de pessoa real — os 9 dígitos-base são aleatórios.
func randomCPF() string {
	var d [11]int
	for {
		for i := 0; i < 9; i++ {
			d[i] = rand.IntN(10)
		}
		if !allSame(d[:9]) {
			break
		}
	}
	d[9] = checkDigit(d[:9], 10)
	d[10] = checkDigit(d[:10], 11)

	var sb strings.Builder
	for _, n := range d {
		fmt.Fprintf(&sb, "%d", n)
	}
	return sb.String()
}

// checkDigit implementa o DV do CPF: soma ponderada decrescente a partir de
// `weight`, resto de (soma*10) por 11, com 10 colapsando em 0.
func checkDigit(digits []int, weight int) int {
	sum := 0
	for i, n := range digits {
		sum += n * (weight - i)
	}
	if r := (sum * 10) % 11; r < 10 {
		return r
	}
	return 0
}

func allSame(d []int) bool {
	for _, n := range d[1:] {
		if n != d[0] {
			return false
		}
	}
	return true
}

func probeEmail() string {
	return fmt.Sprintf("renovi-probe+%s@example.com", uuid.NewString()[:8])
}

func minimalPerson() map[string]any {
	return map[string]any{
		"name":       probeNamePrefix + " Minimo",
		"birth_date": "1990-05-14",
		"cpf":        randomCPF(),
		"status":     true,
	}
}

// ---------------------------------------------------------------------------
// Relatório
// ---------------------------------------------------------------------------

type finding struct {
	order    int
	question string
	verdict  string
	evidence string
}

var (
	findingsMu sync.Mutex
	findings   []finding
)

func record(order int, question, verdict, evidence string) {
	findingsMu.Lock()
	defer findingsMu.Unlock()
	findings = append(findings, finding{order, question, verdict, evidence})
}

func writeReport(t *testing.T, baseURL string) {
	t.Helper()

	findingsMu.Lock()
	defer findingsMu.Unlock()
	sort.Slice(findings, func(i, j int) bool { return findings[i].order < findings[j].order })

	var b strings.Builder
	b.WriteString("# Notas da API Doutor ao Vivo (DAV) — achados da sondagem\n\n")
	b.WriteString("> **Gerado por `make dav-probe`** (`apps/api/internal/adapters/dav/probe_test.go`).\n")
	b.WriteString("> Não edite à mão: rode a bateria de novo.\n\n")
	fmt.Fprintf(&b, "- Ambiente sondado: `%s`\n", baseURL)
	fmt.Fprintf(&b, "- Data: %s\n\n", time.Now().Format("2006-01-02 15:04:05 -07:00"))
	b.WriteString("O spec publicado (`/person/_/api-docs`) se contradiz em pontos que mudam o\n")
	b.WriteString("adapter. Esta é a fonte da verdade sobre o comportamento REAL.\n\n")
	b.WriteString("## Resumo\n\n| # | Pergunta | Veredito |\n|---|---|---|\n")
	for _, f := range findings {
		fmt.Fprintf(&b, "| %d | %s | %s |\n", f.order, f.question, f.verdict)
	}
	b.WriteString("\n## Evidências\n\n")
	for _, f := range findings {
		fmt.Fprintf(&b, "### %d. %s\n\n**Veredito:** %s\n\n%s\n\n", f.order, f.question, f.verdict, f.evidence)
	}

	path, err := filepath.Abs(reportPath)
	if err != nil {
		t.Fatalf("resolver caminho do relatório: %v", err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("escrever relatório: %v", err)
	}
	t.Logf("relatório escrito em %s", path)
}

func codeBlock(lang, s string) string {
	return "```" + lang + "\n" + s + "\n```"
}

// ---------------------------------------------------------------------------
// A bateria
// ---------------------------------------------------------------------------

func TestDAVProbe(t *testing.T) {
	c := newProbeClient(t)

	t.Cleanup(func() {
		cleanup(t, c)
		writeReport(t, c.baseURL)
	})

	t.Run("01_lookup_cpf_inexistente", func(t *testing.T) { probeLookupMissing(t, c) })
	t.Run("02_status_obrigatorio", func(t *testing.T) { probeStatusRequired(t, c) })
	t.Run("03_cpf_opcional", func(t *testing.T) { probeCPFOptional(t, c) })
	t.Run("04_id_do_integrador", func(t *testing.T) { probeIntegratorID(t, c) })
	t.Run("05_cpf_duplicado", func(t *testing.T) { probeDuplicateCPF(t, c) })
	t.Run("06_email_duplicado", func(t *testing.T) { probeDuplicateEmail(t, c) })
	t.Run("07_cpf_dv_invalido", func(t *testing.T) { probeInvalidCPF(t, c) })
	t.Run("08_endereco_cidade", func(t *testing.T) { probeAddressCity(t, c) })
	t.Run("09_superficie_pii", func(t *testing.T) { probePIISurface(t, c) })
	t.Run("10_latencia", func(t *testing.T) { probeLatency(t, c) })
	t.Run("11_id_repetido", func(t *testing.T) { probeDuplicateID(t, c) })
}

// 11. O que acontece ao repetir um POST com o MESMO id?
//
// Esta sondagem nasceu de um bug em produção de HML, não de uma hipótese: um
// POST estourou o teto de 29s do gateway (504) mas TINHA criado a pessoa; o
// retry levou 409 e o cadastro foi reprovado com a pessoa existindo lá. É o
// caminho que todo retry de POST percorre, e nenhuma das 10 primeiras o cobria.
func probeDuplicateID(t *testing.T, c *probeClient) {
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("gerar uuid v7: %v", err)
	}

	first := minimalPerson()
	first["id"] = id.String()
	first["name"] = probeNamePrefix + " Id Repetido"
	r1 := c.createPerson(t, first)
	if r1.outcome() != accepted {
		record(11, "Repetir o POST com o MESMO id devolve o quê?",
			fmt.Sprintf("**Inconclusivo** — a criação-base falhou com HTTP %d.", r1.status),
			codeBlock("json", r1.pretty(600)))
		return
	}

	r2 := c.createPerson(t, first) // idêntico: é o que um retry cego faz

	verdict := fmt.Sprintf("HTTP **%d** — ", r2.status)
	switch {
	case r2.outcome() == accepted:
		verdict += "a DAV é **idempotente** por id. Retry de POST seria seguro."
	case r2.status == http.StatusConflict:
		verdict += "**409 `id already exists`** — prova de que um POST nosso anterior pegou. " +
			"NÃO é falha: mapear para `ErrMaybeApplied` e **sondar** com `GET /person/{nosso-id}` " +
			"antes de concluir. É por isso que `CreatePerson` nunca repete."
	default:
		verdict += "resposta inesperada — revisar o mapeamento de erro do adapter."
	}

	record(11, "Repetir o POST com o MESMO id devolve o quê?", verdict,
		"O cenário do retry cego: dois `POST /person` idênticos, mesmo `id` (`"+id.String()+"`).\n\n"+
			"Primeira: HTTP "+fmt.Sprint(r1.status)+"\n\nSegunda:\n\n"+codeBlock("json", r2.pretty(600))+
			"\n\n> Este achado veio de um cadastro real que falhou, não da lista original de\n"+
			"> perguntas. O 504 do gateway mente: ele diz que falhou depois de ter criado.")
}

// 1. O lookup por CPF responde 204 (como o spec diz) ou 404?
// Define o contrato de FindPersonByCPF: 204 -> (nil, nil), não erro.
func probeLookupMissing(t *testing.T, c *probeClient) {
	r := c.do(t, http.MethodGet, "/person/cpf/"+randomCPF(), nil)

	verdict := fmt.Sprintf("HTTP **%d**", r.status)
	switch r.status {
	case http.StatusNoContent:
		verdict += " — confirma o spec. `FindPersonByCPF` deve mapear 204 → não encontrado."
	case http.StatusNotFound:
		verdict += " — **diverge do spec** (que diz 204). Mapear 404 → não encontrado."
	default:
		verdict += " — **inesperado**. Revisar antes de escrever o adapter."
	}
	record(1, "Lookup de CPF inexistente devolve o quê?", verdict,
		"Requisição: `GET /person/cpf/{cpf-sintetico-inexistente}`\n\nResposta:\n\n"+
			codeBlock("json", r.pretty(600)))
}

// 2. `status` é required no spec mas some de todos os exemplos. Qual vale?
func probeStatusRequired(t *testing.T, c *probeClient) {
	p := minimalPerson()
	delete(p, "status")
	r := c.createPerson(t, p)

	var verdict string
	switch r.outcome() {
	case accepted:
		verdict = fmt.Sprintf("HTTP **%d** — `status` é **opcional na prática** "+
			"(os exemplos estão certos; o `required` do spec mente). Mandamos sempre, mesmo assim.", r.status)
	case rejected:
		verdict = fmt.Sprintf("HTTP **%d** — `status` é **realmente obrigatório** "+
			"(o `required` do spec está certo; os exemplos mentem). Sempre enviar.", r.status)
	default:
		verdict = r.inconclusiveNote()
	}
	record(2, "`status` é mesmo obrigatório no POST /person?", verdict,
		"Requisição: `POST /person` **sem** `status`:\n\n"+codeBlock("json", jsonOf(p))+
			"\n\nResposta:\n\n"+codeBlock("json", r.pretty(800)))
}

// 3. O spec diz que `cpf` é opcional. Se for mesmo, é um furo: nosso vínculo
// inteiro depende do CPF como chave de identidade.
func probeCPFOptional(t *testing.T, c *probeClient) {
	p := minimalPerson()
	delete(p, "cpf")
	p["name"] = probeNamePrefix + " Sem CPF"
	r := c.createPerson(t, p)

	var verdict string
	switch r.outcome() {
	case accepted:
		verdict = fmt.Sprintf("HTTP **%d** — aceita pessoa **sem CPF**. ⚠️ A DAV pode conter "+
			"pessoas que nosso lookup por CPF nunca acha.", r.status)
	case rejected:
		verdict = fmt.Sprintf("HTTP **%d** — `cpf` é obrigatório na prática (o spec diz que é "+
			"opcional). Bom: o lookup por CPF cobre toda a base.", r.status)
	default:
		verdict = r.inconclusiveNote()
	}
	record(3, "`cpf` é opcional no POST /person, como diz o spec?", verdict,
		"Requisição: `POST /person` **sem** `cpf`:\n\n"+codeBlock("json", jsonOf(p))+
			"\n\nResposta:\n\n"+codeBlock("json", r.pretty(800)))
}

// 4. Se a DAV aceita nosso UUIDv7 como id, o POST vira sondável: em timeout,
// GET /person/{nosso-id} diz se a criação pegou. É a base da idempotência.
func probeIntegratorID(t *testing.T, c *probeClient) {
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("gerar uuid v7: %v", err)
	}
	p := minimalPerson()
	p["id"] = id.String()
	p["name"] = probeNamePrefix + " Id Proprio"
	r := c.createPerson(t, p)

	evidence := "Requisição: `POST /person` com `id` = UUIDv7 nosso (`" + id.String() + "`):\n\n" +
		codeBlock("json", jsonOf(p)) + "\n\nResposta:\n\n" + codeBlock("json", r.pretty(600))

	verdict := fmt.Sprintf("HTTP **%d** — ", r.status)
	if r.status < 200 || r.status >= 300 {
		verdict += "**rejeitado**. Sem id do integrador não há sonda de idempotência: " +
			"a reconciliação passa a depender só do lookup por CPF."
		record(4, "A DAV aceita um `id` gerado por nós (UUIDv7)?", verdict, evidence)
		return
	}

	// Confirma o round-trip: o id que mandamos é mesmo o id de lá?
	g := c.do(t, http.MethodGet, "/person/"+id.String(), nil)
	evidence += "\n\nConfirmação: `GET /person/" + id.String() + "` → **" + fmt.Sprint(g.status) + "**\n\n" +
		codeBlock("json", g.pretty(800))

	if g.status == http.StatusOK {
		verdict += "**aceito e confirmado** pelo GET. `POST /person` fica sondável → idempotência via `GET /person/{nosso-id}`."
	} else {
		verdict += fmt.Sprintf("aceito no POST, mas `GET /person/{id}` devolveu **%d** — o id pode ter sido reescrito. Investigar.", g.status)
	}
	record(4, "A DAV aceita um `id` gerado por nós (UUIDv7)?", verdict, evidence)
}

// 5. O erro de CPF duplicado é o caminho quente do item 2 do escopo — define o
// mapeamento de erro do adapter.
func probeDuplicateCPF(t *testing.T, c *probeClient) {
	cpf := randomCPF()

	// E-mails explícitos e DIFERENTES são essenciais aqui. Sem e-mail, a DAV
	// sintetiza <cpf>@dav.med.br — logo, mesmo CPF geraria o mesmo e-mail, e a
	// colisão de e-mail mascararia a de CPF. A primeira versão desta sondagem
	// caiu nessa armadilha e "provou" algo que não tinha testado.
	first := minimalPerson()
	first["cpf"] = cpf
	first["email"] = probeEmail()
	first["name"] = probeNamePrefix + " Dup CPF A"
	r1 := c.createPerson(t, first)
	if r1.outcome() != accepted {
		record(5, "A DAV rejeita CPF duplicado?",
			fmt.Sprintf("**Inconclusivo** — a criação-base falhou com HTTP %d.", r1.status),
			codeBlock("json", r1.pretty(600)))
		return
	}

	second := minimalPerson()
	second["cpf"] = cpf
	second["email"] = probeEmail() // e-mail DIFERENTE: isola o CPF como variável
	second["name"] = probeNamePrefix + " Dup CPF B"
	r2 := c.createPerson(t, second)

	var verdict string
	switch r2.outcome() {
	case accepted:
		verdict = fmt.Sprintf("HTTP **%d** — 🚨 **a DAV ACEITA CPF duplicado.** Grave: dois cadastros "+
			"com o mesmo CPF tornam `GET /person/cpf/{cpf}` ambíguo (qual dos dois volta?), e o "+
			"vínculo do nosso cadastro deixa de ser determinístico. Nossa unicidade de CPF em "+
			"`patient_identity` passa a ser a única barreira.", r2.status)
	case rejected:
		verdict = fmt.Sprintf("HTTP **%d** — a DAV rejeita CPF duplicado. Mapear para `ErrDuplicate` "+
			"(nunca retriável). O lookup por CPF é determinístico.", r2.status)
	default:
		verdict = r2.inconclusiveNote()
	}

	record(5, "A DAV rejeita CPF duplicado?", verdict,
		"Duas pessoas, mesmo CPF (`"+cpf+"`), **e-mails diferentes** — para que uma eventual\n"+
			"recusa seja pelo CPF, e não pelo e-mail sintetizado.\n\n"+
			"Primeira: HTTP "+fmt.Sprint(r1.status)+"\n\nSegunda:\n\n"+codeBlock("json", r2.pretty(800)))
}

// 6. O spec diz que e-mail precisa ser único na base da DAV. Se for, colaborador
// que compartilha e-mail (comum) não consegue se cadastrar — risco de produto.
func probeDuplicateEmail(t *testing.T, c *probeClient) {
	email := probeEmail()

	first := minimalPerson()
	first["email"] = email
	first["name"] = probeNamePrefix + " Dup Mail A"
	r1 := c.createPerson(t, first)
	if r1.status < 200 || r1.status >= 300 {
		record(6, "E-mail duplicado é rejeitado?",
			fmt.Sprintf("**Inconclusivo** — a criação-base falhou com HTTP %d.", r1.status),
			codeBlock("json", r1.pretty(600)))
		return
	}

	second := minimalPerson() // CPF diferente, mesmo e-mail
	second["email"] = email
	second["name"] = probeNamePrefix + " Dup Mail B"
	r2 := c.createPerson(t, second)

	var verdict string
	switch r2.outcome() {
	case accepted:
		verdict = fmt.Sprintf("HTTP **%d** — e-mail **não** precisa ser único. "+
			"Some o risco do e-mail compartilhado.", r2.status)
	case rejected:
		verdict = fmt.Sprintf("HTTP **%d** — e-mail **é único na DAV**. ⚠️ Risco de produto: duas "+
			"pessoas com o mesmo e-mail (casal, e-mail compartilhado) — a segunda não se cadastra. "+
			"Precisa de mensagem própria na UI.", r2.status)
	default:
		verdict = r2.inconclusiveNote()
	}
	record(6, "E-mail duplicado é rejeitado?", verdict,
		"Duas pessoas, **CPFs diferentes**, mesmo e-mail (`"+email+"`).\n\n"+
			"Primeira: HTTP "+fmt.Sprint(r1.status)+"\n\nSegunda:\n\n"+codeBlock("json", r2.pretty(800)))
}

// 7. Se a DAV não valida o DV, nossa validação é a única barreira contra CPF
// inventado — o que reforça o pacote puro `models/cpf`.
func probeInvalidCPF(t *testing.T, c *probeClient) {
	p := minimalPerson()
	p["cpf"] = "12345678900" // DV inválido de propósito
	p["name"] = probeNamePrefix + " CPF Invalido"
	r := c.createPerson(t, p)

	var verdict string
	switch r.outcome() {
	case accepted:
		verdict = fmt.Sprintf("HTTP **%d** — a DAV **não valida** o dígito verificador. "+
			"Nossa validação em `models/cpf` é a única barreira contra CPF inventado.", r.status)
	case rejected:
		verdict = fmt.Sprintf("HTTP **%d** — a DAV valida o DV. Nossa validação continua valendo: "+
			"falha rápido, sem gastar um round-trip lento.", r.status)
	default:
		verdict = r.inconclusiveNote()
	}
	record(7, "A DAV valida o dígito verificador do CPF?", verdict,
		"Requisição: `POST /person` com `cpf` = `12345678900` (DV inválido).\n\nResposta:\n\n"+
			codeBlock("json", r.pretty(800)))
}

// 8. O spec diz que `city` aceita código IBGE OU nome. Se o nome funcionar,
// economizamos uma tabela/lookup de IBGE no cadastro.
func probeAddressCity(t *testing.T, c *probeClient) {
	addr := func(city any) map[string]any {
		return map[string]any{
			"country":      "BR",
			"zip_code":     "06472000",
			"street":       "Avenida Copacabana",
			"number":       "238",
			"neighborhood": "Dezoito do Forte",
			"city":         city,
			"state":        "SP",
		}
	}

	byName := minimalPerson()
	byName["name"] = probeNamePrefix + " Cidade Nome"
	byName["address"] = addr("Barueri")
	rName := c.createPerson(t, byName)

	byIBGE := minimalPerson()
	byIBGE["name"] = probeNamePrefix + " Cidade IBGE"
	byIBGE["address"] = addr(3505708)
	rIBGE := c.createPerson(t, byIBGE)

	nameOK := rName.status >= 200 && rName.status < 300
	ibgeOK := rIBGE.status >= 200 && rIBGE.status < 300

	var verdict string
	switch {
	case nameOK && ibgeOK:
		verdict = "**Ambos funcionam.** Usar o **nome** da cidade — dispensa lookup de código IBGE no cadastro."
	case nameOK:
		verdict = "Só o **nome** funciona. Enviar nome."
	case ibgeOK:
		verdict = "Só o **código IBGE** funciona. ⚠️ O cadastro precisa resolver CEP → IBGE (ViaCEP devolve `ibge`)."
	default:
		verdict = "**Nenhum dos dois funcionou** — investigar o formato de `address` antes da Fase 4."
	}
	record(8, "`address.city` aceita nome do município ou exige código IBGE?", verdict,
		fmt.Sprintf("Por nome (`\"Barueri\"`): HTTP **%d**\n\n%s\n\nPor código IBGE (`3505708`): HTTP **%d**\n\n%s",
			rName.status, codeBlock("json", rName.pretty(500)),
			rIBGE.status, codeBlock("json", rIBGE.pretty(500))))
}

// 9. Confirma a superfície de PII que o lookup por CPF devolve. É a base da
// regra "nunca ecoar nada da DAV na resposta do /auth/register".
func probePIISurface(t *testing.T, c *probeClient) {
	cpf := randomCPF()
	p := minimalPerson()
	p["cpf"] = cpf
	p["name"] = probeNamePrefix + " Pii"
	p["email"] = probeEmail()
	p["cell_phone"] = "11912345678"
	p["mother_name"] = "Mae Da Probe"
	p["address"] = map[string]any{
		"country": "BR", "zip_code": "06472000", "street": "Avenida Copacabana",
		"number": "238", "neighborhood": "Dezoito do Forte", "city": "Barueri", "state": "SP",
	}
	r1 := c.createPerson(t, p)
	if r1.status < 200 || r1.status >= 300 {
		record(9, "Que PII o `GET /person/cpf/{cpf}` expõe?",
			fmt.Sprintf("**Inconclusivo** — a criação-base falhou com HTTP %d.", r1.status),
			codeBlock("json", r1.pretty(600)))
		return
	}

	g := c.do(t, http.MethodGet, "/person/cpf/"+cpf, nil)

	var fields []string
	var parsed map[string]any
	if json.Unmarshal(g.body, &parsed) == nil {
		for k := range parsed {
			fields = append(fields, "`"+k+"`")
		}
		sort.Strings(fields)
	}

	record(9, "Que PII o `GET /person/cpf/{cpf}` expõe?",
		fmt.Sprintf("HTTP **%d**, **%d campos**. Confirma: o CPF sozinho abre o cadastro completo de terceiro. "+
			"Nada disso pode sair na resposta do nosso `/auth/register`.", g.status, len(parsed)),
		"Campos devolvidos: "+strings.Join(fields, ", ")+"\n\nResposta completa:\n\n"+
			codeBlock("json", g.pretty(1500)))
}

// 10. Latência real calibra timeout por tentativa e nº de tentativas — e diz se
// o orçamento de 20s cabe no WriteTimeout do servidor.
func probeLatency(t *testing.T, c *probeClient) {
	// GET e POST são medidos separado: a dor está no POST, e usar a latência do
	// GET para calibrar o timeout do cadastro subestimaria o problema em ~50x.
	gets := make([]time.Duration, 0, 8)
	for i := 0; i < cap(gets); i++ {
		gets = append(gets, c.do(t, http.MethodGet, "/person/cpf/"+randomCPF(), nil).elapsed)
	}

	type postSample struct {
		elapsed time.Duration
		status  int
	}
	posts := make([]postSample, 0, 3)
	for i := 0; i < cap(posts); i++ {
		p := minimalPerson()
		p["name"] = fmt.Sprintf("%s Latencia %d", probeNamePrefix, i)
		r := c.createPerson(t, p)
		posts = append(posts, postSample{r.elapsed, r.status})
	}

	sort.Slice(gets, func(i, j int) bool { return gets[i] < gets[j] })
	sort.Slice(posts, func(i, j int) bool { return posts[i].elapsed < posts[j].elapsed })

	var rows strings.Builder
	rows.WriteString("| Operação | Amostra | Duração | HTTP |\n|---|---|---|---|\n")
	for i, s := range gets {
		fmt.Fprintf(&rows, "| `GET /person/cpf` | %d | %s | — |\n", i+1, s.Round(time.Millisecond))
	}
	for i, s := range posts {
		fmt.Fprintf(&rows, "| `POST /person` | %d | %s | %d |\n", i+1, s.elapsed.Round(time.Millisecond), s.status)
	}

	verdict := fmt.Sprintf("`GET` p50 **%s** · máx **%s** (n=%d). `POST` mediana **%s** · máx **%s** (n=%d).",
		gets[len(gets)/2].Round(time.Millisecond), gets[len(gets)-1].Round(time.Millisecond), len(gets),
		posts[len(posts)/2].elapsed.Round(time.Millisecond), posts[len(posts)-1].elapsed.Round(time.Millisecond), len(posts))

	record(10, "Qual a latência real da DAV?", verdict,
		"**O teto é do AWS API Gateway, não da DAV.** A sondagem já pegou um `POST /person`\n"+
			"morrer em ~29s com `{\"message\": \"Endpoint request timed out\"}` — o limite rígido de\n"+
			"integração do API Gateway. Consequências para o adapter:\n\n"+
			"- Timeout por tentativa **acima de ~29s é inútil**: o gateway desiste antes.\n"+
			"- Um **504 não significa que falhou** — a criação pode ter chegado ao backend deles.\n"+
			"  Por isso o retry precisa sondar com `GET /person/{nosso-id}` antes de repetir o POST\n"+
			"  (viável graças ao achado #4).\n"+
			"- O orçamento total precisa caber no `RENOVI_HTTP_WRITE_TIMEOUT` (hoje **15s**) — daí o\n"+
			"  `SetWriteDeadline` no handler de register.\n\n"+rows.String())
}

// ---------------------------------------------------------------------------
// Limpeza
// ---------------------------------------------------------------------------

// cleanup remove as pessoas de teste. A exclusão da DAV é lógica (o id nunca é
// reaproveitado), então isto é higiene, não garantia.
func cleanup(t *testing.T, c *probeClient) {
	t.Helper()
	var ids []string
	createdIDs.Range(func(k, _ any) bool {
		ids = append(ids, k.(string))
		return true
	})

	deleted := 0
	for _, id := range ids {
		r := c.do(t, http.MethodDelete, "/person/"+id+"?soft=true", nil)
		if r.status >= 200 && r.status < 300 {
			deleted++
		} else {
			t.Logf("limpeza: DELETE /person/%s devolveu %d", id, r.status)
		}
	}
	t.Logf("limpeza: %d/%d pessoas de sondagem removidas", deleted, len(ids))
}

func jsonOf(v any) string {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("(erro ao serializar: %v)", err)
	}
	return string(raw)
}
