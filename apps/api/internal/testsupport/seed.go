//go:build integration

package testsupport

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

// SeededSlot é um horário futuro criado pelo SeedFutureSlots, com o INSTANTE
// exato que o adapter deve devolver (o teste compara com tolerância zero).
type SeededSlot struct {
	ID       string
	StartsAt time.Time
	EndsAt   time.Time
}

// SeedFutureSlots semeia, para cada offset D, um plantão AVAILABLE em hoje+D
// 09:00–12:00 (America/Sao_Paulo) com DOIS slots de 25 minutos (09:00 e 09:30)
// no mock do legado. Devolve os slots por offset, na ordem (09:00, 09:30).
//
// Fuso, espelhando o init.sql e o adapter: as colunas do legado são DATETIME
// ingênuo interpretado como hora de parede de America/Sao_Paulo. Os instantes
// são calculados AQUI, nesse fuso, e o driver (o RootDSN leva
// parseTime=true&loc=America%2FSao_Paulo) os grava como o literal de parede
// certo — assim o instante semeado e o instante lido pela API são IGUAIS, e o
// teste pode comparar sem tolerância.
//
// Ids determinísticos por (professional, offset, slot#), com prefixo "e2e-" para
// nunca colidir com os seeds do init.sql. Usa o RootDSN: o usuário restrito da
// aplicação não tem INSERT (e é exatamente isso que queremos continuar testando).
func SeedFutureSlots(t *testing.T, rootDSN string, professionalID string, offsets []int) map[int][]SeededSlot {
	t.Helper()

	loc, err := time.LoadLocation("America/Sao_Paulo")
	require.NoError(t, err, "carregar America/Sao_Paulo")

	db, err := sql.Open("mysql", rootDSN)
	require.NoError(t, err, "abrir conexão root no mysql legado")
	defer func() { _ = db.Close() }()

	now := time.Now().In(loc)
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	prefix := professionalID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}

	out := make(map[int][]SeededSlot, len(offsets))
	for _, d := range offsets {
		day := base.AddDate(0, 0, d)
		shiftID := fmt.Sprintf("e2e-%s-%03d-shift", prefix, d)
		shiftStart := day.Add(9 * time.Hour)
		shiftEnd := day.Add(12 * time.Hour)

		_, err := db.Exec(
			"INSERT INTO tb_shifts (id, professionalId, status, startsAt, endsAt) VALUES (?, ?, 'AVAILABLE', ?, ?)",
			shiftID, professionalID, shiftStart, shiftEnd)
		require.NoError(t, err, "semear plantão %s", shiftID)

		for n := 0; n < 2; n++ {
			start := shiftStart.Add(time.Duration(n) * 30 * time.Minute)
			end := start.Add(25 * time.Minute)
			slotID := fmt.Sprintf("e2e-%s-%03d-slot-%d", prefix, d, n+1)

			_, err := db.Exec(
				"INSERT INTO tb_slots (id, shiftId, booked, startsAt, endsAt) VALUES (?, ?, 0, ?, ?)",
				slotID, shiftID, start, end)
			require.NoError(t, err, "semear slot %s", slotID)

			out[d] = append(out[d], SeededSlot{ID: slotID, StartsAt: start, EndsAt: end})
		}
	}
	return out
}
