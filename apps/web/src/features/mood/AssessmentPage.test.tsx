import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { AssessmentPage } from './AssessmentPage';

vi.mock('../../shared/api', async () => {
  const actual = await vi.importActual<typeof import('../../shared/api')>('../../shared/api');
  return { ...actual, getAssessmentAvailability: vi.fn(), submitAssessment: vi.fn() };
});
const api = await import('../../shared/api');

function renderAt(path: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/avaliacoes/:codigo" element={<AssessmentPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('AssessmentPage', () => {
  beforeEach(() => {
    vi.mocked(api.getAssessmentAvailability).mockResolvedValue({
      codigo: 'WHO5',
      eligibility: { allowed: true, blocks: [] },
      item_count: 5,
      value_min: 0,
      value_max: 5,
    });
  });

  it('avisa e oferece volta quando o código é inválido', () => {
    renderAt('/avaliacoes/XPTO');
    expect(screen.getByText('Avaliação não encontrada')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /voltar/i })).toHaveAttribute('href', '/jornada');
    // Não renderiza o formulário de instrumento.
    expect(screen.queryByText(/Nas últimas duas semanas/)).not.toBeInTheDocument();
  });

  it('renderiza o formulário do WHO-5 quando o código é válido', async () => {
    renderAt('/avaliacoes/WHO5');
    expect(await screen.findByText(/Nas últimas duas semanas/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /voltar/i })).toHaveAttribute('href', '/jornada');
  });
});
