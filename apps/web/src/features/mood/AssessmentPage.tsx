import { Link, useNavigate, useParams } from 'react-router-dom';

import type { AssessmentCode } from '../../shared/api';
import { useIsDesktop } from '../../shared/viewport';
import { Card } from '../../shared/ui/Card';
import { FlowHeader } from '../../shared/ui/FlowHeader';
import { AssessmentForm } from './AssessmentForm';
import { HelpNowMenu } from './HelpNowMenu';

const TITULO: Record<AssessmentCode, string> = {
  WHO5: 'Índice de bem-estar (WHO-5)',
  PHQ4: 'Rastreio de humor e ansiedade (PHQ-4)',
};

function isAssessmentCode(codigo: string | undefined): codigo is AssessmentCode {
  return codigo === 'WHO5' || codigo === 'PHQ4';
}

/**
 * Entrada fina de /avaliacoes/:codigo — hospeda um instrumento periódico
 * (WHO-5/PHQ-4) fora do fluxo diário do humor, para links diretos. Um código
 * desconhecido não vira página em branco: mostra o aviso e a volta.
 *
 * No mobile é fluxo empilhado (ADR-041): o cabeçalho do card (h1 + "Voltar") dá
 * lugar ao `FlowHeader` (voltar → /jornada, ajuda de sempre) — o AssessmentForm
 * não repete o título de qualquer forma, evitando o duplo-heading. O `title` do
 * FlowHeader é o nome do instrumento quando ele existe no estado atual; sem
 * instrumento (código inválido) cai no mesmo rótulo do eyebrow — nunca inventa
 * um nome. Desktop intocado: o card continua a ÚNICA fonte do título (o h1) e o
 * link "Voltar" original, exatamente como antes.
 */
export function AssessmentPage() {
  const { codigo } = useParams();
  const navigate = useNavigate();
  const isDesktop = useIsDesktop();

  if (!isAssessmentCode(codigo)) {
    return (
      <>
        {!isDesktop && (
          <FlowHeader
            eyebrow="Avaliação"
            title="Avaliação"
            backTo="/jornada"
            help={<HelpNowMenu />}
          />
        )}
        <Card
          as="section"
          padding="lg"
          className={isDesktop ? 'mx-auto max-w-2xl' : 'mx-auto mt-6 max-w-2xl'}
        >
          {isDesktop && (
            <h1 className="text-lg font-bold text-primary-300">Avaliação não encontrada</h1>
          )}
          <p className={isDesktop ? 'mt-2 text-sm text-ink' : 'text-sm text-ink'}>
            Este instrumento não existe ou não está disponível para você.
          </p>
          {isDesktop && (
            <Link
              to="/jornada"
              className="mt-4 inline-block text-sm font-bold text-primary-300 underline"
            >
              Voltar para a jornada
            </Link>
          )}
        </Card>
      </>
    );
  }

  return (
    <>
      {!isDesktop && (
        <FlowHeader
          eyebrow="Avaliação"
          title={TITULO[codigo]}
          backTo="/jornada"
          help={<HelpNowMenu />}
        />
      )}
      <Card
        as="section"
        padding="lg"
        className={isDesktop ? 'mx-auto max-w-2xl' : 'mx-auto mt-6 max-w-2xl'}
      >
        {isDesktop && (
          <div className="mb-4 flex items-center justify-between gap-4">
            <h1 className="text-lg font-bold text-primary-300">{TITULO[codigo]}</h1>
            <Link to="/jornada" className="text-sm font-bold text-primary-300 underline">
              Voltar
            </Link>
          </div>
        )}
        <AssessmentForm codigo={codigo} onDone={() => navigate('/jornada')} />
      </Card>
    </>
  );
}
