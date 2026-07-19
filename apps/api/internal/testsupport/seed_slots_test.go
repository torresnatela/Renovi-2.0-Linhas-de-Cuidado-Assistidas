//go:build integration

package testsupport

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSeedLegacySlotsIsIdempotent prova mecanicamente que
// deploy/mysql-legacy/seed-slots.sql (o script por trás do `make
// seed-legacy-slots`) pode rodar mais de uma vez sem duplicar linhas nem
// falhar — em vez de confiar de olho no `INSERT IGNORE`.
//
// Sobe um MySQL efêmero com o schema real (o mesmo init.sql que StartMySQL usa
// para os outros testes de integração), executa o seed DUAS vezes seguidas e
// confere, depois de cada execução: a contagem de plantões e de slots livres
// futuros por profissional bate com o esperado (7 offsets para a Ana, 3 para o
// Bruno, 2 slots por offset) — e a segunda execução não muda nada.
func TestSeedLegacySlotsIsIdempotent(t *testing.T) {
	legacy := StartMySQL(t)

	script, err := os.ReadFile(seedSlotsPath(t))
	require.NoError(t, err, "ler deploy/mysql-legacy/seed-slots.sql")

	// multiStatements=true: o arquivo tem várias instruções separadas por ';',
	// como o `mysql` client do alvo `seed-legacy-slots` já manda de uma vez.
	db, err := sql.Open("mysql", legacy.RootDSN+"&multiStatements=true")
	require.NoError(t, err, "abrir conexão root (multiStatements) no mysql legado")
	defer func() { _ = db.Close() }()

	runSeed := func() {
		_, err := db.Exec(string(script))
		require.NoError(t, err, "executar seed-slots.sql")
	}

	runSeed()
	assertSeedState(t, db)

	// De novo, no MESMO dia: o INSERT IGNORE não deveria duplicar nada.
	runSeed()
	assertSeedState(t, db)
}

// assertSeedState confere as contagens que o roteiro manual (slice1.http)
// depende: 1 plantão + 2 slots livres por offset, para cada profissional.
func assertSeedState(t *testing.T, db *sql.DB) {
	t.Helper()

	assertCount := func(query string, want int, msg string) {
		t.Helper()
		var got int
		require.NoError(t, db.QueryRow(query).Scan(&got), msg)
		require.Equal(t, want, got, msg)
	}

	const anaOffsets = 7   // +2, +9, +10, +16, +23, +30, +44
	const brunoOffsets = 3 // +5, +37, +68

	assertCount(
		"SELECT COUNT(*) FROM tb_shifts WHERE id LIKE 'manual-a-%'",
		anaOffsets, "plantões da Ana (um por offset, sem duplicar)")
	assertCount(
		"SELECT COUNT(*) FROM tb_shifts WHERE id LIKE 'manual-b-%'",
		brunoOffsets, "plantões do Bruno (um por offset, sem duplicar)")
	assertCount(
		"SELECT COUNT(*) FROM tb_slots WHERE id LIKE 'manual-a-%'",
		anaOffsets*2, "2 slots por offset da Ana, sem duplicar")
	assertCount(
		"SELECT COUNT(*) FROM tb_slots WHERE id LIKE 'manual-b-%'",
		brunoOffsets*2, "2 slots por offset do Bruno, sem duplicar")

	// Livres (booked=0) e FUTUROS: exatamente o que a disponibilidade da API
	// ofereceria. NOW() no fuso do servidor difere de America/Sao_Paulo por
	// só algumas horas — irrelevante porque o offset mínimo semeado é +2 dias.
	assertCount(
		`SELECT COUNT(*) FROM tb_slots
		   WHERE id LIKE 'manual-a-%' AND booked = 0 AND startsAt > NOW()`,
		anaOffsets*2, "todos os slots semeados da Ana estão livres e no futuro")
	assertCount(
		`SELECT COUNT(*) FROM tb_slots
		   WHERE id LIKE 'manual-b-%' AND booked = 0 AND startsAt > NOW()`,
		brunoOffsets*2, "todos os slots semeados do Bruno estão livres e no futuro")
}

// seedSlotsPath resolve o caminho de deploy/mysql-legacy/seed-slots.sql a
// partir DESTE arquivo — mesmo racional do legacyInitScript em mysql.go: o CWD
// do `go test` é o pacote do teste, não a raiz do repo.
func seedSlotsPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "descobrir o caminho deste arquivo")
	// thisFile = <raiz>/apps/api/internal/testsupport/seed_slots_test.go
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	path, err := filepath.Abs(filepath.Join(repoRoot, "deploy", "mysql-legacy", "seed-slots.sql"))
	require.NoError(t, err, fmt.Sprintf("resolver caminho absoluto a partir de %s", thisFile))
	return path
}
