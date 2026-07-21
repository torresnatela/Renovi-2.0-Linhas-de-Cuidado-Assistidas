import { describe, expect, it } from 'vitest';

import type { JourneyEnrollment, JourneyItem, MoodToday } from '../../shared/api';
import { itensComCheckinSintetico } from './derivations';

// ---------------------------------------------------------------------------
// Nó SINTÉTICO de check-in de humor na timeline (decisão de produto aprovada):
// a linha universal de saúde mental não materializa o check-in como item de
// template, mas ele DEVE aparecer na jornada. `itensComCheckinSintetico`
// preposiciona o pseudo-item só quando: matriculado no humor + linha mental +
// a linha ainda não traz o item (dedupe).
// ---------------------------------------------------------------------------

const REF_CHECKIN = 'checkin-humor-diario';
const CODE_MENTAL_ABERTA = 'saude-mental-aberta';

function consulta(id: string, ref: string, sortOrder: number): JourneyItem {
  return {
    item: {
      id,
      ref,
      kind: 'CONSULTA',
      specialty_code: 'PSI',
      label: `Consulta ${ref}`,
      sort_order: sortOrder,
    },
    eligibility: { allowed: true, blocks: [] },
  };
}

function enrollment(careLineCode: string, items: JourneyItem[]): JourneyEnrollment {
  return {
    enrollment: {
      id: 'enr-1',
      care_line_code: careLineCode,
      care_line_version: 1,
      status: 'ativa',
      valid_from: '2026-07-01T00:00:00-03:00',
      valid_until: '2026-12-01T00:00:00-03:00',
      periods: [],
    },
    care_line_name: 'Saúde Mental (aberta)',
    items,
    recent_events: [],
  };
}

const moodEnrolled: MoodToday = { dia: '2026-07-21', can_checkin: true, checkin: null };
const moodNotEnrolled: MoodToday = { dia: '2026-07-21', can_checkin: false, reason: 'not_enrolled' };
const moodConsent: MoodToday = { dia: '2026-07-21', can_checkin: false, reason: 'consent_required' };

describe('itensComCheckinSintetico', () => {
  it('prepõe o nó de check-in na linha mental quando matriculado e a linha não tem o item', () => {
    const enr = enrollment(CODE_MENTAL_ABERTA, [consulta('c1', 'psicologia', 1)]);
    const out = itensComCheckinSintetico(enr, moodEnrolled);

    expect(out).toHaveLength(2);
    const sintetico = out.find((it) => it.item.ref === REF_CHECKIN);
    expect(sintetico).toBeDefined();
    expect(sintetico!.item.kind).toBe('ATIVIDADE');
    expect(sintetico!.item.label).toBe('Check-in de humor');
    expect(sintetico!.item.recurrence).toBe('Diário · opcional');
    // sort_order abaixo do menor item real → abre a timeline após reordenação.
    expect(sintetico!.item.sort_order).toBeLessThan(1);
    // consent_required também conta como matriculado (reason !== 'not_enrolled').
    expect(itensComCheckinSintetico(enr, moodConsent)).toHaveLength(2);
  });

  it('NÃO duplica quando a linha já traz o item de check-in (dedupe)', () => {
    const jaTem: JourneyItem = {
      item: {
        id: 'existente',
        ref: REF_CHECKIN,
        kind: 'ATIVIDADE',
        specialty_code: '',
        label: 'Check-in de humor',
        sort_order: 1,
      },
      eligibility: { allowed: true, blocks: [] },
    };
    const enr = enrollment(CODE_MENTAL_ABERTA, [jaTem, consulta('c1', 'psicologia', 2)]);
    const out = itensComCheckinSintetico(enr, moodEnrolled);

    expect(out).toHaveLength(2);
    expect(out.filter((it) => it.item.ref === REF_CHECKIN)).toHaveLength(1);
    // Mantém o array original (referência intacta) quando não sintetiza.
    expect(out).toBe(enr.items);
  });

  it('NÃO prepõe em linha que não é a de saúde mental aberta', () => {
    const enr = enrollment('ortopedia', [consulta('c1', 'orto', 1)]);
    expect(itensComCheckinSintetico(enr, moodEnrolled)).toHaveLength(1);
  });

  it('NÃO prepõe quando não há matrícula no humor (not_enrolled) nem quando mood não carregou', () => {
    const enr = enrollment(CODE_MENTAL_ABERTA, [consulta('c1', 'psicologia', 1)]);
    expect(itensComCheckinSintetico(enr, moodNotEnrolled)).toHaveLength(1);
    expect(itensComCheckinSintetico(enr, undefined)).toHaveLength(1);
  });
});
