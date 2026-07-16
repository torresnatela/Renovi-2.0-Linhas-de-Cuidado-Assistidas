// Package dav é o Adapter da API Doutor ao Vivo (docs/ARQUITETURA.md §5) — o
// sistema onde vivem os dados de saúde do paciente e a teleconsulta.
//
// O comportamento real da DAV está mapeado em docs/DAV-API-NOTAS.md, gerado por
// `make dav-probe`. Vale mais que o spec publicado, que se contradiz. Os três
// pontos que mais moldam este código:
//
//  1. O lookup por CPF devolve 204 (não 404) quando não acha.
//  2. A DAV aceita um `id` gerado por nós, então o POST é sondável: um 504 não
//     quer dizer que falhou, e dá para conferir com GET /person/{nosso-id}. Por
//     isso escrita NUNCA se repete aqui (viraria 409 "id already exists") — ela
//     devolve ErrMaybeApplied e quem chama sonda. Só as leituras repetem.
//  3. O teto de ~29s é do AWS API Gateway na frente deles. Timeout por
//     tentativa acima disso é inútil.
package dav

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Erros do adapter. Use errors.Is.
var (
	// ErrDuplicateCPF: já existe pessoa com este CPF na DAV. Não é retriável —
	// quem chama deve buscar a pessoa por CPF e anexá-la.
	ErrDuplicateCPF = errors.New("dav: cpf já cadastrado")
	// ErrDuplicateEmail: e-mail já usado por OUTRA pessoa na DAV. A DAV exige
	// e-mail único, então casal/família que compartilha e-mail esbarra aqui.
	ErrDuplicateEmail = errors.New("dav: e-mail já cadastrado")
	// ErrValidation: a DAV recusou o payload (ex.: CPF com DV inválido).
	ErrValidation = errors.New("dav: payload recusado")
	// ErrUnavailable: a DAV não respondeu de forma utilizável. Usado nas
	// operações de LEITURA, onde não houve efeito colateral.
	ErrUnavailable = errors.New("dav: indisponível")
	// ErrMaybeApplied: a escrita PODE ter sido aplicada — não sabemos.
	//
	// É o erro mais importante deste pacote, e existe porque a realidade nos
	// ensinou: um POST /person estourou o teto de 29s do gateway (504) e mesmo
	// assim criou a pessoa. Quem recebe este erro NÃO pode concluir que falhou:
	// precisa sondar (GetPerson / FindPersonByCPF) antes de decidir.
	ErrMaybeApplied = errors.New("dav: resultado desconhecido — a escrita pode ter sido aplicada")
)

// Person é o subconjunto de PERSON que nos interessa.
//
// Enxuto de propósito: o GET /person/cpf devolve 12 campos, incluindo endereço,
// celular e nome da mãe de QUALQUER pessoa, com o CPF como única chave. Não
// carregamos o que não vamos usar — o que não existe não vaza.
type Person struct {
	ID        string
	Name      string
	CPF       string
	BirthDate string
	Email     string
}

// Address é o endereço no formato da DAV.
type Address struct {
	ZipCode      string
	Street       string
	Number       string
	Complement   string
	Neighborhood string
	City         string
	State        string
	Country      string
}

// CreatePersonInput são os dados para criar a pessoa na DAV.
type CreatePersonInput struct {
	// ID é o nosso UUIDv7. A DAV o aceita e o devolve, o que dá idempotência ao
	// POST (ver docs/DAV-API-NOTAS.md, achado #4).
	ID        string
	Name      string
	CPF       string
	BirthDate string // YYYY-MM-DD
	Email     string
	CellPhone string
	Address   *Address
}

// Config parametriza o Client.
type Config struct {
	BaseURL     string
	APIKey      string
	Timeout     time.Duration // por tentativa
	MaxAttempts int
	BaseBackoff time.Duration
	Logger      *slog.Logger
	HTTPClient  *http.Client
}

// Client fala com a DAV.
type Client struct {
	baseURL     string
	apiKey      string
	timeout     time.Duration
	maxAttempts int
	baseBackoff time.Duration
	http        *http.Client
	logger      *slog.Logger
}

