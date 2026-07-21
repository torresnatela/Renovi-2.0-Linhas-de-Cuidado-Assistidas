import { Link, useNavigate, useParams } from 'react-router-dom';

import type { AssessmentCode } from '../../shared/api';
import { Card } from '../../shared/ui/Card';
import { AssessmentForm } from './AssessmentForm';

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
 * desconhecido não vira página em branco: mostra o aviso e a volta. A página é a
 * ÚNICA a titular o instrumento (o h1 abaixo); o AssessmentForm não repete o
 * título, evitando o duplo-heading e o card-in-card.
 */
export function AssessmentPage() {
  const { codigo } = useParams();
  const navigate = useNavigate();

  if (!isAssessmentCode(codigo)) {
    return (
      <Card as="section" padding="lg" className="mx-auto max-w-2xl">
        <h1 className="text-lg font-bold text-primary-300">Avaliação não encontrada</h1>
        <p className="mt-2 text-sm text-ink">
          Este instrumento não existe ou não está disponível para você.
        </p>
        <Link
          to="/jornada"
          className="mt-4 inline-block text-sm font-bold text-primary-300 underline"
        >
          Voltar para a jornada
        </Link>
      </Card>
    );
  }

  return (
    <Card as="section" padding="lg" className="mx-auto max-w-2xl">
      <div className="mb-4 flex items-center justify-between gap-4">
        <h1 className="text-lg font-bold text-primary-300">{TITULO[codigo]}</h1>
        <Link to="/jornada" className="text-sm font-bold text-primary-300 underline">
          Voltar
        </Link>
      </div>
      <AssessmentForm codigo={codigo} onDone={() => navigate('/jornada')} />
    </Card>
  );
}
