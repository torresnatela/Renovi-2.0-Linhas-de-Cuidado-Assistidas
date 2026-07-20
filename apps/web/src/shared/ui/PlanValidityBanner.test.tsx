import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import type { Enrollment } from '../api';
import { PlanValidityBanner } from './PlanValidityBanner';

const enrollment: Enrollment = {
  id: 'enr-1',
  care_line_code: 'saude-mental',
  care_line_version: 1,
  status: 'ativa',
  valid_from: '2026-07-01T00:00:00-03:00',
  valid_until: '2026-09-30T00:00:00-03:00',
  periods: [],
};

const start = new Date(enrollment.valid_from).getTime();
const end = new Date(enrollment.valid_until).getTime();

afterEach(() => {
  vi.useRealTimers();
});

describe('PlanValidityBanner', () => {
  it('status ativa: badge "Plano ativo" com nome e vigência no fuso', () => {
    render(<PlanValidityBanner enrollment={enrollment} careLineName="Saúde Mental" />);
    expect(screen.getByText('Saúde Mental')).toBeInTheDocument();
    expect(screen.getByText('Plano ativo')).toBeInTheDocument();
    // 30/09/2026 no fuso da agenda (FUSO_PADRAO), não no do browser.
    expect(screen.getByText('30/09/2026')).toBeInTheDocument();
  });

  it('outro status: badge neutro com o status humanizado, sem "Plano ativo"', () => {
    render(
      <PlanValidityBanner
        enrollment={{ ...enrollment, status: 'pausada' }}
        careLineName="Saúde Mental"
      />,
    );
    expect(screen.getByText('Pausada')).toBeInTheDocument();
    expect(screen.queryByText('Plano ativo')).not.toBeInTheDocument();
  });

  it('a barra posiciona hoje entre início e fim (50% no meio)', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date((start + end) / 2));
    render(<PlanValidityBanner enrollment={enrollment} careLineName="Saúde Mental" />);
    expect(screen.getByTestId('validity-progress').style.width).toBe('50%');
  });

  it('a barra faz clamp em 0 antes do início e 100 depois do fim', () => {
    vi.useFakeTimers();

    vi.setSystemTime(new Date(start - 86_400_000));
    const antes = render(<PlanValidityBanner enrollment={enrollment} careLineName="X" />);
    expect(screen.getByTestId('validity-progress').style.width).toBe('0%');
    antes.unmount();

    vi.setSystemTime(new Date(end + 86_400_000));
    render(<PlanValidityBanner enrollment={enrollment} careLineName="X" />);
    expect(screen.getByTestId('validity-progress').style.width).toBe('100%');
  });

  /**
   * nearExpiry é aviso informativo, NÃO um funil de renovação: renovar é operação
   * administrativa (não existe endpoint de paciente), então NUNCA há botão de
   * renovar. E as consultas já marcadas não são afetadas — o texto reforça isso.
   */
  it('nearExpiry: variante accent com o aviso e SEM botão de renovar', () => {
    render(
      <PlanValidityBanner
        enrollment={{ ...enrollment, valid_until: '2026-07-30T00:00:00-03:00' }}
        careLineName="Saúde Mental"
        nearExpiry
      />,
    );
    expect(screen.getByText(/Seu plano vai até/)).toBeInTheDocument();
    expect(screen.getByText('30/07/2026')).toBeInTheDocument();
    expect(screen.getByText(/Suas consultas marcadas não são afetadas\./)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /renovar/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/renovar/i)).not.toBeInTheDocument();
  });
});