// New monta o Client. Falha alto se faltar base URL ou chave, em vez de deixar
// o erro aparecer como um 401 misterioso em produção.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("dav: BaseURL é obrigatório")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("dav: APIKey é obrigatório")
	}

	c := &Client{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:      cfg.APIKey,
		timeout:     cfg.Timeout,
		maxAttempts: cfg.MaxAttempts,
		baseBackoff: cfg.BaseBackoff,
		http:        cfg.HTTPClient,
		logger:      cfg.Logger,
	}
	if c.maxAttempts < 1 {
		c.maxAttempts = 3
	}
	if c.baseBackoff <= 0 {
		c.baseBackoff = 200 * time.Millisecond
	}
	if c.timeout <= 0 {
		c.timeout = 30 * time.Second
	}
	if c.http == nil {
		c.http = &http.Client{}
	}
	if c.logger == nil {
		c.logger = slog.Default()
	}
	return c, nil
}

// FindPersonByCPF procura a pessoa pelo CPF. Devolve (nil, nil) quando não
// existe — a DAV sinaliza isso com 204, não 404.
func (c *Client) FindPersonByCPF(ctx context.Context, cpf string) (*Person, error) {
	return c.fetchPerson(ctx, "/person/cpf/"+cpf, "GET /person/cpf/{cpf}")
}

// GetPerson busca a pessoa pelo id da DAV. É a sonda de reconciliação: depois de
// um 504 no POST, dizer se a criação chegou a acontecer.
func (c *Client) GetPerson(ctx context.Context, id string) (*Person, error) {
	return c.fetchPerson(ctx, "/person/"+id, "GET /person/{id}")
}

func (c *Client) fetchPerson(ctx context.Context, path, route string) (*Person, error) {
	res, err := c.doWithRetry(ctx, http.MethodGet, path, route, nil, c.maxAttempts)
	if err != nil {
		return nil, err
	}

	// 204 = não encontrado. Tratar como "sucesso vazio" faria o cadastro achar
	// que ninguém tem o CPF e tentar criar sempre.
	if res.status == http.StatusNoContent {
		return nil, nil
	}
	if res.status != http.StatusOK {
		return nil, c.classify(res, route)
	}

	var body struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		CPF       string `json:"cpf"`
		BirthDate string `json:"birth_date"`
		Email     string `json:"email"`
	}
	if err := json.Unmarshal(res.body, &body); err != nil {
		return nil, fmt.Errorf("%w: resposta ilegível em %s", ErrUnavailable, route)
	}
	return &Person{
		ID: body.ID, Name: body.Name, CPF: body.CPF,
		BirthDate: body.BirthDate, Email: body.Email,
	}, nil
}

// CreatePerson cria a pessoa e devolve o id dela na DAV.
//
// Devolve o id que a DAV RESPONDER, não o que enviamos: se um dia ela deixar de
// honrar o id do integrador, o vínculo continua correto em vez de apontar para
// um registro que não existe.
func (c *Client) CreatePerson(ctx context.Context, in CreatePersonInput) (string, error) {
	const route = "POST /person"

	body := createPersonBody{
		ID:        in.ID,
		Name:      in.Name,
		CPF:       in.CPF,
		BirthDate: in.BirthDate,
		Email:     in.Email,
		CellPhone: in.CellPhone,
		// A sondagem mostrou que `status` é opcional na prática, mas o spec o
		// declara obrigatório. Enviar sempre satisfaz os dois.
		Status: true,
	}
	if in.Address != nil {
		body.Address = &addressBody{
			ZipCode: in.Address.ZipCode, Street: in.Address.Street,
			Number: in.Address.Number, Complement: in.Address.Complement,
			Neighborhood: in.Address.Neighborhood,
			// Nome do município: a DAV aceita nome ou código IBGE (achado #8),
			// e o nome dispensa um lookup de CEP -> IBGE no cadastro.
			City: in.Address.City, State: in.Address.State,
			Country: orDefault(in.Address.Country, "BR"),
		}
	}

	// UMA tentativa, nunca mais. Um POST que estourou pode ter sido aplicado —
	// repeti-lo devolve 409 "id already exists" e transforma sucesso em falha.
	// Quem reconcilia é o chamador, sondando; ver ErrMaybeApplied.
	res, err := c.doWithRetry(ctx, http.MethodPost, "/person", route, body, 1)
	if err != nil {
		// Erro de transporte numa escrita: resultado desconhecido.
		//
		// Dois %w de propósito: quem chama precisa reconhecer o ErrMaybeApplied
		// (para sondar) E o context.Canceled (para não tratar desistência do
		// usuário como incidente). Um cancelamento antes do envio não aplicou
		// nada, mas errar para "pode ter aplicado" é o lado seguro: o pior que
		// acontece é uma sonda a mais.
		return "", fmt.Errorf("%w: %w", ErrMaybeApplied, err)
	}
	if res.status != http.StatusOK && res.status != http.StatusCreated {
		return "", c.classifyWrite(res, route)
	}

	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res.body, &out); err != nil || out.ID == "" {
		// 2xx significa que a DAV ACEITOU: a pessoa existe lá, ainda que não
		// consigamos ler o id. ErrUnavailable faria o cadastro falhar sem
		// reconciliar — o mesmo erro do 409 que este PR corrigiu. Com
		// ErrMaybeApplied, o model sonda pelo id que enviamos e conclui.
		return "", fmt.Errorf("%w: %s respondeu %d sem id utilizável", ErrMaybeApplied, route, res.status)
	}
	return out.ID, nil
}

