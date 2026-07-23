package http

import (
	"strings"
	"testing"
)

// O token do convite de onboarding viaja no PATH (é a credencial da rota). O log de
// request NÃO pode gravá-lo — mesma disciplina do LogNotifier. redactSensitivePath
// troca o token por <redacted>, mantendo o sufixo de ação para observabilidade.
func TestRedactSensitivePath(t *testing.T) {
	const token = "SEGREDO-DO-TOKEN-DE-ONBOARDING"

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"info", "/api/v1/onboarding/" + token, "/api/v1/onboarding/<redacted>"},
		{"complete", "/api/v1/onboarding/" + token + "/complete", "/api/v1/onboarding/<redacted>/complete"},
		{"decline", "/api/v1/onboarding/" + token + "/decline", "/api/v1/onboarding/<redacted>/decline"},
		{"sem token", "/api/v1/onboarding/", "/api/v1/onboarding/"},
		{"outra rota intacta", "/api/v1/auth/login", "/api/v1/auth/login"},
		{"cpf_hmac do resend não é redigido aqui", "/api/v1/integration/gestao/employees/abc/resend-invite", "/api/v1/integration/gestao/employees/abc/resend-invite"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactSensitivePath(tc.in)
			if got != tc.want {
				t.Fatalf("redactSensitivePath(%q) = %q, quero %q", tc.in, got, tc.want)
			}
			if strings.Contains(got, token) {
				t.Errorf("o token vazou no path redigido: %q", got)
			}
		})
	}
}
