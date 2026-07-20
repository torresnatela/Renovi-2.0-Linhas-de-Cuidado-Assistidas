import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { SegmentedControl } from './SegmentedControl';

const options = [
  { value: 'login', label: 'Entrar' },
  { value: 'signup', label: 'Criar conta' },
];

describe('SegmentedControl', () => {
  it('marca a aba selecionada com aria-selected e roving tabindex', () => {
    render(<SegmentedControl options={options} value="login" onChange={() => {}} />);
    const entrar = screen.getByRole('tab', { name: 'Entrar' });
    const criar = screen.getByRole('tab', { name: 'Criar conta' });
    expect(entrar).toHaveAttribute('aria-selected', 'true');
    expect(criar).toHaveAttribute('aria-selected', 'false');
    expect(entrar).toHaveAttribute('tabindex', '0');
    expect(criar).toHaveAttribute('tabindex', '-1');
  });

  it('clique numa aba chama onChange com o valor', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<SegmentedControl options={options} value="login" onChange={onChange} />);
    await user.click(screen.getByRole('tab', { name: 'Criar conta' }));
    expect(onChange).toHaveBeenCalledWith('signup');
  });

  it('seta direita move a seleção', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<SegmentedControl options={options} value="login" onChange={onChange} />);
    screen.getByRole('tab', { name: 'Entrar' }).focus();
    await user.keyboard('{ArrowRight}');
    expect(onChange).toHaveBeenCalledWith('signup');
  });

  it('seta esquerda dá a volta para o último', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<SegmentedControl options={options} value="login" onChange={onChange} />);
    screen.getByRole('tab', { name: 'Entrar' }).focus();
    await user.keyboard('{ArrowLeft}');
    expect(onChange).toHaveBeenCalledWith('signup');
  });
});
