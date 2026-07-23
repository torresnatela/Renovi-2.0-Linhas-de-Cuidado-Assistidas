// Package notify entrega os convites de onboarding da ingestão da Gestão.
//
// No piloto a entrega é um STUB (LogNotifier): o invite_url volta na resposta da
// API e o gestor o repassa (WhatsApp). Um adapter de e-mail/SMTP real entra depois
// implementando a mesma InviteMessage. A interface do consumidor (Notifier) é
// declarada no model (ADR-012); aqui vive só o tipo da mensagem e a implementação.
package notify

import (
	"context"
	"log/slog"
	"time"
)

// InviteMessage é o convite a entregar. NÃO carrega CPF nem dado de saúde. A
// InviteURL contém o token de onboarding (credencial portadora) — trate-a como
// segredo: nunca a coloque em log.
type InviteMessage struct {
	Name      string
	Email     string
	Phone     string
	InviteURL string
	ExpiresAt time.Time
}

// LogNotifier é o stub do piloto: registra que um convite foi emitido, sem entregar
// nada e sem vazar a URL/token. O zero-value é utilizável (cai no slog default).
type LogNotifier struct {
	logger *slog.Logger
}

// NewLogNotifier monta o stub com o logger dado.
func NewLogNotifier(logger *slog.Logger) LogNotifier {
	return LogNotifier{logger: logger}
}

// SendInvite "entrega" o convite: no stub, só loga o evento. Nunca erra (a entrega
// real, quando existir, é que poderá falhar).
func (n LogNotifier) SendInvite(_ context.Context, msg InviteMessage) error {
	logger := n.logger
	if logger == nil {
		logger = slog.Default()
	}
	// Só metadados não sensíveis: se havia contato e quando o convite expira. Nem a
	// URL (token), nem nome/e-mail em claro.
	logger.Info("convite de onboarding emitido (stub — entrega real pendente)",
		"tem_email", msg.Email != "",
		"tem_telefone", msg.Phone != "",
		"expira_em", msg.ExpiresAt)
	return nil
}
