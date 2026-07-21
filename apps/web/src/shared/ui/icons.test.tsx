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
    it.each([
      ['IconHomeFilled', IconHomeFilled],
      ['IconAppointmentsFilled', IconAppointmentsFilled],
      ['IconProfileFilled', IconProfileFilled],
    ])('%s renderiza um svg no grid 24 preenchido com currentColor', (_name, Icon) => {
      const { container } = render(<Icon />);
      const svg = container.querySelector('svg');
      expect(svg).not.toBeNull();
      expect(svg).toHaveAttribute('aria-hidden', 'true');
      expect(svg).toHaveAttribute('viewBox', '0 0 24 24');
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
  });
});
