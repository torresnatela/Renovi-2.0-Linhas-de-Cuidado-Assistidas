// Package agenda é o Adapter do MySQL legado da Renovi — a verdade da escala, de
// quem atende o quê e de quais horários existem (ADR-004).
//
// Este banco NÃO é nosso: não temos migration nele, não escolhemos o schema dele
// e outro aplicativo escreve nele o tempo todo. Três consequências moldam este
// pacote:
//
//  1. ESCRITA MÍNIMA. A única coisa que escrevemos é `tb_slots.booked`. A
//     consulta em si vive no nosso Postgres. Isso não é disciplina, é permissão:
//     o usuário do banco só tem SELECT + UPDATE(booked, updatedAt) em tb_slots
//     (ver deploy/mysql-legacy/init.sql), então um INSERT em tb_appointments
//     recebe "command denied".
//
//  2. A TRAVA É COMPORTAMENTAL. Não há unique nem FK ligando consulta a slot:
//     `booked` é um flag solto. O que segura é o app legado virar booked=1 ao
//     agendar — medido na HML: 84 das 85 consultas ativas. Por isso reservamos
//     com CAS (UPDATE ... WHERE booked = 0) e conferimos RowsAffected: uma ida ao
//     banco, atômica sob InnoDB, sem janela entre ler e escrever. Um
//     SELECT..FOR UPDATE faria duas idas e seguraria lock enquanto decidíamos.
//
//  3. FUSO. As colunas são DATETIME, que o MySQL guarda LITERAL (ao contrário de
//     TIMESTAMP, que o servidor converte). Não há fuso gravado: é hora de parede
//     de São Paulo. Quem resolve para instante é este adapter — e ele se recusa a
//     subir se o DSN discordar disso.
package agenda

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// legacyTZ é propriedade do DADO, não do ambiente: as colunas datetime do legado
// são ingênuas e sempre significam horário de São Paulo. Deixar isto virar
// variável de ambiente convidaria alguém a "consertar" um bug de fuso mudando a
// config — e o dado continuaria querendo dizer o que sempre quis.
const legacyTZ = "America/Sao_Paulo"

// Erros do adapter. Use errors.Is.
var (
	// ErrSlotTaken: o horário já estava reservado quando tentamos. Pode ter sido
	// o app legado ou outro paciente nosso. Não é falha: é corrida perdida.
	ErrSlotTaken = errors.New("agenda: horário já reservado")
	// ErrSlotNotFound: o horário não existe (ou o profissional dele não atende a
	// especialidade pedida).
	ErrSlotNotFound = errors.New("agenda: horário não encontrado")
	// ErrSpecialtyMismatch: o slot existe, mas o profissional não atende aquela
	// especialidade. Separado do NotFound porque a resposta HTTP é outra (400, e
	// não 404) e o front pode corrigir a escolha.
	ErrSpecialtyMismatch = errors.New("agenda: profissional não atende esta especialidade")
	// ErrSpecialtyInactive: o profissional atende a especialidade, mas ela foi
	// desativada. Distinto do mismatch: dizer "não atende" quando atende é falso.
	ErrSpecialtyInactive = errors.New("agenda: especialidade desativada")
	// ErrProfessionalNotFound: o profissional não existe (ou saiu do legado).
	ErrProfessionalNotFound = errors.New("agenda: profissional não encontrado")
	// ErrUnavailable: o legado não respondeu. Nunca confundir com "não há
	// horários" — ver a resposta LegacyUnavailable no openapi.yaml.
	ErrUnavailable = errors.New("agenda: legado indisponível")
)

// Specialty, Professional e Slot são os pedaços do legado que o produto usa.
// Enxutos de propósito: `tb_professionals` tem CPF e e-mail do médico, e o que
// não trazemos não vaza.
type (
	Specialty struct {
		ID   string
		Name string
	}

	Professional struct {
		ID             string
		FullName       string
		ImageURL       string // vazio quando não há
		LicenseNumber  string
		LicenseRegion  string
		LicenseCouncil string
		RQE            string // vazio quando não há
	}

	Slot struct {
		ID       string
		StartsAt time.Time
		EndsAt   time.Time
	}
)

// Booking é tudo o que a saga precisa saber para agendar, numa consulta só.
//
// Existe porque as três coisas são lidas juntas e conferidas juntas: o slot
// (existe? é futuro?), o profissional (quem é o MMD na DAV, e o nome que a
// consulta vai exibir) e a especialidade (que o slot NÃO determina — o vínculo é
// muitos-para-muitos e há profissional com três).
type Booking struct {
	Slot         Slot
	Professional Professional
	Specialty    Specialty
	Booked       bool
}

