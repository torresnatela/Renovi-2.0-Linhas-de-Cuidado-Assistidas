package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/renovisaude/renovi-care/internal/adapters/agenda"
	"github.com/renovisaude/renovi-care/internal/db/gen"
	"github.com/renovisaude/renovi-care/internal/models/careline"
)

// Erros do catálogo de linhas de cuidado. Use errors.Is (ou errors.As para o
// ErrCareLineInvalid, que carrega a lista de problemas).
var (
	// ErrCareLineDraftExists: já existe um rascunho aberto para este code. A trava
	// é o índice parcial ux_care_line_draft — um code tem no máximo um draft.
	ErrCareLineDraftExists = errors.New("catálogo: já existe um rascunho para este code")
	// ErrCareLinePublished: a linha já está publicada e é imutável (não aceita item,
	// regra nem uma segunda publicação).
	ErrCareLinePublished = errors.New("catálogo: linha de cuidado já publicada")
	// ErrCareLineNotFound: a linha (ou o item referenciado) não existe.
	ErrCareLineNotFound = errors.New("catálogo: linha de cuidado não encontrada")
)

// Vocabulário do catálogo (espelha os CHECKs das migrations 0005/0009). Os kinds
// vêm do motor puro, fonte da verdade do vocabulário.
const (
	careLineStatusDraft = "draft"
	itemKindConsulta    = careline.KindConsulta
	itemKindAtividade   = careline.KindAtividade
)

// ErrCareLineInvalid reúne, de uma vez, os problemas que impedem uma operação de
// catálogo — os params de uma regra, ou o template inteiro na publicação. Existe
// como struct (e não sentinela) porque o admin quer corrigir tudo junto; o
// controller a devolve em `errors[]`.
type ErrCareLineInvalid struct {
	Errors []string
}

func (e ErrCareLineInvalid) Error() string {
	return "catálogo: linha de cuidado inválida: " + strings.Join(e.Errors, "; ")
}

// SpecialtyLister lista as especialidades ativas do legado, para o publish validar
// que todo item aponta para uma especialidade que existe. A interface vive AQUI, no
// consumidor (ADR-012): o publish só precisa disto do adapter da agenda.
type SpecialtyLister interface {
	ListSpecialties(ctx context.Context, now time.Time) ([]agenda.Specialty, error)
}

// CareLine é o espelho amigável do template versionado, com itens e regras já
// resolvidos para a resposta. Mapeado dos tipos gerados (gen), nunca exposto cru.
type CareLine struct {
	ID          uuid.UUID
	Code        string
	Version     int
	Name        string
	Description string
	Status      string
	PublishedAt *time.Time
	Items       []CareLineItem
}

// CareLineItem é um passo da linha (no slice, uma consulta), com suas regras.
type CareLineItem struct {
	ID            uuid.UUID
	Ref           string
	Kind          string
	SpecialtyCode string
	Label         string
	Recurrence    *string
	SortOrder     int
	Rules         []CareLineRule
}

// CareLineRule é uma regra de elegibilidade presa a um item. Params fica cru
// (JSON) — cada rule_type tem sua forma, resolvida pelo motor puro.
type CareLineRule struct {
	RuleType string
	Params   json.RawMessage
}

// AddItemInput são os dados de um item novo. Kind vazio assume CONSULTA (o único
// tipo do slice); SortOrder nil assume 0.
type AddItemInput struct {
	Ref           string
	Kind          string
	SpecialtyCode string
	Label         string
	Recurrence    *string
	SortOrder     *int
}

// CareLineStore é a camada de dados + regra do catálogo.
type CareLineStore struct {
	pool        *pgxpool.Pool
	q           *gen.Queries
	specialties SpecialtyLister
}

// NewCareLineStore monta o store. specialties é consultado só na publicação.
func NewCareLineStore(pool *pgxpool.Pool, specialties SpecialtyLister) *CareLineStore {
	return &CareLineStore{pool: pool, q: gen.New(pool), specialties: specialties}
}

