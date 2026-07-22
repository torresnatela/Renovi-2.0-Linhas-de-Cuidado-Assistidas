package models

import (
	"bytes"
	"errors"
	"testing"
)

// A matriz de decisão do convite é a lógica sensível da ingestão (person_status,
// paciente já existe, já há convite vivo). Ela é PURA e vive aqui como unidade; a
// orquestração SQL em volta é coberta pelo teste de integração.
func TestDecideInvite(t *testing.T) {
	tests := []struct {
		nome            string
		personStatus    string
		patientExists   bool
		liveTokenExists bool
		wantAction      inviteAction
		wantSent        bool
		wantEvent       string
	}{
		{
			nome:         "pessoa nova, sem conta e sem convite: cunha e envia",
			personStatus: "pendente", patientExists: false, liveTokenExists: false,
			wantAction: actionMint, wantSent: true, wantEvent: "convite_emitido",
		},
		{
			nome:         "pendente com convite vivo: idempotente, não reemite",
			personStatus: "pendente", patientExists: false, liveTokenExists: true,
			wantAction: actionSuppressLiveToken, wantSent: false, wantEvent: "contrato_recebido",
		},
		{
			nome:         "pendente mas já tem conta (cpf_match): defere ao consentimento",
			personStatus: "pendente", patientExists: true, liveTokenExists: false,
			wantAction: actionSuppressCPFMatch, wantSent: false, wantEvent: "cpf_match_pendente",
		},
		{
			nome:         "conta existente tem prioridade sobre um convite vivo",
			personStatus: "pendente", patientExists: true, liveTokenExists: true,
			wantAction: actionSuppressCPFMatch, wantSent: false, wantEvent: "cpf_match_pendente",
		},
		{
			nome:         "já vinculado: onboarding não faz sentido",
			personStatus: "vinculado", patientExists: false, liveTokenExists: false,
			wantAction: actionSuppressLinked, wantSent: false, wantEvent: "contrato_recebido",
		},
		{
			nome:         "vinculado com conta detectada: ainda suprime",
			personStatus: "vinculado", patientExists: true, liveTokenExists: false,
			wantAction: actionSuppressLinked, wantSent: false, wantEvent: "contrato_recebido",
		},
	}

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			got := decideInvite(tt.personStatus, tt.patientExists, tt.liveTokenExists)
			if got != tt.wantAction {
				t.Fatalf("decideInvite = %v; quero %v", got, tt.wantAction)
			}
			if got.inviteSent() != tt.wantSent {
				t.Errorf("inviteSent = %v; quero %v", got.inviteSent(), tt.wantSent)
			}
			if got.eventType() != tt.wantEvent {
				t.Errorf("eventType = %q; quero %q", got.eventType(), tt.wantEvent)
			}
		})
	}
}

func TestValidateContractPush(t *testing.T) {
	valida := func() ContractPush {
		return ContractPush{
			ContractID: "C-1", Status: "ativo",
			Employee: EmployeePush{ID: "E-1", CPFHmac: bytes.Repeat([]byte{0x01}, 32), Name: "Maria"},
			Company:  CompanyPush{ID: "CO-1", DisplayName: "ACME"},
		}
	}

	if err := validateContractPush(valida()); err != nil {
		t.Fatalf("push válido recusado: %v", err)
	}

	invalidas := map[string]func(*ContractPush){
		"cpf_hmac curto":     func(p *ContractPush) { p.Employee.CPFHmac = []byte("curto") },
		"cpf_hmac nil":       func(p *ContractPush) { p.Employee.CPFHmac = nil },
		"status inválido":    func(p *ContractPush) { p.Status = "ferias" },
		"contract_id vazio":  func(p *ContractPush) { p.ContractID = "" },
		"employee id vazio":  func(p *ContractPush) { p.Employee.ID = "" },
		"nome vazio":         func(p *ContractPush) { p.Employee.Name = "  " },
		"company id vazio":   func(p *ContractPush) { p.Company.ID = "" },
		"display_name vazio": func(p *ContractPush) { p.Company.DisplayName = "" },
	}
	for nome, quebra := range invalidas {
		t.Run(nome, func(t *testing.T) {
			p := valida()
			quebra(&p)
			err := validateContractPush(p)
			if !errors.Is(err, ErrInvalidContractPush) {
				t.Errorf("validateContractPush(%s) = %v; quero ErrInvalidContractPush", nome, err)
			}
		})
	}
}

func TestInviteURL(t *testing.T) {
	tests := []struct {
		base string
		raw  string
		want string
	}{
		{"https://app.test", "ABC", "https://app.test/onboarding/ABC"},
		{"https://app.test/", "ABC", "https://app.test/onboarding/ABC"}, // barra final não duplica
	}
	for _, tt := range tests {
		s := &GestaoIngestionStore{webBaseURL: tt.base}
		if got := s.inviteURL(tt.raw); got != tt.want {
			t.Errorf("inviteURL(%q, %q) = %q; quero %q", tt.base, tt.raw, got, tt.want)
		}
	}
}
