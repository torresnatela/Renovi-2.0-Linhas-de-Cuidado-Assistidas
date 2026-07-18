//go:build integration

package testsupport

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"
)

// LegacyMySQL são as duas portas de entrada do legado efêmero.
//
// São duas porque o mock aplica a MESMA restrição de privilégio da produção: o
// usuário da aplicação só pode SELECT + UPDATE(booked, updatedAt) em tb_slots.
// Isso é exatamente o que queremos testar — mas um teste também precisa MONTAR
// cenário (semear slot, simular reserva do app legado), e para isso ele entra
// como root, por fora da aplicação. Se o teste semeasse com o usuário da
// aplicação, ou não conseguiria montar nada, ou teríamos que afrouxar a permissão
// e deixar de testar a coisa certa.
type LegacyMySQL struct {
	// AppDSN é o usuário RESTRITO. É com ele que o adapter roda.
	//
	// Deliberadamente CRU: sem parseTime, sem loc. É o adapter que precisa forçar
	// os dois, e um DSN "já certo" aqui esconderia o bug de fuso em vez de
	// prová-lo.
	AppDSN string
	// RootDSN é para o teste montar cenário. Nunca para código de produção.
	RootDSN string
}

// StartMySQL sobe um MySQL efêmero com o schema REAL do legado (o mesmo
// deploy/mysql-legacy/init.sql que o `make up` usa) e o derruba no fim do teste.
//
// Para uma bateria inteira, prefira StartMySQLShared num TestMain: o container
// leva dezenas de segundos para subir, e um por teste transforma
// `make test-integration` em algo que ninguém roda.
func StartMySQL(t *testing.T) LegacyMySQL {
	t.Helper()

	legacy, stop, err := StartMySQLShared(context.Background())
	require.NoError(t, err, "subir container mysql")
	t.Cleanup(func() { _ = stop() })
	return legacy
}

// StartMySQLShared é o StartMySQL sem *testing.T, para ser chamado de um
// TestMain e compartilhado por todos os testes do pacote. Quem chama é dono de
// chamar o `stop`.
func StartMySQLShared(ctx context.Context) (LegacyMySQL, func() error, error) {
	script, err := legacyInitScript()
	if err != nil {
		return LegacyMySQL{}, nil, err
	}

	container, err := mysql.Run(ctx,
		"mysql:8.0",
		mysql.WithDatabase("renovi_legacy"),
		mysql.WithUsername("renovi"),
		mysql.WithPassword("renovi"),
		mysql.WithScripts(script),
		testcontainers.WithWaitStrategy(
			// UMA ocorrência, não duas (como é no Postgres deste mesmo pacote).
			//
			// O entrypoint do mysql:8 sobe um servidor TEMPORÁRIO para rodar o
			// initdb.d, mas com --skip-networking: ele não loga "port: 3306". A
			// linha só aparece quando o servidor definitivo abre para a rede, ou
			// seja, DEPOIS do init.sql. Esperar por duas ocorrências não deixa o
			// teste mais seguro — deixa ele estourar o timeout sempre.
			//
			// O timeout é generoso porque o init.sql cria o schema e semeia antes
			// de o servidor aparecer (medido: ~12s numa máquina ociosa).
			wait.ForLog("port: 3306  MySQL Community Server").
				WithOccurrence(1).
				WithStartupTimeout(180*time.Second),
		),
	)
	if err != nil {
		return LegacyMySQL{}, nil, err
	}
	stop := func() error { return container.Terminate(context.Background()) }

	host, err := container.Host(ctx)
	if err != nil {
		_ = stop()
		return LegacyMySQL{}, nil, err
	}
	port, err := container.MappedPort(ctx, "3306/tcp")
	if err != nil {
		_ = stop()
		return LegacyMySQL{}, nil, err
	}

	addr := fmt.Sprintf("%s:%s", host, port.Port())
	return LegacyMySQL{
		AppDSN:  fmt.Sprintf("renovi:renovi@tcp(%s)/renovi_legacy", addr),
		RootDSN: fmt.Sprintf("root:renovi@tcp(%s)/renovi_legacy?parseTime=true&loc=America%%2FSao_Paulo", addr),
	}, stop, nil
}

// legacyInitScript resolve o caminho do init.sql a partir DESTE arquivo, e não do
// diretório de trabalho.
//
// O `go test` roda com a CWD no pacote do TESTE, então um caminho relativo daqui
// mudaria de significado conforme quem chamasse (o adapter está 5 níveis abaixo
// da raiz; o testsupport, 4). runtime.Caller torna isso indiferente.
func legacyInitScript() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("testsupport: não consegui descobrir o caminho deste arquivo")
	}
	// thisFile = <raiz>/apps/api/internal/testsupport/mysql.go
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	return filepath.Abs(filepath.Join(repoRoot, "deploy", "mysql-legacy", "init.sql"))
}
