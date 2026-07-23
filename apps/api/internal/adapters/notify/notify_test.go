package notify_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/adapters/notify"
)

// O LogNotifier é o stub do piloto: ele NÃO entrega o convite (o invite_url volta
// na resposta da API para o gestor mandar por WhatsApp). Aqui garantimos que ele
// não vaza o token/URL no log — a URL de onboarding é uma credencial portadora,
// igual ao token de sessão, e log não é canal para credencial.
func TestLogNotifier_SendInvite_NaoVazaURL(t *testing.T) {
	var buf bytes.Buffer
	n := notify.NewLogNotifier(slog.New(slog.NewTextHandler(&buf, nil)))

	msg := notify.InviteMessage{
		Name:      "Maria de Teste",
		Email:     "maria@example.test",
		Phone:     "11999999999",
		InviteURL: "https://app.example/onboarding/SEGREDO-DO-TOKEN",
		ExpiresAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	}

	err := n.SendInvite(context.Background(), msg)
	require.NoError(t, err)

	logs := buf.String()
	assert.NotContains(t, logs, "SEGREDO-DO-TOKEN", "o token do convite não pode ir para o log")
	assert.NotContains(t, logs, "onboarding/SEGREDO", "a URL do convite não pode ir para o log")
	// Mas registra QUE um convite foi emitido — observabilidade sem vazar o segredo.
	assert.Contains(t, logs, "convite")
}

// Um Notifier nil não pode existir na fiação, mas o zero-value do LogNotifier
// (sem logger) não deve dar panic — cai no logger default.
func TestLogNotifier_SemLoggerNaoDaPanic(t *testing.T) {
	var n notify.LogNotifier
	require.NotPanics(t, func() {
		_ = n.SendInvite(context.Background(), notify.InviteMessage{Name: "X"})
	})
}
