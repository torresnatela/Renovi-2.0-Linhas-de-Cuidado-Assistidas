-- Consultas do agendamento (tabela appointment, migration 0003).
--
-- Cada UPDATE aqui é um passo da saga, e todos são estreitos de propósito: um
-- "UpdateAppointment" genérico deixaria o próximo model escrever qualquer
-- transição, inclusive as que os CHECK do banco existem para impedir.

-- name: CreateAppointmentIntent :one
-- Passo 1 da saga: registra a INTENÇÃO antes de tocar em qualquer sistema
-- externo. O índice único parcial decide a corrida entre dois pacientes nossos.
INSERT INTO appointment (
    id, account_id,
    legacy_slot_id, legacy_professional_id, legacy_specialty_id,
    professional_name, specialty_name,
    starts_at, ends_at, status
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'PENDING_SLOT')
RETURNING *;

-- name: MarkAppointmentSlotHeld :exec
-- Passo 2: o horário é nosso e estamos prestes a fazer a escrita insondável.
-- Gravar ANTES do POST é o que separa "sabemos que a DAV nunca foi chamada" de
-- "a DAV pode ter sido chamada". Custa ~1ms e é o que torna o crash recuperável.
UPDATE appointment
SET status = 'DAV_PENDING',
    slot_held_at = now(),
    dav_attempted_at = now(),
    updated_at = now()
WHERE id = $1 AND status = 'PENDING_SLOT';

-- name: ConfirmAppointment :exec
-- Passo 3: a DAV criou e temos o link. O CHECK confirmed_exige_dav garante que
-- não dá para chegar em CONFIRMED sem os dois.
UPDATE appointment
SET status = 'CONFIRMED',
    dav_appointment_id = $2,
    patient_join_url = $3,
    confirmed_at = now(),
    updated_at = now()
WHERE id = $1 AND status = 'DAV_PENDING';

-- name: FailAppointment :exec
-- A consulta comprovadamente NÃO aconteceu (a DAV recusou o payload, ou nem
-- chegamos a reservar). Só use quando houver certeza: FAILED tira a linha do
-- índice de reservas vivas e libera o horário para outro paciente.
UPDATE appointment
SET status = 'FAILED', updated_at = now()
WHERE id = $1 AND status IN ('PENDING_SLOT', 'DAV_PENDING');

-- name: MarkAppointmentUnknown :exec
-- O ErrMaybeApplied virando estado. A consulta PODE existir na DAV e nunca
-- saberemos sozinhos (id é deles, não há rota de busca). O horário fica retido —
-- o CHECK desconhecido_nao_libera impede que alguém o solte por engano.
UPDATE appointment
SET status = 'DAV_UNKNOWN', updated_at = now()
WHERE id = $1 AND status = 'DAV_PENDING';

-- name: MarkSlotReleased :exec
-- Registra que o horário voltou ao mercado no legado. Separado do FailAppointment
-- porque são dois sistemas: entre marcar FAILED aqui e soltar o booked lá pode
-- haver um crash, e é essa diferença que o worker usa como fila de compensação.
UPDATE appointment
SET slot_released_at = now(), updated_at = now()
WHERE id = $1 AND slot_held_at IS NOT NULL AND slot_released_at IS NULL;

-- name: ListPendingSlotRelease :many
-- A fila de compensação do worker: falhou, o horário é nosso e ainda não voltou.
-- É uma consulta ao banco, e não um estado em memória, para sobreviver a um
-- restart no meio.
SELECT * FROM appointment
WHERE status = 'FAILED'
  AND slot_held_at IS NOT NULL
  AND slot_released_at IS NULL
ORDER BY updated_at
FOR UPDATE SKIP LOCKED
LIMIT $1;

-- name: ListStaleInFlight :many
-- Agendamentos que ficaram no meio do caminho (o processo morreu). O worker
-- decide o destino de cada um consultando o legado.
SELECT * FROM appointment
WHERE status IN ('PENDING_SLOT', 'DAV_PENDING')
  AND updated_at < $1
ORDER BY updated_at
FOR UPDATE SKIP LOCKED
LIMIT $2;

-- name: ListAppointmentsByAccount :many
-- "Minhas consultas". FAILED não aparece: consulta que comprovadamente não
-- aconteceu não é informação para o paciente. DAV_UNKNOWN aparece — ele pode ter
-- uma consulta de verdade marcada.
SELECT * FROM appointment
WHERE account_id = $1
  AND status <> 'FAILED'
ORDER BY starts_at DESC;

-- name: GetAppointmentForAccount :one
-- Sempre por (id, dono). Nunca só por id: um SELECT por id sozinho é um convite a
-- alguém esquecer o WHERE do dono na próxima rota e devolver a consulta de
-- terceiro — e aqui a consulta carrega o link da sala.
SELECT * FROM appointment
WHERE id = $1 AND account_id = $2;

-- name: CountAppointmentsNeedingReview :one
-- Quantas consultas estão em DAV_UNKNOWN. Se esta lista cresce, alguém precisa
-- olhar: a máquina não consegue resolver sozinha.
SELECT COUNT(*) FROM appointment WHERE status = 'DAV_UNKNOWN';

-- name: GetAccountDavPersonID :one
-- O id do paciente na DAV, que vira o participante PAT do appointment.
--
-- Filtra por ACTIVE porque conta PENDING_DAV não tem vínculo (é o que o CHECK
-- active_exige_vinculo_dav garante). Na prática ela nem chega aqui — a sessão só
-- valida conta ACTIVE — mas o agendamento não deve depender disso para não criar
-- consulta na DAV sem paciente.
SELECT dav_person_id FROM patient_account
WHERE id = $1 AND status = 'ACTIVE';
