package cpf_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/renovisaude/renovi-care/internal/models/cpf"
)

// pepperTeste é a chave usada nos vetores conhecidos abaixo. Os hexes esperados
// NÃO foram gerados por este código: vêm do openssl
// (`printf '%s' <cpf> | openssl dgst -sha256 -hmac <pepper> -r`), uma
// implementação independente de HMAC. É isso que impede o teste de ser
// tautológico — ele discorda do código se a nossa implementação divergir do
// padrão.
var pepperTeste = []byte("pepper-de-teste-renovi")

func TestHMAC_VetoresConhecidos(t *testing.T) {
	tests := []struct {
		nome    string
		cpf     string
		pepper  []byte
		wantHex string
	}{
		{
			nome:    "cpf de exemplo com pepper de teste",
			cpf:     "94819089846",
			pepper:  pepperTeste,
			wantHex: "73b1ae4ebdf25ea1f52c39ee411c9d26820d9fd5fc9f488e783d063a8a1a79ad",
		},
		{
			nome:    "cpf com zeros à esquerda muda o digest",
			cpf:     "00000003700",
			pepper:  pepperTeste,
			wantHex: "92a9e4f1e547abcb81ea63c31c6bfb3dd2c81b7442d5df973e0257ad8e321f2d",
		},
		{
			nome:    "mesmo cpf com pepper diferente muda o digest",
			cpf:     "94819089846",
			pepper:  []byte("outro-pepper"),
			wantHex: "79c7248dff90d67a88713020af6245939aef961934da044989a3259f4101a2b4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			c, err := cpf.Parse(tt.cpf)
			if err != nil {
				t.Fatalf("Parse(%q) falhou no setup: %v", tt.cpf, err)
			}
			got, err := c.HMAC(tt.pepper)
			if err != nil {
				t.Fatalf("HMAC devolveu erro inesperado: %v", err)
			}
			want, _ := hex.DecodeString(tt.wantHex)
			if !bytes.Equal(got, want) {
				t.Errorf("HMAC = %x; quero %s", got, tt.wantHex)
			}
		})
	}
}

func TestHMAC_Tem32Bytes(t *testing.T) {
	c, err := cpf.Parse("94819089846")
	if err != nil {
		t.Fatalf("Parse falhou no setup: %v", err)
	}
	got, err := c.HMAC(pepperTeste)
	if err != nil {
		t.Fatalf("HMAC devolveu erro inesperado: %v", err)
	}
	// 32 bytes = SHA-256; casa com o CHECK (octet_length = 32) das colunas bytea.
	if len(got) != 32 {
		t.Errorf("HMAC tem %d bytes; quero 32", len(got))
	}
}

func TestHMAC_Deterministico(t *testing.T) {
	c, err := cpf.Parse("94819089846")
	if err != nil {
		t.Fatalf("Parse falhou no setup: %v", err)
	}
	a, err := c.HMAC(pepperTeste)
	if err != nil {
		t.Fatalf("HMAC (1ª) devolveu erro: %v", err)
	}
	b, err := c.HMAC(pepperTeste)
	if err != nil {
		t.Fatalf("HMAC (2ª) devolveu erro: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("HMAC não é determinístico: %x != %x", a, b)
	}
}

func TestHMAC_PepperVazioEhErro(t *testing.T) {
	c, err := cpf.Parse("94819089846")
	if err != nil {
		t.Fatalf("Parse falhou no setup: %v", err)
	}
	for _, pepper := range [][]byte{nil, {}} {
		// Um HMAC com chave vazia é um digest público (qualquer um recalcula) —
		// exatamente o que o pepper existe para impedir. Falhar aqui evita que um
		// deploy sem RENOVI_CPF_PEPPER grave hashes reversíveis em silêncio.
		if _, err := c.HMAC(pepper); !errors.Is(err, cpf.ErrNoPepper) {
			t.Errorf("HMAC(pepper vazio %v) = %v; quero ErrNoPepper", pepper, err)
		}
	}
}