// Config parametriza o Client.
type Config struct {
	DSN string
	// MaxOpenConns limita o quanto sobrecarregamos um banco de PRODUÇÃO de
	// terceiro. Default conservador: 5.
	MaxOpenConns int
	Timeout      time.Duration
	Logger       *slog.Logger
}

// Client fala com o MySQL legado.
type Client struct {
	db     *sql.DB
	loc    *time.Location
	logger *slog.Logger
}

// New abre o pool e FORÇA as duas opções de DSN que não são opinião.
//
// Não confiamos na string que o operador escreveu. Sem `parseTime=true` o driver
// nem consegue ler DATETIME em time.Time; com parseTime mas `loc` no default
// (UTC), 09:00 de São Paulo é lido como 09:00Z — e toda consulta acontece 3h fora
// do horário, em silêncio, sem erro nenhum. É o tipo de bug que só aparece com um
// paciente perdendo a consulta.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.DSN) == "" {
		return nil, errors.New("agenda: DSN é obrigatório")
	}

	parsed, err := mysql.ParseDSN(cfg.DSN)
	if err != nil {
		// Sem %w: o DSN tem a senha do banco dentro.
		return nil, errors.New("agenda: DSN inválido")
	}

	loc, err := time.LoadLocation(legacyTZ)
	if err != nil {
		// Acontece em imagem sem tzdata (scratch/distroless). Por isso cmd/api e
		// cmd/worker importam _ "time/tzdata".
		return nil, fmt.Errorf("agenda: não consegui carregar %s (falta tzdata no binário?): %w", legacyTZ, err)
	}

	// Recusa no boot em vez de ler todo horário errado por 3h. Se alguém pôs um
	// `loc` explícito e diferente no DSN, é engano ou é uma decisão que precisa
	// passar por aqui.
	if parsed.Loc != nil && parsed.Loc != time.UTC && parsed.Loc.String() != legacyTZ {
		return nil, fmt.Errorf("agenda: DSN pede loc=%s, mas as datas do legado são %s",
			parsed.Loc, legacyTZ)
	}
	parsed.ParseTime = true
	parsed.Loc = loc

	// Timeout de rede no próprio DSN, como backstop. Sem isto, um caller sem
	// deadline no contexto (o worker rodando ReleaseSlot, por exemplo) trava numa
	// conexão emperrada do banco de terceiro até o TCP desistir. O ctx do request
	// HTTP continua sendo o limite fino; isto é a rede embaixo dele.
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	parsed.Timeout = timeout      // dial
	parsed.ReadTimeout = timeout  // leitura
	parsed.WriteTimeout = timeout // escrita

	db, err := sql.Open("mysql", parsed.FormatDSN())
	if err != nil {
		return nil, errors.New("agenda: não consegui abrir o pool")
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 5
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(2)
	// MySQL e proxies derrubam conexão ociosa por conta própria; sem um teto de
	// vida, o pool serve conexão morta e o erro vira "invalid connection"
	// intermitente, do tipo que ninguém reproduz.
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(time.Minute)

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{db: db, loc: loc, logger: logger}, nil
}

func (c *Client) Close() error { return c.db.Close() }

// Ping serve ao /readyz.
func (c *Client) Ping(ctx context.Context) error {
	if err := c.db.PingContext(ctx); err != nil {
		// Loga a causa (como fail) antes de esconder: um flap no /readyz sem log
		// não deixa nada para diagnosticar.
		c.logger.Error("agenda: ping falhou", "error", err.Error())
		return fmt.Errorf("%w: ping", ErrUnavailable)
	}
	return nil
}

// Location é o fuso da agenda. O controller o devolve em `time_zone` para que o
// front exiba a hora no fuso do profissional, e não no do browser.
func (c *Client) Location() *time.Location { return c.loc }

// ---------------------------------------------------------------------------
// Leitura
// ---------------------------------------------------------------------------

