import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { AppShell } from './AppShell';
import { mockViewport } from '../viewport.testkit';

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

  // Chrome mobile (< lg): sem header sticky desktop. `mockViewport('mobile')`
  // força `useIsDesktop()` para false; `restore()` devolve o default desktop
  // para não vazar entre os `it()`s (nem para as asserções desktop acima).
  describe('mobile', () => {
    let viewport: ReturnType<typeof mockViewport>;

    beforeEach(() => {
      viewport = mockViewport('mobile');
    });
    afterEach(() => {
      viewport.restore();
    });

    function renderMobile(variant?: 'tabs' | 'flow', path = '/jornada') {
      return render(
        <MemoryRouter initialEntries={[path]}>
          <AppShell userName="Ana" help={<span>ajuda-slot</span>} mobileVariant={variant}>
            <p>conteúdo da tela</p>
          </AppShell>
        </MemoryRouter>,
      );
    }

    it('variant tabs: sem header desktop, com faixa de logo rolável e TabBar', () => {
      renderMobile('tabs');
      // Sem o header sticky do desktop (role banner) nem o avatar/slot de ajuda.
      expect(screen.queryByRole('banner')).not.toBeInTheDocument();
      expect(screen.queryByRole('img', { name: 'Ana' })).not.toBeInTheDocument();
      expect(screen.queryByText('ajuda-slot')).not.toBeInTheDocument();
      // Logo presente, dentro do <main> (rola com o conteúdo, não é sticky).
      const logo = screen.getByRole('img', { name: 'Renovi Saúde' });
      expect(logo.closest('main')).not.toBeNull();
      // TabBar presente.
      expect(screen.getByRole('navigation', { name: 'Principal' })).toBeInTheDocument();
      expect(screen.getByText('conteúdo da tela')).toBeInTheDocument();
    });

    it('variant flow: sem TabBar e sem faixa de logo', () => {
      renderMobile('flow');
      expect(screen.queryByRole('navigation', { name: 'Principal' })).not.toBeInTheDocument();
      expect(screen.queryByRole('img', { name: 'Renovi Saúde' })).not.toBeInTheDocument();
      expect(screen.getByText('conteúdo da tela')).toBeInTheDocument();
    });

    it('o skip-link continua o primeiro focável e aponta para o main (tabs)', () => {
      const { container } = renderMobile('tabs');
      const primeiroLink = container.querySelector('a');
      expect(primeiroLink).toHaveTextContent(/pular para o conteúdo/i);
      expect(primeiroLink).toHaveAttribute('href', '#conteudo');
      expect(screen.getByRole('main')).toHaveAttribute('id', 'conteudo');
    });

    it('o skip-link continua o primeiro focável e aponta para o main (flow)', () => {
      const { container } = renderMobile('flow');
      const primeiroLink = container.querySelector('a');
      expect(primeiroLink).toHaveTextContent(/pular para o conteúdo/i);
      expect(primeiroLink).toHaveAttribute('href', '#conteudo');
      expect(screen.getByRole('main')).toHaveAttribute('id', 'conteudo');
    });

    it('default de mobileVariant é tabs (TabBar presente sem passar a prop)', () => {
      renderMobile(undefined);
      expect(screen.getByRole('navigation', { name: 'Principal' })).toBeInTheDocument();
    });
  });
});
