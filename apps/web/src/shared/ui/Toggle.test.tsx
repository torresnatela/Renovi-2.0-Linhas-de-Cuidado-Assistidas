import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { Toggle } from './Toggle';

describe('Toggle', () => {
  it('expõe role switch com aria-checked refletindo o estado', () => {
    render(<Toggle checked={false} onChange={() => {}} label="Notificações" />);
    expect(screen.getByRole('switch', { name: 'Notificações' })).toHaveAttribute(
      'aria-checked',
      'false',
    );
  });

  it('clique alterna chamando onChange com o novo valor', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<Toggle checked={false} onChange={onChange} label="Notificações" />);
    await user.click(screen.getByRole('switch', { name: 'Notificações' }));
    expect(onChange).toHaveBeenCalledWith(true);
  });

  it('a tecla Espaço alterna o switch', async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<Toggle checked={true} onChange={onChange} label="Notificações" />);
    screen.getByRole('switch', { name: 'Notificações' }).focus();
    await user.keyboard(' ');
    expect(onChange).toHaveBeenCalledWith(false);
  });
});
