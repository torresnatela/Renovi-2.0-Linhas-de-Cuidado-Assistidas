package cpf_test

import (
	"errors"
	"testing"

	"github.com/renovisaude/renovi-care/internal/models/cpf"
)

func TestParse_Validos(t *testing.T) {
	tests := []struct {
		nome string
		raw  string
		want string
	}{
		{
			nome: "só dígitos",
			// CPF do exemplo do spec da DAV (/person/_/api-docs).
			raw:  "94819089846",
			want: "94819089846",
		},
		{
			nome: "formatado com pontos e traço",
			raw:  "948.190.898-46",
			want: "94819089846",
		},
		{
			nome: "com espaços nas bordas",
			raw:  "  94819089846  ",
			want: "94819089846",
		},
		{
			nome: "zeros à esquerda e DV 00 sobrevivem",
			// Zeros à esquerda + ambos os DV zero: pega implementação que
			// converte para int e perde os zeros pelo caminho.
			raw:  "00000003700",
			want: "00000003700",
		},
	}

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			got, err := cpf.Parse(tt.raw)
			if err != nil {
				t.Fatalf("Parse(%q) devolveu erro inesperado: %v", tt.raw, err)
			}
			if got.String() != tt.want {
				t.Errorf("Parse(%q) = %q; quero %q", tt.raw, got.String(), tt.want)
			}
		})
	}
}

func TestParse_Invalidos(t *testing.T) {
	tests := []struct {
		nome string
		raw  string
	}{
		{
			nome: "vazio",
			raw:  "",
		},
		{
			nome: "só espaços",
			raw:  "   ",
		},
		{
			nome: "curto demais",
			raw:  "9481908984",
		},
		{
			nome: "longo demais",
			raw:  "948190898461",
		},
		{
			nome: "contém letra",
			raw:  "9481908984a",
		},
		{
			nome: "primeiro DV errado",
			raw:  "94819089836",
		},
		{
			nome: "segundo DV errado",
			raw:  "94819089845",
		},
		{
			nome: "CPF do mock da Gestão tem DV inválido",
			// deploy/pg-gestao/init.sql semeia "Maria de Teste" com este CPF.
			// O 2º DV deveria ser 9. É um CPF de fachada, não um CPF válido.
			raw: "12345678901",
		},
		{
			nome: "todos os dígitos iguais passam no DV mas não são CPF",
			// 111.111.111-11 satisfaz o algoritmo do DV. Só uma regra
			// explícita o rejeita — por isso este caso existe.
			raw: "11111111111",
		},
		{
			nome: "zeros",
			raw:  "00000000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			_, err := cpf.Parse(tt.raw)
			if err == nil {
				t.Fatalf("Parse(%q) não devolveu erro; queria ErrInvalid", tt.raw)
			}
			if !errors.Is(err, cpf.ErrInvalid) {
				t.Errorf("Parse(%q) devolveu %v; quero que case com ErrInvalid", tt.raw, err)
			}
		})
	}
}
