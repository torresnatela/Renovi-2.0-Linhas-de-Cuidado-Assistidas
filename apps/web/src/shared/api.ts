// Cliente HTTP mínimo. Na fase MVP, este arquivo é substituído pelos hooks
// gerados pelo orval a partir do OpenAPI (ver packages/api-client).
const BASE = import.meta.env.VITE_API_BASE_URL ?? '';

export interface HealthStatus {
  status: string;
  service: string;
  version: string;
}

export interface Account {
  id: string;
  full_name: string;
  email: string;
}

export interface Address {
  zip_code: string;
  street: string;
  number: string;
  complement?: string;
  neighborhood: string;
  city: string;
  state: string;
}

export interface RegisterRequest {
  full_name: string;
  cpf: string;
  birth_date: string;
  email: string;
  phone: string;
  password: string;
  address: Address;
}

/**
 * Motivo máquina-legível de um erro ou de um veredito (RFC 7807 §3.2).
 *
 * Existe porque status HTTP não basta: "cedo demais" e "horário tomado" são os
 * dois 409, e a tela reage diferente a cada um. Casar pelo `detail` seria casar
 * por frase escrita para humano — que muda no dia em que alguém melhorar o texto.
 */
export interface Reason {
  code: string;
  detail?: string;
}

/** Erro da API no formato RFC 7807 (application/problem+json). */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly title: string,
    readonly detail?: string,
    /** Presente quando a API quer que o cliente DECIDA algo, e não só exiba. */
    readonly reason?: Reason,
    /**
     * Os bloqueios do motor de elegibilidade — presentes SÓ no 422 de agendamento
     * de linha (ELIGIBILITY_BLOCKED). Cada um traz uma frase JÁ pronta (`reason`) e,
     * quando o desbloqueio depende do relógio, o `available_from`. É extensão da
     * RFC 7807 (§3.2), espelhada aqui como o `reason` já é.
     */
    readonly blocks?: EligibilityBlock[],
  ) {
    // `detail` já é a frase pronta para o usuário — a API a escreve pensando
    // nele, e é ela que distingue "e-mail já em uso" de "dados inválidos".
    super(detail || title);
    this.name = 'ApiError';
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}/api/v1${path}`, {
    ...init,
    // A sessão é um cookie httpOnly: o JS não o lê nem escreve, só pede ao
    // browser que o mande. É isso que a torna imune a roubo por XSS — e é por
    // isso que não existe nenhum token guardado neste código.
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });

  if (!res.ok) {
    const problem = await res.json().catch(() => null);
    throw new ApiError(
      res.status,
      problem?.title ?? 'erro',
      problem?.detail,
      problem?.reason,
      problem?.blocks,
    );
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export async function getHealth(): Promise<HealthStatus> {
  const res = await fetch(`${BASE}/api/v1/healthz`);
  if (!res.ok) {
    throw new Error(`health check falhou: ${res.status}`);
  }
  return res.json() as Promise<HealthStatus>;
}

export const registerAccount = (body: RegisterRequest) =>
  request<Account>('/auth/register', { method: 'POST', body: JSON.stringify(body) });

export const login = (cpf: string, password: string) =>
  request<Account>('/auth/login', { method: 'POST', body: JSON.stringify({ cpf, password }) });

export const logout = () => request<void>('/auth/logout', { method: 'POST' });

export const getMe = () => request<Account>('/me');

// ---------------------------------------------------------------------------
// Agendamento
// ---------------------------------------------------------------------------

export interface Specialty {
  id: string;
  name: string;
}

export interface ProfessionalLicense {
  council: string;
  number: string;
  region: string;
  rqe: string | null;
}

export interface Professional {
  id: string;
  full_name: string;
  image_url: string | null;
  license: ProfessionalLicense;
}

export interface Slot {
  id: string;
  /** RFC 3339 COM offset. Nunca construa Date a partir de string sem offset. */
  starts_at: string;
  ends_at: string;
  /**
   * Fuso IANA em que este horário deve ser EXIBIDO — o da agenda.
   *
   * Não é redundante com o offset: o offset é propriedade de um instante, o fuso
   * é a regra. A consulta é às 09:00 em São Paulo, e é 09:00 que o paciente
   * precisa ler mesmo que o relógio dele esteja em outro lugar. Ver shared/datetime.
   */
  time_zone: string;
}

export interface SlotPage {
  professional: Professional;
  from: string;
  to: string;
  items: Slot[];
}

/** O profissional como a CONSULTA o guarda: fotografia, sem o registro no conselho. */
export interface AppointmentProfessional {
  id: string;
  full_name: string;
}

export type JoinStatus = 'OPEN' | 'TOO_EARLY' | 'TOO_LATE' | 'UNAVAILABLE';

/**
 * O ESTADO da janela de entrada — nunca a url.
 *
 * Repare que "30 minutos" não existe neste arquivo nem em lugar nenhum do front:
 * o que chega é `opens_at`, JÁ calculado pelo servidor. É o que permite ao produto
 * mudar a antecedência sem deploy daqui — e impede que um relógio errado no
 * cliente abra a sala mais cedo.
 */
export interface JoinWindow {
  status: JoinStatus;
  opens_at: string;
  closes_at: string;
  reason?: Reason;
}

export type AppointmentStatus = 'PROCESSING' | 'CONFIRMED' | 'UNCONFIRMED' | 'CANCELLED';

export interface Appointment {
  id: string;
  status: AppointmentStatus;
  starts_at: string;
  ends_at: string;
  time_zone: string;
  specialty: Specialty;
  professional: AppointmentProfessional;
  join: JoinWindow;
  created_at?: string;
}

/** O link da sala. É CAPACIDADE, não dado: não guarde, não logue, não compartilhe. */
export interface JoinTicket {
  url: string;
}

interface ListOf<T> {
  items: T[];
}

export const listSpecialties = () =>
  request<ListOf<Specialty>>('/specialties').then((r) => r.items);

export const listProfessionals = (specialtyId: string) =>
  request<ListOf<Professional>>(
    `/specialties/${encodeURIComponent(specialtyId)}/professionals`,
  ).then((r) => r.items);

export const listSlots = (professionalId: string) =>
  request<SlotPage>(`/professionals/${encodeURIComponent(professionalId)}/slots`);

export const listAppointments = () =>
  request<ListOf<Appointment>>('/appointments').then((r) => r.items);

export const getAppointment = (id: string) =>
  request<Appointment>(`/appointments/${encodeURIComponent(id)}`);

export const createAppointment = (body: { slot_id: string; specialty_id: string }) =>
  request<Appointment>('/appointments', { method: 'POST', body: JSON.stringify(body) });

/**
 * Pede o link da sala. É POST porque não é leitura pura: o acesso é registrado
 * do lado do servidor, e POST não entra em cache de proxy, histórico do browser
 * nem prefetch de link — três lugares onde o link de uma teleconsulta não deveria
 * parar.
 */
export const joinAppointment = (id: string) =>
  request<JoinTicket>(`/appointments/${encodeURIComponent(id)}/join`, { method: 'POST' });

// ---------------------------------------------------------------------------
// Verificador Diário de Humor (Anexo C)
// ---------------------------------------------------------------------------

/** Status do consentimento (LGPD) do paciente para uma finalidade. */
export interface ConsentStatus {
  finalidade: string;
  active: boolean;
  versao_termo?: string | null;
  concedido_em?: string | null;
}

/** Um check-in de humor (execução do anel diário). */
export interface MoodCheckin {
  valencia: number;
  energia: number;
  /** Quadrante derivado determinístico — o front NÃO recalcula, exibe. */
  quadrante: string;
  emotion_label?: string | null;
  context_tags?: string[] | null;
  respondido_em: string;
}

export type MoodReason = 'consent_required' | 'not_enrolled';

export type AssessmentCode = 'WHO5' | 'PHQ4';

/** O check-in de hoje (ou nulo) e a elegibilidade do paciente. */
export interface MoodToday {
  dia: string;
  can_checkin: boolean;
  reason?: MoodReason | null;
  checkin?: MoodCheckin | null;
  /** Instrumento de aprofundamento ofertado agora pelo gatilho (ou nulo). */
  offer?: AssessmentCode | null;
  /** true quando o gatilho indica escalonamento à trilha clínica. */
  escalate?: boolean;
}

// ---------------------------------------------------------------------------
// Jornada (linha de cuidado) — rotas /me/*
// ---------------------------------------------------------------------------

/**
 * Qual regra barrou o item. VIGENCIA é da MATRÍCULA (fora do período de
 * validade); as outras quatro são regras do template. O front reage por CÓDIGO
 * (não por texto) quando quer — o `reason` do bloco já é a frase pronta.
 */
export type EligibilityRuleType = 'VIGENCIA' | 'QUOTA' | 'MIN_INTERVAL' | 'MAX_ADVANCE' | 'PREREQUISITE';

/**
 * Um motivo, JÁ PRONTO PARA EXIBIR, de o motor ter barrado um item.
 *
 * `reason` é escrito pelo servidor pensando no paciente — NÃO se re-traduz aqui
 * (ao contrário do `Reason.code` dos erros). `available_from` só vem quando o
 * desbloqueio depende do relógio (cota, intervalo, antecedência); um pré-requisito
 * não tem data.
 */
export interface EligibilityBlock {
  rule_type: EligibilityRuleType;
  reason: string;
  available_from?: string;
}

/** O veredito do motor para UM item: `allowed`, e o porquê do não em `blocks`. */
export interface Eligibility {
  allowed: boolean;
  blocks: EligibilityBlock[];
}

/** Disponibilidade de um instrumento periódico + seu descritor. */
export interface AssessmentAvailability {
  codigo: string;
  eligibility: Eligibility;
  item_count: number;
  value_min: number;
  value_max: number;
}

/** Resultado pontuado de um instrumento periódico. */
export interface AssessmentResult {
  codigo: string;
  raw_score?: number;
  index_score?: number | null;
  subscores?: Record<string, number> | null;
  faixa: string;
  flag_encaminhar: boolean;
  respondido_em: string;
}

/** Canal de urgência/care navigation (triagem, não tratamento). */
export interface HelpChannel {
  type: string;
  label: string;
  message: string;
}

export const getAssessmentAvailability = (codigo: string) =>
  request<AssessmentAvailability>(`/me/assessments/${encodeURIComponent(codigo)}`);

export const submitAssessment = (codigo: AssessmentCode, items: number[]) =>
  request<AssessmentResult>('/me/assessments', {
    method: 'POST',
    body: JSON.stringify({ codigo, items }),
  });

export const moodHelpNow = () => request<HelpChannel>('/me/mood/help-now', { method: 'POST' });

export interface InstrumentDimension {
  dimensao: string;
  polaridade: string;
  min_score: number;
  max_score: number;
}
export interface EmotionLabel {
  quadrante: string;
  rotulo: string;
}
export interface ContextTag {
  chave: string;
  rotulo: string;
}
export interface InstrumentConfig {
  codigo: string;
  versao: string;
  anel: string;
  dimensions: InstrumentDimension[];
  emotion_labels: EmotionLabel[];
  context_tags: ContextTag[];
}

export const CHECKIN_FINALIDADE = 'checkin_humor';

export const getConsent = (finalidade = CHECKIN_FINALIDADE) =>
  request<ConsentStatus>(`/me/consent?finalidade=${encodeURIComponent(finalidade)}`);

export const grantConsent = (versaoTermo: string, finalidade = CHECKIN_FINALIDADE) =>
  request<ConsentStatus>('/me/consent', {
    method: 'POST',
    body: JSON.stringify({ finalidade, versao_termo: versaoTermo }),
  });

export const getMoodInstrument = (codigo: string) =>
  request<InstrumentConfig>(`/me/mood/instruments/${encodeURIComponent(codigo)}`);

export const getMoodToday = () => request<MoodToday>('/me/mood/today');

export const recordMoodCheckin = (body: {
  valencia: number;
  energia: number;
  emotion_label?: string;
  context_tags?: string[];
}) => request<MoodCheckin>('/me/mood/checkin', { method: 'POST', body: JSON.stringify(body) });

export const getMoodHistory = (limit?: number) =>
  request<MoodCheckin[]>(
    `/me/mood/history${limit != null ? `?limit=${encodeURIComponent(limit)}` : ''}`,
  );

/**
 * Um passo da linha (no Slice 1, sempre uma CONSULTA). `ref` é o código estável
 * dentro da linha (o que os pré-requisitos referenciam), distinto do `id` (UUID).
 */
export interface CareLineItemInfo {
  id: string;
  ref: string;
  kind: 'CONSULTA';
  specialty_code: string;
  label: string;
  /** Cadência em texto livre, só informativa. As regras que valem vêm no motor. */
  recurrence?: string | null;
  sort_order: number;
}

export interface JourneyItem {
  item: CareLineItemInfo;
  eligibility: Eligibility;
}

export interface EnrollmentPeriod {
  id: string;
  starts_at: string;
  ends_at: string;
  source: 'admin';
}

export type EnrollmentStatus = 'ativa' | 'pausada' | 'concluida' | 'encerrada' | 'expirada';

/** O vínculo do paciente a uma versão publicada da linha. */
export interface Enrollment {
  id: string;
  care_line_code: string;
  care_line_version: number;
  status: EnrollmentStatus;
  valid_from: string;
  valid_until: string;
  periods: EnrollmentPeriod[];
}

export type JourneyEventType =
  | 'matricula_criada'
  | 'matricula_renovada'
  | 'matricula_expirada'
  | 'matricula_encerrada'
  | 'consulta_agendada'
  | 'consulta_cancelada'
  | 'consulta_status_forcado';

export type JourneyActor = 'paciente' | 'sistema' | 'admin';

/** Um fato da jornada (event log append-only). `payload` varia por tipo. */
export interface JourneyEvent {
  id: string;
  event_type: JourneyEventType;
  actor: JourneyActor;
  occurred_at: string;
  payload: Record<string, unknown>;
}

/** Uma matrícula pronta para a tela: matrícula + nome + itens avaliados + eventos. */
export interface JourneyEnrollment {
  enrollment: Enrollment;
  care_line_name: string;
  items: JourneyItem[];
  recent_events: JourneyEvent[];
}

export interface Journey {
  enrollments: JourneyEnrollment[];
}

export type CareAppointmentStatus =
  | 'agendada'
  | 'confirmada'
  | 'em_andamento'
  | 'realizada'
  | 'falta'
  | 'cancelada';

/**
 * A consulta de um item da linha, na visão da JORNADA — distinta de `Appointment`
 * (o booking). `booking_id` é a ponte: é o `id` usado em /appointments/{id} e
 * .../join. `time_zone` segue a mesma regra do Slot (exibir no fuso da agenda).
 */
export interface CareAppointment {
  id: string;
  item_ref: string;
  label: string;
  status: CareAppointmentStatus;
  scheduled_at: string;
  time_zone: string;
  cancelled_at?: string;
  booking_id: string;
}

/**
 * Um horário livre JÁ com o profissional e o veredito do motor embutidos. A
 * disponibilidade de uma LINHA agrega horários de VÁRIOS profissionais, por isso
 * o profissional vai por slot (diferente do SlotPage, de um profissional só).
 */
export interface AnnotatedSlot extends Slot {
  professional: AppointmentProfessional;
  eligibility: Eligibility;
}

export interface AvailabilityPage {
  item_id: string;
  from: string;
  to: string;
  items: AnnotatedSlot[];
}

/** Uma página do event log. `next_cursor` é OPACO; ausente = fim. */
export interface AuditPage {
  items: JourneyEvent[];
  next_cursor?: string;
}

export const getJourney = () => request<Journey>('/me/journey');

export const getEligibility = (itemId: string, date?: string) => {
  const q = new URLSearchParams({ item_id: itemId });
  if (date) q.set('date', date);
  return request<Eligibility>(`/me/eligibility?${q.toString()}`);
};

export const getAvailability = (itemId: string, from?: string, to?: string) => {
  const q = new URLSearchParams({ item_id: itemId });
  if (from) q.set('from', from);
  if (to) q.set('to', to);
  return request<AvailabilityPage>(`/me/availability?${q.toString()}`);
};

/**
 * Agenda a consulta de um item. É IDEMPOTENTE pelo header `Idempotency-Key`: a
 * MESMA key devolve a MESMA consulta sem criar outra. A key NÃO nasce aqui — ela
 * é a identidade da INTENÇÃO do paciente e chega pronta de quem chamou, para que
 * um retry da mesma intenção reuse a mesma key (ver useScheduleCare).
 */
export const createCareAppointment = (body: { item_id: string; slot_id: string }, idemKey: string) =>
  request<CareAppointment>('/me/appointments', {
    method: 'POST',
    body: JSON.stringify(body),
    headers: { 'Idempotency-Key': idemKey },
  });

export const cancelCareAppointment = (id: string) =>
  request<CareAppointment>(`/me/appointments/${encodeURIComponent(id)}/cancel`, { method: 'POST' });

export const listCareAppointments = (status?: string) => {
  const q = status ? `?${new URLSearchParams({ status }).toString()}` : '';
  return request<ListOf<CareAppointment>>(`/me/appointments${q}`).then((r) => r.items);
};

export const getAudit = (cursor?: string, limit?: number) => {
  const q = new URLSearchParams();
  if (cursor) q.set('cursor', cursor);
  if (limit) q.set('limit', String(limit));
  const qs = q.toString();
  return request<AuditPage>(`/me/audit${qs ? `?${qs}` : ''}`);
};
