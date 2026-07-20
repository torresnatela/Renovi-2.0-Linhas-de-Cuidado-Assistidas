import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { Button } from './Button';

describe('Button', () => {
  it('renderiza os children como rótulo acessível', () => {
    render(<Button>Agendar</Button>);
    expect(screen.getByRole('button', { name: /agendar/i })).toBeInTheDocument();
  });

  it('em loading mantém o nome acessível e desabilita', () => {
    render(<Button loading>Agendar</Button>);
    const btn = screen.getByRole('button', { name: /agendar/i });
    expect(btn).toBeDisabled();
    expect(btn).toHaveAttribute('aria-busy', 'true');
  });

  it('não dispara onClick quando desabilitado', async () => {
    const onClick = vi.fn();
    const user = userEvent.setup();
    render(
      <Button disabled onClick={onClick}>
        Agendar
      </Button>,
    );
    await user.click(screen.getByRole('button', { name: /agendar/i }));
    expect(onClick).not.toHaveBeenCalled();
  });

  it('aplica a variante accent (smoke de classe)', () => {
    render(<Button color="accent">Renovar</Button>);
    expect(screen.getByRole('button', { name: /renovar/i }).className).toContain('bg-accent-300');
  });

  it('uppercase vem de classe CSS — o texto-fonte fica em sentence case', () => {
    render(<Button>Agendar</Button>);
    const btn = screen.getByRole('button', { name: /agendar/i });
    expect(btn.className).toContain('uppercase');
    expect(btn).toHaveTextContent('Agendar');
  });
});
