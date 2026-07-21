import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { CareLineItemInfo, Eligibility } from '../api';
import { CareItemCard } from './CareItemCard';

const item: CareLineItemInfo = {
  id: 'item-1',
  ref: 'retorno',
  kind: 'CONSULTA',
  specialty_code: 'PSI',
  label: 'Consulta de retorno',
  recurrence: '1x por mês',
  sort_order: 2,
};

const liberado: Eligibility = { allowed: true, blocks: [] };
const bloqueado: Eligibility = {
  allowed: false,
  blocks: [
    {
      rule_type: 'MIN_INTERVAL',
      reason: 'Aguarde o intervalo mínimo entre consultas.',
      available_from: '2026-08-01T00:00:00-03:00',
    },
  ],
};

describe('CareItemCard', () => {
  it('mostra o título e a caption com a recorrência', () => {
    render(<CareItemCard item={item} eligibility={liberado} />);
    expect(screen.getByText('Consulta de retorno')).toBeInTheDocument();
    expect(screen.getByText('1x por mês')).toBeInTheDocument();
  });

  it('sem recorrência, a caption humaniza o kind', () => {
    render(<CareItemCard item={{ ...item, recurrence: null }} eligibility={liberado} />);
    expect(screen.getByText('Consulta')).toBeInTheDocument();
  });

  it('liberado: renderiza a ação que a tela passa (o card não conhece rotas)', () => {
    render(
      <CareItemCard
        item={item}
        eligibility={liberado}
        action={<a href="/jornada/agendar/item-1">Agendar</a>}
      />,
    );
    expect(screen.getByRole('link', { name: 'Agendar' })).toBeInTheDocument();
    // Liberado não mostra bloqueio.
    expect(screen.queryByText(/Aguarde o intervalo/)).not.toBeInTheDocument();
  });

  it('bloqueado: renderiza os motivos e não a ação', () => {
    render(
      <CareItemCard
        item={item}
        eligibility={bloqueado}
        action={<a href="/x">Agendar</a>}
      />,
    );
    expect(screen.getByText('Aguarde o intervalo mínimo entre consultas.')).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Agendar' })).not.toBeInTheDocument();
  });

  it('done: mostra "Feito" em verde e ignora a ação', () => {
    render(
      <CareItemCard item={item} eligibility={liberado} done action={<a href="/x">Agendar</a>} />,
    );
    const feito = screen.getByText('Feito');
    expect(feito).toBeInTheDocument();
    expect(feito.closest('[class*="text-success"]')).not.toBeNull();
    expect(screen.queryByRole('link', { name: 'Agendar' })).not.toBeInTheDocument();
  });

  it('não usa a cor de erro no bloqueio', () => {
    const { container } = render(<CareItemCard item={item} eligibility={bloqueado} />);
    expect(container.querySelector('[class*="error"]')).toBeNull();
  });
});
