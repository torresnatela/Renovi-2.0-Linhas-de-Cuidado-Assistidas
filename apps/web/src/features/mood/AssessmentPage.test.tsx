import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { mockViewport } from '../../shared/viewport.testkit';
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

  // Intenção migrada de MoodPage.test (aposentada na Etapa 8): responder o
  // instrumento até o resultado e ver o encaminhamento — em tom INFORMATIVO,
  // nunca vermelho de erro (o encaminhamento não é uma falha do paciente).
  it('envia o WHO-5, mostra o resultado e o encaminhamento sem tom de erro', async () => {
    vi.mocked(api.submitAssessment).mockResolvedValue({
      codigo: 'WHO5',
      index_score: 20,
      raw_score: 5,
      faixa: 'encaminha',
      flag_encaminhar: true,
      respondido_em: '2026-07-20T10:00:00-03:00',
    });

    const user = userEvent.setup();
    renderAt('/avaliacoes/WHO5');

    await screen.findByText(/Nas últimas duas semanas/);
    // Responde os 5 itens (o primeiro rótulo de cada: "Em nenhum momento").
    for (const botao of screen.getAllByRole('button', { name: /Em nenhum momento/i })) {
      await user.click(botao);
    }
    await user.click(screen.getByRole('button', { name: /Enviar respostas/i }));

    expect(await screen.findByText(/Índice de bem-estar:/)).toBeInTheDocument();
    expect(screen.getByText(/vale conversar com a equipe de cuidado/i)).toBeInTheDocument();
    // Encaminhamento é informativo, não erro: nenhum role=alert.
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  // Regra de ouro de UX do repo: nunca mostrar botão só desabilitado sem dizer
  // o motivo. Enquanto faltar responder algo, uma microcopy discreta explica —
  // ela some assim que o formulário fica completo.
  it('mostra a dica de formulário incompleto e a esconde quando todas as perguntas são respondidas', async () => {
    const user = userEvent.setup();
    renderAt('/avaliacoes/WHO5');

    await screen.findByText(/Nas últimas duas semanas/);
    const botaoEnviar = screen.getByRole('button', { name: /Enviar respostas/i });
    expect(botaoEnviar).toBeDisabled();
    expect(screen.getByText('Responda todas as perguntas para enviar.')).toBeInTheDocument();

    for (const botao of screen.getAllByRole('button', { name: /Em nenhum momento/i })) {
      await user.click(botao);
    }

    expect(botaoEnviar).toBeEnabled();
    expect(screen.queryByText('Responda todas as perguntas para enviar.')).not.toBeInTheDocument();
  });

  // --- Mobile: fluxo empilhado (Etapa 5) ---

  describe('no mobile (fluxo empilhado)', () => {
    let viewport: ReturnType<typeof mockViewport>;

    beforeEach(() => {
      viewport = mockViewport('mobile');
    });

    afterEach(() => {
      viewport.restore();
    });

    /**
     * O cabeçalho do card (h1 + "Voltar") some; o FlowHeader assume com o nome
     * REAL do instrumento (nada inventado) e volta para /jornada.
     */
    it('troca o cabeçalho do card pelo FlowHeader, com o nome do instrumento e voltar para /jornada', async () => {
      renderAt('/avaliacoes/WHO5');

      await screen.findByText(/Nas últimas duas semanas/);
      expect(screen.getByText('Índice de bem-estar (WHO-5)')).toBeInTheDocument();
      expect(screen.getByRole('link', { name: 'Voltar' })).toHaveAttribute('href', '/jornada');
      expect(screen.getByRole('button', { name: /Pedir ajuda/i })).toBeInTheDocument();
    });

    /**
     * Código inválido: sem instrumento para titular, o FlowHeader cai no mesmo
     * rótulo do eyebrow — nunca inventa um nome de instrumento que não existe.
     */
    it('código inválido: o FlowHeader não inventa um nome de instrumento', () => {
      renderAt('/avaliacoes/XPTO');

      expect(screen.getByRole('link', { name: 'Voltar' })).toHaveAttribute('href', '/jornada');
      expect(screen.getByText(/Este instrumento não existe/i)).toBeInTheDocument();
      expect(screen.queryByText('Avaliação não encontrada')).not.toBeInTheDocument();
    });

    // Alvo de toque ≥44px é regra de acessibilidade motora do DS, não estética.
    it('as opções do instrumento viram alvos de toque full-width ≥44px', async () => {
      renderAt('/avaliacoes/WHO5');

      const [opcao] = await screen.findAllByRole('button', { name: /Em nenhum momento/i });
      expect(opcao.className).toContain('min-h-[44px]');
      expect(opcao.className).toContain('w-full');
    });
  });
});
