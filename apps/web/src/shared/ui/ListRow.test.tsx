import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { ListRow } from './ListRow';

describe('ListRow', () => {
  it('mostra título e caption', () => {
    render(<ListRow title="Consulta" caption="Hoje, 14h" />);
    expect(screen.getByText('Consulta')).toBeInTheDocument();
    expect(screen.getByText('Hoje, 14h')).toBeInTheDocument();
  });

  it('clicável vira um botão acessível e dispara onClick', async () => {
    const onClick = vi.fn();
    const user = userEvent.setup();
    render(<ListRow title="Abrir consulta" onClick={onClick} />);
    await user.click(screen.getByRole('button', { name: /abrir consulta/i }));
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it('sem onClick não expõe role de botão', () => {
    render(<ListRow title="Estático" />);
    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });
});