type addressBody struct {
	Country      string `json:"country"`
	ZipCode      string `json:"zip_code"`
	Street       string `json:"street"`
	Number       string `json:"number"`
	Complement   string `json:"complement,omitempty"`
	Neighborhood string `json:"neighborhood"`
	City         string `json:"city"`
	State        string `json:"state"`
}

type createPersonBody struct {
	ID        string       `json:"id,omitempty"`
	Name      string       `json:"name"`
	CPF       string       `json:"cpf"`
	BirthDate string       `json:"birth_date"`
	Email     string       `json:"email,omitempty"`
	CellPhone string       `json:"cell_phone,omitempty"`
	Address   *addressBody `json:"address,omitempty"`
	Status    bool         `json:"status"`
}

// ---------------------------------------------------------------------------
// Transporte
// ---------------------------------------------------------------------------

type response struct {
	status int
	body   []byte
}

// doWithRetry executa a chamada, repetindo apenas o que é transitório.
//
// `route` é um rótulo SEM dados (ex.: "GET /person/cpf/{cpf}"): é ele que vai
// para o log, nunca o path real — que carrega o CPF.
func (c *Client) doWithRetry(ctx context.Context, method, path, route string, body any, attempts int) (*response, error) {
	var last error

	for attempt := 1; attempt <= attempts; attempt++ {
		// Quem chama desistiu (usuário fechou a aba, request cancelado): parar
		// já, em vez de queimar as tentativas restantes contra uma API lenta.
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		res, err := c.doOnce(ctx, method, path, route, body, attempt)
		switch {
		case err != nil:
			// Erro de transporte (timeout da tentativa, conexão recusada).
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			last = err
		case !retryable(res.status):
			return res, nil
		default:
			last = fmt.Errorf("%s respondeu %d", route, res.status)
		}

		if attempt < attempts {
			if err := sleep(ctx, c.backoff(attempt)); err != nil {
				return nil, err
			}
		}
	}

	return nil, fmt.Errorf("%w: %s falhou em %d tentativa(s): %v", ErrUnavailable, route, attempts, last)
}

func (c *Client) doOnce(ctx context.Context, method, path, route string, body any, attempt int) (*response, error) {
	var buf io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("serializar corpo de %s: %w", route, err)
		}
		buf = bytes.NewReader(raw)
	}

	// Deadline por TENTATIVA aqui, e não em http.Client.Timeout: com um
	// HTTPClient injetado (testes, instrumentação), o Timeout do config era
	// silenciosamente ignorado e a chamada só parava quando o contexto pai
	// caísse — ou seja, nunca, na prática.
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, buf)
	if err != nil {
		return nil, fmt.Errorf("montar %s: %w", route, err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	start := time.Now()
	resp, err := c.http.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		// %v e não %w: o erro do net/http traz a URL, e a URL traz o CPF.
		c.logger.WarnContext(ctx, "dav: chamada falhou",
			"route", route, "attempt", attempt, "duration_ms", elapsed.Milliseconds(),
			"error", classifyTransport(err))
		return nil, fmt.Errorf("%s: falha de transporte na tentativa %d", route, attempt)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: ler resposta: %w", route, err)
	}

	// LGPD (CLAUDE.md): só metadados. Nunca o corpo (que tem CPF, nome, e-mail e
	// endereço), nunca o path real, nunca a chave. O `trace` é da DAV e existe
	// justamente para acionar o suporte deles.
	c.logger.InfoContext(ctx, "dav: chamada",
		"route", route, "attempt", attempt,
		"status", resp.StatusCode, "duration_ms", elapsed.Milliseconds(),
		"dav_trace", traceOf(raw))

	return &response{status: resp.StatusCode, body: raw}, nil
}