// Create abre um rascunho novo do code. A versão é MAX+1 (NextCareLineVersion), e o
// índice ux_care_line_draft garante que só há um draft por code — uma corrida ou
// um segundo draft vira ErrCareLineDraftExists.
func (s *CareLineStore) Create(ctx context.Context, code, name, description string) (CareLine, error) {
	code = strings.TrimSpace(code)
	name = strings.TrimSpace(name)
	var problems []string
	if code == "" {
		problems = append(problems, "code é obrigatório")
	}
	if name == "" {
		problems = append(problems, "name é obrigatório")
	}
	if len(problems) > 0 {
		return CareLine{}, ErrCareLineInvalid{Errors: problems}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CareLine{}, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	version, err := q.NextCareLineVersion(ctx, code)
	if err != nil {
		return CareLine{}, fmt.Errorf("próxima versão: %w", err)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return CareLine{}, fmt.Errorf("gerar uuid v7: %w", err)
	}
	row, err := q.CreateCareLine(ctx, gen.CreateCareLineParams{
		ID: id, Code: code, Version: version, Name: name, Description: description,
	})
	if err != nil {
		if isUniqueViolation(err) {
			// Colisão em ux_care_line_draft (ou code+version): há draft aberto.
			return CareLine{}, ErrCareLineDraftExists
		}
		return CareLine{}, fmt.Errorf("criar linha: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CareLine{}, fmt.Errorf("commit: %w", err)
	}
	return toCareLine(row, nil), nil
}

// AddItem acrescenta um item a uma linha em draft. Published é imutável (409);
// kind fora de CONSULTA e ref duplicado são erro de validação (400).
func (s *CareLineStore) AddItem(ctx context.Context, careLineID uuid.UUID, in AddItemInput) (CareLineItem, error) {
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = itemKindConsulta
	}
	ref := strings.TrimSpace(in.Ref)
	specialty := strings.TrimSpace(in.SpecialtyCode)
	label := strings.TrimSpace(in.Label)

	var problems []string
	if kind != itemKindConsulta && kind != itemKindAtividade {
		problems = append(problems, fmt.Sprintf("kind %q não é suportado (use CONSULTA ou ATIVIDADE)", in.Kind))
	}
	if ref == "" {
		problems = append(problems, "ref é obrigatório")
	}
	// specialty_code é condicional ao kind: CONSULTA exige, ATIVIDADE não tem.
	switch kind {
	case itemKindConsulta:
		if specialty == "" {
			problems = append(problems, "specialty_code é obrigatório para CONSULTA")
		}
	case itemKindAtividade:
		if specialty != "" {
			problems = append(problems, "specialty_code não se aplica a ATIVIDADE")
		}
	}
	if label == "" {
		problems = append(problems, "label é obrigatório")
	}
	if len(problems) > 0 {
		return CareLineItem{}, ErrCareLineInvalid{Errors: problems}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CareLineItem{}, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	if err := s.requireDraft(ctx, q, careLineID); err != nil {
		return CareLineItem{}, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return CareLineItem{}, fmt.Errorf("gerar uuid v7: %w", err)
	}
	sortOrder := int32(0)
	if in.SortOrder != nil {
		sortOrder = int32(*in.SortOrder)
	}
	// specialty_code é NULL para ATIVIDADE (só CONSULTA aponta para especialidade).
	var specialtyPtr *string
	if kind == itemKindConsulta {
		specialtyPtr = &specialty
	}
	item, err := q.InsertCareLineItem(ctx, gen.InsertCareLineItemParams{
		ID: id, CareLineID: careLineID, Ref: ref, Kind: kind,
		SpecialtyCode: textPtr(specialtyPtr), Label: label,
		Recurrence: textPtr(in.Recurrence), SortOrder: sortOrder,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return CareLineItem{}, ErrCareLineInvalid{Errors: []string{
				fmt.Sprintf("já existe um item com o ref %q nesta linha", ref)}}
		}
		return CareLineItem{}, fmt.Errorf("inserir item: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CareLineItem{}, fmt.Errorf("commit: %w", err)
	}
	return toCareLineItem(item, nil), nil
}

// AddRule prende uma regra a um item de uma linha em draft. Os params são validados
// JÁ AQUI, com o mesmo ParseRuleParams do motor: config com typo ou valor absurdo
// não entra no banco (o publish revalida o conjunto). item_ref inexistente é 404.
func (s *CareLineStore) AddRule(ctx context.Context, careLineID uuid.UUID, itemRef, ruleType string, params json.RawMessage) (CareLineRule, error) {
	ruleType = strings.TrimSpace(ruleType)
	if !isStorableRule(ruleType) {
		return CareLineRule{}, ErrCareLineInvalid{Errors: []string{
			fmt.Sprintf("rule_type %q não é suportado (use QUOTA, MIN_INTERVAL, MAX_ADVANCE ou PREREQUISITE)", ruleType)}}
	}
	if _, err := careline.ParseRuleParams(ruleType, params); err != nil {
		return CareLineRule{}, ErrCareLineInvalid{Errors: []string{err.Error()}}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CareLineRule{}, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	if err := s.requireDraft(ctx, q, careLineID); err != nil {
		return CareLineRule{}, err
	}

	items, err := q.ListItemsByCareLine(ctx, careLineID)
	if err != nil {
		return CareLineRule{}, fmt.Errorf("listar itens: %w", err)
	}
	var itemID uuid.UUID
	found := false
	for _, it := range items {
		if it.Ref == itemRef {
			itemID = it.ID
			found = true
			break
		}
	}
	if !found {
		// O par (linha, item_ref) endereça um recurso; item ausente responde 404,
		// como a linha ausente.
		return CareLineRule{}, ErrCareLineNotFound
	}

	id, err := uuid.NewV7()
	if err != nil {
		return CareLineRule{}, fmt.Errorf("gerar uuid v7: %w", err)
	}
	rule, err := q.InsertCareLineRule(ctx, gen.InsertCareLineRuleParams{
		ID: id, CareLineItemID: itemID, RuleType: ruleType, Params: []byte(params),
	})
	if err != nil {
		return CareLineRule{}, fmt.Errorf("inserir regra: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CareLineRule{}, fmt.Errorf("commit: %w", err)
	}
	return CareLineRule{RuleType: rule.RuleType, Params: json.RawMessage(rule.Params)}, nil
}

// Publish valida o template inteiro (itens com especialidade do legado,
// pré-requisitos coerentes, sem ciclos) e, se tudo passa, sela a versão. Legado
// indisponível sobe intacto (o controller vira 503); template inconsistente vira
// ErrCareLineInvalid (400 com a lista toda).
func (s *CareLineStore) Publish(ctx context.Context, id uuid.UUID, now time.Time) (CareLine, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return CareLine{}, fmt.Errorf("abrir transação: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	// FOR UPDATE: valida o template e vira o status na MESMA transação, para que dois
	// publishes concorrentes da mesma linha não validem os dois contra o estado draft
	// e selem duas vezes — o segundo espera a trava e vê o status já publicado.
	line, err := q.GetCareLineForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CareLine{}, ErrCareLineNotFound
		}
		return CareLine{}, fmt.Errorf("travar linha: %w", err)
	}
	if line.Status != careLineStatusDraft {
		return CareLine{}, ErrCareLinePublished
	}

	items, err := q.ListItemsByCareLine(ctx, id)
	if err != nil {
		return CareLine{}, fmt.Errorf("listar itens: %w", err)
	}
	ruleRows, err := q.ListRulesByCareLine(ctx, id)
	if err != nil {
		return CareLine{}, fmt.Errorf("listar regras: %w", err)
	}

	// Especialidades do legado. Uma falha aqui (agenda.ErrUnavailable) sobe intacta:
	// o publish depende do legado e não pode selar uma linha sem confirmar isto.
	specs, err := s.specialties.ListSpecialties(ctx, now)
	if err != nil {
		return CareLine{}, err
	}
	names := make([]string, 0, len(specs))
	for _, sp := range specs {
		names = append(names, sp.Name)
	}

	engineItems, engineRules := toEngine(items, ruleRows)
	if problems := careline.ValidatePublish(engineItems, engineRules, names); len(problems) > 0 {
		return CareLine{}, ErrCareLineInvalid{Errors: problems}
	}

	n, err := q.PublishCareLine(ctx, gen.PublishCareLineParams{
		ID: id, PublishedAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return CareLine{}, fmt.Errorf("publicar linha: %w", err)
	}
	if n == 0 {
		// O guard status='draft' não casou: a linha travada não era mais draft.
		return CareLine{}, ErrCareLinePublished
	}

	if err := tx.Commit(ctx); err != nil {
		return CareLine{}, fmt.Errorf("commit: %w", err)
	}
	return s.Get(ctx, id)
}

// ListVersions traz as linhas do catálogo, já com itens e regras. Code vazio = tudo;
// senão, todas as versões daquele code.
func (s *CareLineStore) ListVersions(ctx context.Context, code string) ([]CareLine, error) {
	code = strings.TrimSpace(code)
	var rows []gen.CareLine
	var err error
	if code == "" {
		rows, err = s.q.ListCareLines(ctx)
	} else {
		rows, err = s.q.ListCareLinesByCode(ctx, code)
	}
	if err != nil {
		return nil, fmt.Errorf("listar linhas: %w", err)
	}

	out := make([]CareLine, 0, len(rows))
	for _, row := range rows {
		cl, err := s.hydrate(ctx, row)
		if err != nil {
			return nil, err
		}
		out = append(out, cl)
	}
	return out, nil
}

// Get devolve uma linha completa (itens + regras).
func (s *CareLineStore) Get(ctx context.Context, id uuid.UUID) (CareLine, error) {
	row, err := s.q.GetCareLine(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CareLine{}, ErrCareLineNotFound
		}
		return CareLine{}, fmt.Errorf("carregar linha: %w", err)
	}
	return s.hydrate(ctx, row)
}

// requireDraft carrega a linha e exige que ela esteja em draft. NotFound quando não
// existe; Published quando já foi selada.
func (s *CareLineStore) requireDraft(ctx context.Context, q *gen.Queries, id uuid.UUID) error {
	line, err := q.GetCareLine(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCareLineNotFound
		}
		return fmt.Errorf("carregar linha: %w", err)
	}
	if line.Status != careLineStatusDraft {
		return ErrCareLinePublished
	}
	return nil
}

// hydrate monta a CareLine completa a partir da linha crua: busca itens e regras e
// agrupa as regras por item.
func (s *CareLineStore) hydrate(ctx context.Context, row gen.CareLine) (CareLine, error) {
	items, err := s.q.ListItemsByCareLine(ctx, row.ID)
	if err != nil {
		return CareLine{}, fmt.Errorf("listar itens: %w", err)
	}
	ruleRows, err := s.q.ListRulesByCareLine(ctx, row.ID)
	if err != nil {
		return CareLine{}, fmt.Errorf("listar regras: %w", err)
	}

	byItem := make(map[uuid.UUID][]CareLineRule, len(items))
	for _, rr := range ruleRows {
		byItem[rr.CareLineRule.CareLineItemID] = append(byItem[rr.CareLineRule.CareLineItemID], CareLineRule{
			RuleType: rr.CareLineRule.RuleType,
			Params:   json.RawMessage(rr.CareLineRule.Params),
		})
	}

	out := make([]CareLineItem, 0, len(items))
	for _, it := range items {
		out = append(out, toCareLineItem(it, byItem[it.ID]))
	}
	return toCareLine(row, out), nil
}

// ---------------------------------------------------------------------------
// Mapeamento gen -> domínio
// ---------------------------------------------------------------------------

func toCareLine(row gen.CareLine, items []CareLineItem) CareLine {
	if items == nil {
		items = []CareLineItem{}
	}
	return CareLine{
		ID:          row.ID,
		Code:        row.Code,
		Version:     int(row.Version),
		Name:        row.Name,
		Description: row.Description,
		Status:      row.Status,
		PublishedAt: timestamptzPtr(row.PublishedAt),
		Items:       items,
	}
}

func toCareLineItem(row gen.CareLineItem, rules []CareLineRule) CareLineItem {
	if rules == nil {
		rules = []CareLineRule{}
	}
	return CareLineItem{
		ID:            row.ID,
		Ref:           row.Ref,
		Kind:          row.Kind,
		SpecialtyCode: row.SpecialtyCode.String, // "" quando NULL (ATIVIDADE)
		Label:         row.Label,
		Recurrence:    textToPtr(row.Recurrence),
		SortOrder:     int(row.SortOrder),
		Rules:         rules,
	}
}

// toEngine converte itens e regras crus para os tipos do motor puro, que
// ValidatePublish consome (agrupando as regras por ref do item).
func toEngine(items []gen.CareLineItem, ruleRows []gen.ListRulesByCareLineRow) ([]careline.Item, map[string][]careline.Rule) {
	engineItems := make([]careline.Item, 0, len(items))
	for _, it := range items {
		engineItems = append(engineItems, careline.Item{
			Ref:           it.Ref,
			Kind:          it.Kind,
			SpecialtyCode: it.SpecialtyCode.String, // "" quando NULL (ATIVIDADE)
			Label:         it.Label,
		})
	}
	engineRules := make(map[string][]careline.Rule, len(ruleRows))
	for _, rr := range ruleRows {
		engineRules[rr.ItemRef] = append(engineRules[rr.ItemRef], careline.Rule{
			Type:   rr.CareLineRule.RuleType,
			Params: json.RawMessage(rr.CareLineRule.Params),
		})
	}
	return engineItems, engineRules
}

// ---------------------------------------------------------------------------
// Auxiliares
// ---------------------------------------------------------------------------

func isStorableRule(ruleType string) bool {
	switch ruleType {
	case careline.RuleQuota, careline.RuleMinInterval, careline.RuleMaxAdvance, careline.RulePrerequisite:
		return true
	}
	return false
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

// textPtr converte um *string opcional em pgtype.Text (nil -> nulo).
func textPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}

// textToPtr é o inverso: pgtype.Text -> *string (nulo -> nil).
func textToPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

// timestamptzPtr converte pgtype.Timestamptz em *time.Time (nulo -> nil).
func timestamptzPtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// pgUUID embrulha um uuid.UUID como pgtype.UUID válido (para ref_id de eventos).
func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte(id), Valid: true}
}
