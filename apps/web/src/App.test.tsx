import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import App from './App';

function renderWithQuery(ui: React.ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

describe('App', () => {
  it('renderiza o cabeçalho do produto', () => {
    renderWithQuery(<App />);
    expect(screen.getByRole('heading', { level: 1, name: 'Renovi 2.0' })).toBeInTheDocument();
  });

  it('mostra o badge de saúde da API', () => {
    renderWithQuery(<App />);
    // Sem backend no teste, o badge começa em "Verificando…".
    expect(screen.getByText(/Verificando API/i)).toBeInTheDocument();
  });
});