// retryable diz se vale repetir. 4xx nunca: a DAV já decidiu, e insistir só
// gasta ~2s de latência dela dentro de um cadastro que o usuário está esperando.
//
// 504 entra aqui, mas com uma ressalva que o chamador precisa conhecer: o POST
// pode ter sido aplicado antes do gateway desistir. Por isso o retry do cadastro
// começa pelo lookup por CPF.
func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func (c *Client) backoff(attempt int) time.Duration {
	return c.baseBackoff * time.Duration(1<<(attempt-1)) // 1x, 2x, 4x...
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// classifyTransport reduz o erro do net/http a um rótulo. O erro original traz a
// URL completa — e a URL do lookup contém o CPF.
func classifyTransport(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelado"
	default:
		return "erro de conexão"
	}
}

// ---------------------------------------------------------------------------
// Erros da DAV
// ---------------------------------------------------------------------------

// apiError é o DavHttpExceptionDto.
type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Trace   string `json:"trace"`
	Detail  []struct {
		Message string `json:"message"`
		I18n    struct {
			Phrase   string            `json:"phrase"`
			Mustache map[string]string `json:"mustache"`
		} `json:"i18n"`
	} `json:"detail"`
}

const phraseAlreadyExists = "entity.unique.attribute.already.exists"

// classify traduz o erro da DAV nos nossos.
//
// A parte chata: CPF duplicado e CPF inválido chegam com a MESMA message
// ("Person invalid"). Só o detail[].i18n.phrase os separa — e a diferença
// importa, porque um significa "essa pessoa já existe lá" e o outro "seu CPF
// está errado". Ver docs/DAV-API-NOTAS.md, achados #5 e #7.
func (c *Client) classify(res *response, route string) error {
	var e apiError
	_ = json.Unmarshal(res.body, &e)

	if res.status == http.StatusBadRequest || res.status == http.StatusUnprocessableEntity {
		for _, d := range e.Detail {
			if d.I18n.Phrase != phraseAlreadyExists {
				continue
			}
			switch d.I18n.Mustache["field"] {
			case "cpf":
				return fmt.Errorf("%w (trace %s)", ErrDuplicateCPF, e.Trace)
			case "email":
				return fmt.Errorf("%w (trace %s)", ErrDuplicateEmail, e.Trace)
			}
		}

		// O e-mail duplicado foge do padrão: vem sem i18n e sem detail, só com
		// uma frase em português cravada. Casar por literal é frágil — se a DAV
		// traduzir a mensagem, isto vira ErrValidation e a UI passa a mostrar
		// "dados inválidos" no lugar de "e-mail já em uso". Aceitável porque o
		// caso acima (por i18n) cobre a forma correta, se um dia eles a usarem.
		if strings.Contains(strings.ToLower(e.Message), "email já cadastrado") {
			return fmt.Errorf("%w (trace %s)", ErrDuplicateEmail, e.Trace)
		}

		return fmt.Errorf("%w: %s (trace %s)", ErrValidation, e.Message, e.Trace)
	}

	return fmt.Errorf("%w: %s respondeu %d (trace %s)", ErrUnavailable, route, res.status, e.Trace)
}

// classifyWrite é o classify das ESCRITAS. A diferença é o que se conclui do
// desconhecido: numa leitura, um 5xx não deixou rastro; numa escrita, pode ter
// deixado — e concluir "falhou" é o erro que reprova cadastros que funcionaram.
func (c *Client) classifyWrite(res *response, route string) error {
	// 409 "id already exists.": a DAV está dizendo que um POST NOSSO anterior
	// pegou — o id é gerado por nós. É a QUARTA forma de erro deles (sem i18n,
	// sem detail), e a que revelou o bug: veio depois de um 504 que "falhou".
	if res.status == http.StatusConflict {
		var e apiError
		_ = json.Unmarshal(res.body, &e)
		return fmt.Errorf("%w: %s respondeu 409 (%s, trace %s)", ErrMaybeApplied, route, e.Message, e.Trace)
	}

	// 4xx é opinião firme da DAV sobre o payload: não houve efeito.
	if res.status >= 400 && res.status < 500 {
		return c.classify(res, route)
	}

	// 5xx numa escrita: pode ter sido aplicado. Sonde antes de concluir.
	var e apiError
	_ = json.Unmarshal(res.body, &e)
	return fmt.Errorf("%w: %s respondeu %d (trace %s)", ErrMaybeApplied, route, res.status, e.Trace)
}

// traceOf extrai só o trace da DAV do corpo — o único campo seguro de logar.
func traceOf(raw []byte) string {
	var e apiError
	if json.Unmarshal(raw, &e) != nil {
		return ""
	}
	return e.Trace
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
