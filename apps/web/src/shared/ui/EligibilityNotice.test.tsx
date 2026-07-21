import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import type { EligibilityBlock } from '../api';
import { EligibilityNotice } from './EligibilityNotice';

const intervalo: EligibilityBlock = {
  rule_type: 'MIN_INTERVAL',
  reason: 'Aguarde o intervalo mínimo entre consultas.',
  available_from: '2026-08-01T00:00:00-03:00',
};

describe('EligibilityNotice', () => {
  /**
   * A regra de ouro: o `reason` vem PRONTO do servidor e é exibido LETRA POR
   * LETRA — nunca reescrito nem traduzido (é a exceção à tabela do reasons.ts).
   */
  it('mostra o reason verbatim do servidor', () => {
    render(<EligibilityNotice blocks={[intervalo]} />);
    expect(screen.getByText('Aguarde o intervalo mínimo entre consultas.')).toBeInTheDocument();
  });

  it('formata available_from no fuso America/Sao_Paulo, em negrito', () => {
    render(<EligibilityNotice blocks={[intervalo]} />);
    // 01/08 no fuso da agenda — em UTC este instante já seria outro dia.
    const forte = screen.getByText(/01\/08/);
    expect(forte).toBeInTheDocument();
    expect(forte.tagName).toBe('STRONG');
  });

  it('respeita o timeZone recebido em vez do padrão', () => {
    // Em Lisboa (UTC+1) a meia-noite de SP de 01/08 cai às 04:00 do mesmo dia.
    render(<EligibilityNotice blocks={[intervalo]} timeZone="Europe/Lisbon" />);
    expect(screen.getByText(/01\/08/)).toBeInTheDocument();
  });

  it('omite o sufixo quando não há available_from', () => {
    const prereq: EligibilityBlock = {
      rule_type: 'PREREQUISITE',
      reason: 'Faça a avaliação inicial antes desta consulta.',
    };
    render(<EligibilityNotice blocks={[prereq]} />);
    expect(screen.getByText('Faça a avaliação inicial antes desta consulta.')).toBeInTheDocument();
    expect(screen.queryByText(/Disponível a partir de/)).not.toBeInTheDocument();
  });

  it('escolhe o ícone pelo rule_type', () => {
    const cases: Array<[EligibilityBlock['rule_type'], string]> = [
      ['QUOTA', 'clock'],
      ['MIN_INTERVAL', 'clock'],
      ['PREREQUISITE', 'arrow'],
      ['VIGENCIA', 'calendar'],
      ['MAX_ADVANCE', 'calendar'],
    ];
    for (const [rule, icon] of cases) {
      const { unmount } = render(
        <EligibilityNotice blocks={[{ rule_type: rule, reason: 'x' }]} />,
      );
      expect(screen.getByTestId('eligibility-icon')).toHaveAttribute('data-icon', icon);
      unmount();
    }
  });

  /**
   * Bloqueio de regra é ESTADO DO PLANO, não erro. Jamais em vermelho: o
   * componente não pode carregar nenhuma classe da cor de erro do DS.
   */
  it('nunca usa a cor de erro (não é falha, é estado do plano)', () => {
    const { container } = render(<EligibilityNotice blocks={[intervalo]} />);
    expect(container.querySelector('[class*="error"]')).toBeNull();
    expect(container.querySelector('[class*="red"]')).toBeNull();
  });

  it('renderiza um box por block', () => {
    render(
      <EligibilityNotice
        blocks={[
          intervalo,
          { rule_type: 'PREREQUISITE', reason: 'Faça a avaliação inicial antes.' },
        ]}
      />,
    );
    expect(screen.getAllByTestId('eligibility-icon')).toHaveLength(2);
  });

  it('no modo compact usa ícone menor (14px)', () => {
    const { container } = render(<EligibilityNotice blocks={[intervalo]} compact />);
    expect(container.querySelector('svg')).toHaveAttribute('width', '14');
  });
});
