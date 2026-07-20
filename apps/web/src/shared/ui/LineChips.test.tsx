import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { LineChips } from './LineChips';

const lines = [
  { code: 'saude-mental', name: 'Saúde Mental' },
  { code: 'cardio', name: 'Cardiologia' },
];

describe('LineChips', () => {
  it('marca a linha ativa com aria-selected e roving tabindex', () => {
    render(<LineChips lines={lines} active="saude-mental" onSelect={() => {}} />);
    const ativo = screen.getByRole('tab', { name: 'Saúde Mental' });
    const inativo = screen.getByRole('tab', { name: 'Cardiologia' });
    expect(ativo).toHaveAttribute('aria-selected', 'true');
    expect(inativo).toHaveAttribute('aria-selected', 'false');
    expect(ativo).toHaveAttribute('tabindex', '0');
    expect(inativo).toHaveAttribute('tabindex', '-1');
  });

  it('a ativa é navy sólido; a inativa é branca com borda', () => {
    render(<LineChips lines={lines} active="saude-mental" onSelect={() => {}} />);
    expect(screen.getByRole('tab', { name: 'Saúde Mental' }).className).toContain('bg-primary-300');
    expect(screen.getByRole('tab', { name: 'Cardiologia' }).className).toContain(
      'border-primary-200',
    );
  });

  it('clicar numa linha chama onSelect com o code', async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();
    render(<LineChips lines={lines} active="saude-mental" onSelect={onSelect} />);
    await user.click(screen.getByRole('tab', { name: 'Cardiologia' }));
    expect(onSelect).toHaveBeenCalledWith('cardio');
  });

  it('seta direita move a seleção para a próxima', async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();
    render(<LineChips lines={lines} active="saude-mental" onSelect={onSelect} />);
    screen.getByRole('tab', { name: 'Saúde Mental' }).focus();
    await user.keyboard('{ArrowRight}');
    expect(onSelect).toHaveBeenCalledWith('cardio');
  });

  it('seta esquerda dá a volta para a última', async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();
    render(<LineChips lines={lines} active="saude-mental" onSelect={onSelect} />);
    screen.getByRole('tab', { name: 'Saúde Mental' }).focus();
    await user.keyboard('{ArrowLeft}');
    expect(onSelect).toHaveBeenCalledWith('cardio');
  });
});