// ListSpecialties devolve as especialidades ATIVAS que têm algum profissional com
// horário livre no futuro.
//
// O filtro por horário livre não é enfeite: especialidade que leva a uma lista
// vazia de profissionais é beco sem saída, e o paciente não tem como saber que o
// problema não é dele.
func (c *Client) ListSpecialties(ctx context.Context, now time.Time) ([]Specialty, error) {
	const q = `
		SELECT DISTINCT sp.id, sp.name
		FROM tb_specialities sp
		JOIN tb_professionals_specialities ps ON ps.specialityId = sp.id
		JOIN tb_shifts sh ON sh.professionalId = ps.professionalId
		JOIN tb_slots sl ON sl.shiftId = sh.id
		WHERE sp.active = 1 AND sl.booked = 0 AND sl.startsAt > ?
		ORDER BY sp.name`

	rows, err := c.db.QueryContext(ctx, q, c.wall(now))
	if err != nil {
		return nil, c.fail("ListSpecialties", err)
	}
	defer rows.Close()

	var out []Specialty
	for rows.Next() {
		var s Specialty
		if err := rows.Scan(&s.ID, &s.Name); err != nil {
			return nil, c.fail("ListSpecialties/scan", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, c.fail("ListSpecialties/rows", err)
	}
	return out, nil
}

// ListProfessionalsBySpecialty devolve quem atende a especialidade E tem horário
// livre futuro.
func (c *Client) ListProfessionalsBySpecialty(ctx context.Context, specialtyID string, now time.Time) ([]Professional, error) {
	const q = `
		SELECT DISTINCT p.id, p.firstName, p.lastName, p.imageUrl,
		       p.licenseNumber, p.licenseRegion, p.licenseCouncil, p.rqe
		FROM tb_professionals p
		JOIN tb_professionals_specialities ps ON ps.professionalId = p.id
		JOIN tb_shifts sh ON sh.professionalId = p.id
		JOIN tb_slots sl ON sl.shiftId = sh.id
		WHERE ps.specialityId = ? AND sl.booked = 0 AND sl.startsAt > ?
		ORDER BY p.firstName, p.lastName`

	rows, err := c.db.QueryContext(ctx, q, specialtyID, c.wall(now))
	if err != nil {
		return nil, c.fail("ListProfessionalsBySpecialty", err)
	}
	defer rows.Close()

	var out []Professional
	for rows.Next() {
		p, err := scanProfessional(rows)
		if err != nil {
			return nil, c.fail("ListProfessionalsBySpecialty/scan", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, c.fail("ListProfessionalsBySpecialty/rows", err)
	}
	return out, nil
}

// ListSlots devolve os horários livres do profissional entre `from` e `to`.
//
// `from` nunca é anterior a `now`: horário no passado não serve para nada e a
// própria DAV o recusa (422 — achado #18). O corte é feito aqui e não na tela
// porque o relógio que vale é o do servidor.
func (c *Client) ListSlots(ctx context.Context, professionalID string, from, to, now time.Time) ([]Slot, error) {
	if from.Before(now) {
		from = now
	}
	if !to.After(from) {
		return nil, nil
	}

	// `startsAt > ?` (estrito), não `>=`, para casar com o "future" das listas
	// (ListSpecialties/ListProfessionals) e com o `StartsAt.After(now)` do
	// loadBooking: um horário que começa EXATAMENTE agora não é agendável (o
	// loadBooking o recusa como expirado), então oferecê-lo aqui seria mostrar algo
	// que falha ao clicar. Slots reais são de 25 min, então a borda de segundo
	// exato é teórica — mas o operador tem que ser o mesmo nos três lugares.
	const q = `
		SELECT sl.id, sl.startsAt, sl.endsAt
		FROM tb_slots sl
		JOIN tb_shifts sh ON sh.id = sl.shiftId
		WHERE sh.professionalId = ? AND sl.booked = 0
		  AND sl.startsAt > ? AND sl.startsAt < ?
		ORDER BY sl.startsAt`

	rows, err := c.db.QueryContext(ctx, q, professionalID, c.wall(from), c.wall(to))
	if err != nil {
		return nil, c.fail("ListSlots", err)
	}
	defer rows.Close()

	var out []Slot
	for rows.Next() {
		var s Slot
		var start, end time.Time
		if err := rows.Scan(&s.ID, &start, &end); err != nil {
			return nil, c.fail("ListSlots/scan", err)
		}
		s.StartsAt, s.EndsAt = c.instant(start), c.instant(end)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, c.fail("ListSlots/rows", err)
	}
	return out, nil
}

// GetProfessional busca um profissional pelo id.
//
// Existe porque a tela de horários é "os horários da Ana": o nome e o registro
// vêm junto com os slots. Separá-los em outra rota acrescentaria um segundo
// estado de carregando e um segundo modo de falhar, para um dado que já está aqui.
func (c *Client) GetProfessional(ctx context.Context, id string) (Professional, error) {
	const q = `
		SELECT p.id, p.firstName, p.lastName, p.imageUrl,
		       p.licenseNumber, p.licenseRegion, p.licenseCouncil, p.rqe
		FROM tb_professionals p
		WHERE p.id = ?`

	p, err := scanProfessional(c.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Professional{}, ErrProfessionalNotFound
	}
	if err != nil {
		return Professional{}, c.fail("GetProfessional", err)
	}
	return p, nil
}

// LoadBooking resolve, numa consulta só, tudo o que a saga precisa: o slot, o
// profissional dele (que é o MMD na DAV) e a especialidade pedida.
//
// Devolve ErrSpecialtyMismatch quando o slot existe mas o profissional não atende
// aquela especialidade — distinguir isso de "não existe" custa uma query a mais
// SÓ no caminho de erro, e a diferença importa: um é 404, o outro é 400 e o
// paciente consegue corrigir.
func (c *Client) LoadBooking(ctx context.Context, slotID, specialtyID string) (Booking, error) {
	const q = `
		SELECT sl.id, sl.startsAt, sl.endsAt, sl.booked,
		       p.id, p.firstName, p.lastName, p.imageUrl,
		       p.licenseNumber, p.licenseRegion, p.licenseCouncil, p.rqe,
		       sp.id, sp.name
		FROM tb_slots sl
		JOIN tb_shifts sh ON sh.id = sl.shiftId
		JOIN tb_professionals p ON p.id = sh.professionalId
		JOIN tb_professionals_specialities ps ON ps.professionalId = p.id
		JOIN tb_specialities sp ON sp.id = ps.specialityId
		WHERE sl.id = ? AND sp.id = ? AND sp.active = 1`

	var (
		b                   Booking
		start, end          time.Time
		imageURL, rqe       sql.NullString
		firstName, lastName string
	)
	err := c.db.QueryRowContext(ctx, q, slotID, specialtyID).Scan(
		&b.Slot.ID, &start, &end, &b.Booked,
		&b.Professional.ID, &firstName, &lastName, &imageURL,
		&b.Professional.LicenseNumber, &b.Professional.LicenseRegion,
		&b.Professional.LicenseCouncil, &rqe,
		&b.Specialty.ID, &b.Specialty.Name,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Booking{}, c.explainMissing(ctx, slotID, specialtyID)
	}
	if err != nil {
		return Booking{}, c.fail("LoadBooking", err)
	}

	b.Slot.StartsAt, b.Slot.EndsAt = c.instant(start), c.instant(end)
	b.Professional.FullName = strings.TrimSpace(firstName + " " + lastName)
	b.Professional.ImageURL = imageURL.String
	b.Professional.RQE = rqe.String
	return b, nil
}

// explainMissing separa os três motivos de LoadBooking não achar linha: o slot
// não existe (404), o profissional dele não atende a especialidade (400
// "não atende"), ou atende mas a especialidade está desativada (400, frase
// diferente — dizer "não atende" seria mentira). Só roda no caminho de erro.
func (c *Client) explainMissing(ctx context.Context, slotID, specialtyID string) error {
	// O profissional do slot atende essa especialidade, ignorando o active? Um
	// join do slot até a especialidade SEM o filtro `active = 1`. Se casar, o
	// motivo é a desativação; se não, é mismatch de verdade.
	var active bool
	err := c.db.QueryRowContext(ctx, `
		SELECT sp.active
		FROM tb_slots sl
		JOIN tb_shifts sh ON sh.id = sl.shiftId
		JOIN tb_professionals_specialities ps ON ps.professionalId = sh.professionalId
		JOIN tb_specialities sp ON sp.id = ps.specialityId
		WHERE sl.id = ? AND sp.id = ?`, slotID, specialtyID).Scan(&active)
	switch {
	case err == nil && !active:
		return ErrSpecialtyInactive
	case err == nil:
		// Casou e está ativa — não deveria cair aqui (a query principal teria
		// achado). Trata como mismatch por segurança.
		return ErrSpecialtyMismatch
	case !errors.Is(err, sql.ErrNoRows):
		return c.fail("explainMissing", err)
	}

	// Não casou: ou o slot não existe, ou o profissional não atende a especialidade.
	var exists bool
	err = c.db.QueryRowContext(ctx, `SELECT 1 FROM tb_slots WHERE id = ?`, slotID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSlotNotFound
	}
	if err != nil {
		return c.fail("explainMissing", err)
	}
	return ErrSpecialtyMismatch
}

// ---------------------------------------------------------------------------
// A única escrita
// ---------------------------------------------------------------------------

// BookSlot reserva o horário. É a trava de double-booking inteira do sistema.
//
// Compare-and-set numa instrução: o `WHERE booked = 0` faz o InnoDB travar a
// linha, conferir e escrever atomicamente. Se outra transação (nossa ou do app
// legado) chegou antes, o UPDATE simplesmente não casa e RowsAffected é 0 — sem
// janela entre ler e decidir, e sem lock aberto enquanto pensamos.
//
// A DAV NÃO ajuda aqui: ela aceita dois appointments no mesmo horário para o
// mesmo profissional (achado #17). Se este CAS falhar, não há segunda rede.
func (c *Client) BookSlot(ctx context.Context, slotID string) error {
	const q = `UPDATE tb_slots SET booked = 1, updatedAt = NOW() WHERE id = ? AND booked = 0`

	res, err := c.db.ExecContext(ctx, q, slotID)
	if err != nil {
		return c.fail("BookSlot", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return c.fail("BookSlot/rowsAffected", err)
	}
	if n == 1 {
		return nil
	}

	// 0 linhas: ou o slot sumiu, ou alguém chegou antes. Distinguir importa
	// porque a resposta ao paciente é diferente (404 vs "escolha outro horário").
	// Basta a EXISTÊNCIA da linha: o CAS falhou com `booked=0` no WHERE, então se a
	// linha existe ela só pode estar com booked=1 (ou acabou de ser liberada — aí
	// o retry do paciente resolve). Não precisamos ler o valor.
	var exists bool
	switch err := c.db.QueryRowContext(ctx, `SELECT 1 FROM tb_slots WHERE id = ?`, slotID).Scan(&exists); {
	case errors.Is(err, sql.ErrNoRows):
		return ErrSlotNotFound
	case err != nil:
		return c.fail("BookSlot/explain", err)
	default:
		return ErrSlotTaken
	}
}

// ReleaseSlot devolve o horário ao mercado. É a compensação da saga.
//
// Idempotente de propósito: soltar um slot já solto é sucesso, não erro. Quem
// chama é um worker que pode repetir depois de um crash, e um erro aqui só o
// faria repetir para sempre.
//
// ATENÇÃO: só chame quando tiver CERTEZA de que a consulta não existe na DAV. Se
// o resultado for desconhecido, o slot FICA reservado — devolvê-lo entregaria
// dois pacientes ao mesmo profissional no mesmo horário. Perder um horário é
// problema operacional; double-booking é problema clínico.
func (c *Client) ReleaseSlot(ctx context.Context, slotID string) error {
	const q = `UPDATE tb_slots SET booked = 0, updatedAt = NOW() WHERE id = ? AND booked = 1`

	if _, err := c.db.ExecContext(ctx, q, slotID); err != nil {
		return c.fail("ReleaseSlot", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Fuso e erros
// ---------------------------------------------------------------------------

// wall converte um instante na hora de parede de São Paulo que o legado usa nas
// comparações. Nunca use NOW() do MySQL num WHERE: ele usa o fuso do SERVIDOR, e
// o servidor é de terceiro.
func (c *Client) wall(t time.Time) time.Time { return t.In(c.loc) }

// instant faz o contrário: o driver já devolve o DATETIME em c.loc, então isto é
// só a afirmação explícita de que o valor lido é um instante, e não hora solta.
func (c *Client) instant(t time.Time) time.Time { return t.In(c.loc) }

// fail loga e embrulha. O erro do driver pode trazer a query e o DSN, então só o
// rótulo da operação vai para o log — nunca %w do erro cru para fora.
func (c *Client) fail(op string, err error) error {
	c.logger.Error("agenda: falha no legado", "op", op, "error", err.Error())
	return fmt.Errorf("%w: %s", ErrUnavailable, op)
}

type scanner interface{ Scan(dest ...any) error }

func scanProfessional(s scanner) (Professional, error) {
	var (
		p                   Professional
		firstName, lastName string
		imageURL, rqe       sql.NullString
	)
	if err := s.Scan(&p.ID, &firstName, &lastName, &imageURL,
		&p.LicenseNumber, &p.LicenseRegion, &p.LicenseCouncil, &rqe); err != nil {
		return Professional{}, err
	}
	p.FullName = strings.TrimSpace(firstName + " " + lastName)
	p.ImageURL = imageURL.String
	p.RQE = rqe.String
	return p, nil
}
