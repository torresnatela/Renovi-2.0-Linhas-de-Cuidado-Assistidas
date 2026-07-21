import { Card } from '../../shared/ui/Card';
import { useSession } from '../auth/useSession';

/**
 * Dados pessoais — SOMENTE LEITURA e SÓ o que a API expõe (nome + e-mail do /me).
 *
 * A honestidade é o requisito de produto aqui: não há endpoint de edição de
 * perfil, nem de leitura de CPF/celular/endereço. Então nada de "modo edição",
 * nada de campos travados ou vazios — apenas os dois pares que existem, e a
 * microcopy que aponta o caminho real para atualizar cadastro (o suporte).
 */
export function PersonalDataSection() {
  const session = useSession();
  const conta = session.data;

  const pares = [
    { label: 'Nome completo', value: conta?.full_name ?? '—' },
    { label: 'E-mail', value: conta?.email ?? '—' },
  ];

  return (
    <section id="dados" className="scroll-mt-24">
      <Card padding="lg">
        <h2 className="text-lg font-bold text-primary-300">Dados pessoais</h2>

        <dl className="mt-5 grid grid-cols-1 gap-x-8 gap-y-[18px] sm:grid-cols-2">
          {pares.map((p) => (
            <div
              key={p.label}
              className="flex flex-col gap-[3px] border-b border-primary-100 pb-3.5"
            >
              <dt className="text-xs font-bold uppercase tracking-[0.05em] text-muted">
                {p.label}
              </dt>
              <dd className="text-[15px] text-primary-300">{p.value}</dd>
            </div>
          ))}
        </dl>

        <p className="mt-5 text-[13px] text-muted">
          Para atualizar seus dados cadastrais, fale com o suporte Renovi.
        </p>
      </Card>
    </section>
  );
}
