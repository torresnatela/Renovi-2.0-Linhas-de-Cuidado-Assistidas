import { useIsDesktop } from '../../shared/viewport';
import { HelpNowMenu } from '../mood/HelpNowMenu';
import { PersonalDataSection } from './PersonalDataSection';
import { PlanSection } from './PlanSection';
import { PrivacySection } from './PrivacySection';
import { LogoutAction, ProfileSummary } from './ProfileSummary';

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
 *
 * No mobile (tela RAIZ, mantém a tab bar — ADR-041) o cabeçalho de duas linhas
 * do desktop dá lugar ao padrão de tela raiz (eyebrow "Seu perfil" + título
 * grande + Pedir ajuda), igual ao mock da Jornada. O grid de duas colunas já
 * colapsa sozinho no mobile; só reordenamos o Sair para o FIM da pilha de
 * seções (resumo → dados → plano → privacidade → Sair) — no desktop ele
 * continua dentro do aside sticky, como sempre foi. Desktop intocado.
 */
export function ProfilePage() {
  const isDesktop = useIsDesktop();

  return (
    <div className="flex flex-col gap-7">
      {isDesktop ? (
        <div className="flex flex-col gap-1">
          <span className="text-xs font-bold uppercase tracking-[0.08em] text-muted">
            Sua conta
          </span>
          <h1 className="text-[32px] font-bold leading-tight text-primary-300">Perfil</h1>
        </div>
      ) : (
        <div className="flex items-start justify-between gap-3 pt-2">
          <div className="flex flex-col gap-0.5">
            <span className="text-[11px] font-bold uppercase tracking-[0.08em] text-muted">
              Seu perfil
            </span>
            <span className="text-[26px] font-bold leading-8 text-primary-300">Perfil</span>
          </div>
          <HelpNowMenu />
        </div>
      )}

      <div className="grid grid-cols-1 items-start gap-5 lg:gap-8 lg:grid-cols-[340px_minmax(0,1fr)]">
        <ProfileSummary />
        <div className="flex min-w-0 flex-col gap-5 lg:gap-6">
          <PersonalDataSection />
          <PlanSection />
          <PrivacySection />
          {!isDesktop && <LogoutAction />}
        </div>
      </div>
    </div>
  );
}
