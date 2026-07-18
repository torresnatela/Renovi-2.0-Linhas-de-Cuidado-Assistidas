/**
 * Sai do app para a sala da teleconsulta.
 *
 * `replace`, e não `assign`: o link é uma CREDENCIAL, e `assign` o gravaria no
 * histórico do browser, de onde qualquer um com o aparelho o recupera depois.
 * `replace` troca a entrada atual (o detalhe da consulta) pela sala, sem deixar o
 * link no histórico. Custo aceito: o "voltar" da sala não retorna ao detalhe —
 * quem está entrando numa consulta não deveria estar quicando de volta mesmo.
 *
 * E é `replace`, não `window.open`: só sabemos a url DEPOIS do POST /join, fora
 * do gesto do usuário, e pop-up fora do gesto é bloqueado. Navegação na mesma aba
 * não passa por bloqueador.
 *
 * É um módulo, e não uma linha solta no componente, porque é o que permite
 * `vi.mock` no teste: o jsdom não deixa espionar `window.location`.
 */
export function openExternal(url: string): void {
  // Defesa em profundidade: só navega para https. A url vem da DAV (upstream
  // confiável), mas um valor `javascript:` executaria no NOSSO domínio. Barato de
  // conferir, e o custo de recusar um link malformado é menor que o de rodar
  // script arbitrário na origem do paciente.
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    throw new Error('link de atendimento inválido');
  }
  if (parsed.protocol !== 'https:') {
    throw new Error('link de atendimento com esquema inesperado');
  }
  window.location.replace(url);
}
