import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import {
  IconAppointmentsFilled,
  IconCheck,
  IconHelpTarget,
  IconHome,
  IconHomeFilled,
  IconProfileFilled,
} from './icons';

describe('icons', () => {
  it('renderiza um svg decorativo no grid 24 (aria-hidden)', () => {
    const { container } = render(<IconHome />);
    const svg = container.querySelector('svg');
    expect(svg).not.toBeNull();
    expect(svg).toHaveAttribute('aria-hidden', 'true');
    expect(svg).toHaveAttribute('viewBox', '0 0 24 24');
  });

  it('usa o size default 20 e aceita size custom', () => {
    const { container: def } = render(<IconHome />);
    expect(def.querySelector('svg')).toHaveAttribute('width', '20');

    const { container } = render(<IconCheck size={32} />);
    const svg = container.querySelector('svg');
    expect(svg).toHaveAttribute('width', '32');
    expect(svg).toHaveAttribute('height', '32');
  });

  it('herda a cor via currentColor no stroke', () => {
    const { container } = render(<IconHelpTarget />);
    expect(container.querySelector('svg')).toHaveAttribute('stroke', 'currentColor');
  });

  describe('ícones filled (estado ativo do tab bar)', () => {
    // viewBox 21×21: viewBox nativo do handoff, portado verbatim (não
    // reescalado para o grid 24 dos ícones outline).
    it.each([
      ['IconHomeFilled', IconHomeFilled],
      ['IconAppointmentsFilled', IconAppointmentsFilled],
      ['IconProfileFilled', IconProfileFilled],
    ])('%s renderiza um svg no viewBox nativo do handoff (21×21) preenchido com currentColor', (_name, Icon) => {
      const { container } = render(<Icon />);
      const svg = container.querySelector('svg');
      expect(svg).not.toBeNull();
      expect(svg).toHaveAttribute('aria-hidden', 'true');
      expect(svg).toHaveAttribute('viewBox', '0 0 21 21');
      expect(svg).toHaveAttribute('fill', 'currentColor');
      expect(svg).not.toHaveAttribute('stroke');
    });

    it('usa o size default 20 e aceita size custom', () => {
      const { container: def } = render(<IconHomeFilled />);
      expect(def.querySelector('svg')).toHaveAttribute('width', '20');

      const { container } = render(<IconProfileFilled size={32} />);
      const svg = container.querySelector('svg');
      expect(svg).toHaveAttribute('width', '32');
      expect(svg).toHaveAttribute('height', '32');
    });

    // O detalhe branco do handoff (linhas/pontos por cima do preenchimento
    // navy) precisa vir do token --color-white, nunca do hex #fff do
    // arquivo original (regra do DS: sem hex hardcoded).
    it('usa o token --color-white no detalhe interno, nunca #fff hardcoded', () => {
      const { container } = render(<IconAppointmentsFilled />);
      const paths = Array.from(container.querySelectorAll('path'));
      const withWhiteDetail = paths.filter(
        (path) => path.getAttribute('fill') === 'var(--color-white)' || path.getAttribute('stroke') === 'var(--color-white)',
      );
      expect(withWhiteDetail.length).toBeGreaterThan(0);
      expect(container.innerHTML).not.toContain('#fff');
    });
  });
});
