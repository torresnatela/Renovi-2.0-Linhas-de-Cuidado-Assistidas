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
    throw new ApiError(res.status, problem?.title ?? 'erro', problem?.detail, problem?.reason);
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
