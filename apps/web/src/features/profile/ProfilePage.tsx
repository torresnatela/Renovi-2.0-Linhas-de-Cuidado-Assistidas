import { PersonalDataSection } from './PersonalDataSection';
import { PlanSection } from './PlanSection';
import { PrivacySection } from './PrivacySection';
import { ProfileSummary } from './ProfileSummary';

/**
 * Perfil (/perfil) na versão reduzida-honesta: mostra SÓ o que a API expõe —
 * identidade (nome/e-mail do /me), o plano REAL da jornada e o único controle de
 * privacidade que existe (revogar o consentimento do check-in). Sem edição de
 * perfil, sem troca de senha, sem notificações, sem "baixar/excluir dados" — não
 * há endpoint para nada disso, e um botão que não faz nada engana.
 *
 * Renderiza DENTRO do AppShell (não traz `<main>` nem header próprios). Layout de
 * duas colunas: o resumo sticky à esquerda e as seções à direita (com âncoras
 * que o resumo navega).
 */
export function ProfilePage() {
  return (
    <div className="flex flex-col gap-7">
      <div className="flex flex-col gap-1">
        <span className="text-xs font-bold uppercase tracking-[0.08em] text-muted">Sua conta</span>
        <h1 className="text-[32px] font-bold leading-tight text-primary-300">Perfil</h1>
      </div>

      <div className="grid grid-cols-1 items-start gap-8 lg:grid-cols-[340px_minmax(0,1fr)]">
        <ProfileSummary />
        <div className="flex min-w-0 flex-col gap-6">
          <PersonalDataSection />
          <PlanSection />
          <PrivacySection />
        </div>
      </div>
    </div>
  );
}
