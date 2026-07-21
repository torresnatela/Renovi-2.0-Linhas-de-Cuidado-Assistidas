import { Component, type ReactNode } from 'react';

/**
 * Rede de segurança de renderização.
 *
 * Um erro síncrono no render de qualquer tela (por exemplo, um `time_zone`
 * malformado vindo da API faz `toLocaleTimeString` lançar RangeError) deixaria o
 * app INTEIRO em branco, sem cabeçalho nem saída. O boundary transforma isso numa
 * mensagem com um caminho de volta.
 *
 * É class component porque o React só oferece captura de erro de render via
 * `componentDidCatch`/`getDerivedStateFromError` — não há hook equivalente.
 */
interface Props {
  children: ReactNode;
}

interface State {
  erro: boolean;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { erro: false };

  static getDerivedStateFromError(): State {
    return { erro: true };
  }

  componentDidCatch(erro: unknown) {
    // Vai para o console (e, no futuro, para a telemetria). Sem dado de saúde: é
    // um erro de render, não o conteúdo.
    console.error('[renovi] erro de render capturado pelo boundary', erro);
  }

  render() {
    if (this.state.erro) {
      return (
        <main className="mx-auto max-w-3xl px-6 py-10">
          <h2 className="mb-2 text-lg font-bold text-primary-300">Algo deu errado nesta tela</h2>
          <p className="mb-4 text-sm text-muted">
            Tente recarregar a página. Se continuar, fale com o suporte.
          </p>
          <a
            href="/"
            className="text-sm font-bold text-primary-300 underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-300"
          >
            Voltar ao início
          </a>
        </main>
      );
    }
    return this.props.children;
  }
}
