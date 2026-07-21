import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';

import { AppShell } from './AppShell';

function renderShell(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AppShell userName="Ana" help={<span>ajuda-slot</span>}>
        <p>conteúdo da tela</p>
      </AppShell>
    </MemoryRouter>,
  );
}

describe('AppShell', () => {
  it('expõe a navegação do produto com os três destinos', () => {
    renderShell('/jornada');
    expect(screen.getByRole('navigation')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Jornada' })).toHaveAttribute('href', '/jornada');
    expect(screen.getByRole('link', { name: 'Consultas' })).toHaveAttribute('href', '/consultas');
    // Nav e avatar apontam para /perfil; ao menos um link rotulado "Perfil" existe.
    const perfilLinks = screen.getAllByRole('link', { name: 'Perfil' });
    expect(perfilLinks.some((a) => a.getAttribute('href') === '/perfil')).toBe(true);
  });

  it('marca como ativo o link da rota atual (via NavLink)', () => {
    renderShell('/consultas');
    expect(screen.getByRole('link', { name: 'Consultas' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'Jornada' })).not.toHaveAttribute('aria-current');
  });

  it('renderiza o slot de ajuda, os filhos e o avatar linkando /perfil', () => {
    renderShell('/jornada');
    expect(screen.getByText('ajuda-slot')).toBeInTheDocument();
    expect(screen.getByText('conteúdo da tela')).toBeInTheDocument();
    const avatar = screen.getByRole('img', { name: 'Ana' });
    expect(avatar.closest('a')).toHaveAttribute('href', '/perfil');
  });

  // Acessibilidade de teclado: um "pular para o conteúdo" salta a navegação
  // repetida do topo e leva direto ao <main>.
  it('oferece um link "Pular para o conteúdo" que aponta para o main', () => {
    renderShell('/jornada');
    const skip = screen.getByRole('link', { name: /pular para o conteúdo/i });
    expect(skip).toHaveAttribute('href', '#conteudo');
    // O alvo existe e é o <main>.
    expect(screen.getByRole('main')).toHaveAttribute('id', 'conteudo');
  });
});
