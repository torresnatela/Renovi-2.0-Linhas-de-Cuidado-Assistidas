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

/** Erro da API no formato RFC 7807 (application/problem+json). */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly title: string,
    readonly detail?: string,
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
    throw new ApiError(res.status, problem?.title ?? 'erro', problem?.detail);
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
