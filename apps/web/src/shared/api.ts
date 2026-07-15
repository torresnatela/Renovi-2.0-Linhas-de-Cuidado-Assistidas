// Cliente HTTP mínimo. Na fase MVP, este arquivo é substituído pelos hooks
// gerados pelo orval a partir do OpenAPI (ver packages/api-client).
const BASE = import.meta.env.VITE_API_BASE_URL ?? '';

export interface HealthStatus {
  status: string;
  service: string;
  version: string;
}

export async function getHealth(): Promise<HealthStatus> {
  const res = await fetch(`${BASE}/api/v1/healthz`);
  if (!res.ok) {
    throw new Error(`health check falhou: ${res.status}`);
  }
  return res.json() as Promise<HealthStatus>;
}
