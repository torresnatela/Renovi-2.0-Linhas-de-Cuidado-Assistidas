import type { ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';

import logoBlue from '../../assets/logos/logo-blue.svg';
import { SegmentedControl } from '../../shared/ui/SegmentedControl';
import { IconCheck } from '../../shared/ui/icons';

const BULLETS = [
  'Agende consultas por vídeo em minutos',
  'Acompanhe seu humor e seu progresso',
  'Sigilo total — sua empresa nunca vê dados individuais',
];

/**
 * Painel de marca (esquerda). É ilustração pura, então mora em `style` inline
 * fiel ao handoff — o gradiente navy e os círculos radiais não são cores
 * semânticas do DS. O lado direito (o formulário) usa tokens/Tailwind.
 */
function BrandPanel() {
  return (
    <div
      className="relative hidden overflow-hidden text-white lg:flex lg:flex-col lg:justify-between"
      style={{
        background: 'linear-gradient(160deg, var(--color-primary-300) 0%, #0A123F 100%)',
        padding: '56px 64px',
      }}
    >
      <div
        aria-hidden
        className="pointer-events-none absolute"
        style={{
          width: 520,
          height: 520,
          borderRadius: '50%',
          background: 'radial-gradient(circle at 30% 30%, rgba(250,143,27,0.35), transparent 70%)',
          top: -160,
          right: -160,
        }}
      />
      <div
        aria-hidden
        className="pointer-events-none absolute"
        style={{
          width: 420,
          height: 420,
          borderRadius: '50%',
          background: 'radial-gradient(circle at 50% 50%, rgba(255,255,255,0.10), transparent 70%)',
          bottom: -140,
          left: -120,
        }}
      />

      <img
        src={logoBlue}
        alt="Renovi"
        className="relative h-[30px] self-start brightness-0 invert"
      />

      <div className="relative flex max-w-[460px] flex-col gap-5">
        <span className="text-[40px] font-bold leading-[48px] tracking-tight">
          Cuidado contínuo, do seu jeito.
        </span>
        <span className="text-[17px] leading-[26px] text-white/80">
          Psicologia, psiquiatria e saúde ocupacional em um só lugar. Sua jornada de cuidado,
          sempre à mão.
        </span>
        <div className="mt-2 flex flex-col gap-3.5">
          {BULLETS.map((text) => (
            <div key={text} className="flex items-center gap-3">
              <span className="inline-flex h-[34px] w-[34px] flex-shrink-0 items-center justify-center rounded-full bg-white/[0.14]">
                <IconCheck size={17} />
              </span>
              <span className="text-[15px] text-white/90">{text}</span>
            </div>
          ))}
        </div>
      </div>

      <span className="relative text-[13px] text-white/60">
        Renovi — tecnologia a serviço da sua saúde e do seu bem-estar.
      </span>
    </div>
  );
}

interface AccessLayoutProps {
  /** Aba ativa; controla o alternador. */
  active: 'login' | 'cadastro';
  /** Esconde o alternador (ex.: tela de sucesso do cadastro). */
  showSwitcher?: boolean;
  children: ReactNode;
}

/**
 * Split-screen das telas de acesso: marca à esquerda, formulário à direita.
 * O alternador Entrar/Criar conta é NAVEGAÇÃO entre `/entrar` e `/cadastro`
 * (rotas distintas seguem existindo) — não um toggle de estado local.
 */
export function AccessLayout({ active, showSwitcher = true, children }: AccessLayoutProps) {
  const navigate = useNavigate();

  return (
    <div className="grid min-h-screen bg-page lg:grid-cols-[1.05fr_1fr]">
      <BrandPanel />

      <div className="flex items-center justify-center px-6 py-12 sm:px-10">
        <div className="flex w-full max-w-[440px] flex-col gap-6">
          {showSwitcher && (
            <SegmentedControl
              options={[
                { value: 'login', label: 'Entrar' },
                { value: 'cadastro', label: 'Criar conta' },
              ]}
              value={active}
              onChange={(value) => navigate(value === 'login' ? '/entrar' : '/cadastro')}
            />
          )}
          {children}
        </div>
      </div>
    </div>
  );
}
