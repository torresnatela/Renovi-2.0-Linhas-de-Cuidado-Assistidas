package credential_test

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"

	"github.com/renovisaude/renovi-care/internal/models/credential"
)

const senhaBoa = "cavalo-bateria-grampo-correto"

func TestVerify_AceitaSenhaCorreta(t *testing.T) {
	encoded, err := credential.Hash(senhaBoa)
	if err != nil {
		t.Fatalf("Hash devolveu erro: %v", err)
	}

	if err := credential.Verify(encoded, senhaBoa); err != nil {
		t.Errorf("Verify recusou a senha correta: %v", err)
	}
}

func TestVerify_RejeitaSenhaErrada(t *testing.T) {
	encoded, err := credential.Hash(senhaBoa)
	if err != nil {
		t.Fatalf("Hash devolveu erro: %v", err)
	}

	err = credential.Verify(encoded, "cavalo-bateria-grampo-incorreto")
	if !errors.Is(err, credential.ErrMismatch) {
		t.Errorf("Verify com senha errada devolveu %v; quero ErrMismatch", err)
	}
}

// Salt aleatório: duas contas com a mesma senha não podem compartilhar hash,
// senão um vazamento do banco entrega quem repetiu senha.
func TestHash_MesmaSenhaGeraHashesDiferentes(t *testing.T) {
	a, err := credential.Hash(senhaBoa)
	if err != nil {
		t.Fatalf("Hash devolveu erro: %v", err)
	}
	b, err := credential.Hash(senhaBoa)
	if err != nil {
		t.Fatalf("Hash devolveu erro: %v", err)
	}

	if a == b {
		t.Error("dois Hash da mesma senha produziram o mesmo valor; o salt não é aleatório")
	}
	// Ambos precisam continuar verificando.
	if err := credential.Verify(a, senhaBoa); err != nil {
		t.Errorf("Verify(a) falhou: %v", err)
	}
	if err := credential.Verify(b, senhaBoa); err != nil {
		t.Errorf("Verify(b) falhou: %v", err)
	}
}

func TestHash_NaoVazaSenhaEmClaro(t *testing.T) {
	encoded, err := credential.Hash(senhaBoa)
	if err != nil {
		t.Fatalf("Hash devolveu erro: %v", err)
	}

	if strings.Contains(encoded, senhaBoa) {
		t.Errorf("o hash contém a senha em claro: %q", encoded)
	}
	if !strings.HasPrefix(encoded, "$argon2id$") {
		t.Errorf("hash = %q; quero uma string PHC começando com $argon2id$", encoded)
	}
}

// Verify precisa LER os parâmetros do hash, não assumir os nossos. Sem isso,
// subir o custo do Argon2 amanhã invalidaria toda senha já cadastrada.
func TestVerify_UsaOsParametrosGravadosNoHash(t *testing.T) {
	// Parâmetros propositalmente diferentes dos nossos (custo mínimo).
	const (
		memory  = 8
		time    = 1
		threads = 1
		keyLen  = 32
	)
	salt := []byte("salt-de-16-bytes")
	key := argon2.IDKey([]byte(senhaBoa), salt, time, memory, threads, keyLen)

	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memory, time, threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)

	if err := credential.Verify(encoded, senhaBoa); err != nil {
		t.Errorf("Verify recusou hash com parâmetros alheios: %v", err)
	}
	if err := credential.Verify(encoded, "senha-errada"); !errors.Is(err, credential.ErrMismatch) {
		t.Errorf("Verify com senha errada devolveu %v; quero ErrMismatch", err)
	}
}

func TestVerify_HashMalformado(t *testing.T) {
	tests := []struct {
		nome    string
		encoded string
	}{
		{"vazio", ""},
		{"não é PHC", "só-uma-string-qualquer"},
		{"algoritmo desconhecido", "$bcrypt$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA"},
		{"argon2i em vez de argon2id", "$argon2i$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaA"},
		{"sem o campo de hash", "$argon2id$v=19$m=19456,t=2,p=1$c2FsdA"},
		{"parâmetros não numéricos", "$argon2id$v=19$m=abc,t=2,p=1$c2FsdA$aGFzaA"},
		{"salt não é base64", "$argon2id$v=19$m=19456,t=2,p=1$!!!$aGFzaA"},
		{"versão não numérica", "$argon2id$v=xx$m=19456,t=2,p=1$c2FsdA$aGFzaA"},
		// Os três abaixo fazem argon2.IDKey entrar em PÂNICO se chegarem até ele:
		// ele valida rounds>=1 e parallelism>=1 com panic, não com erro.
		{"t=0 faz o argon2 entrar em pânico", "$argon2id$v=19$m=19456,t=0,p=1$c2FsdGRlMTZieXRlcw$aGFzaGRlMzJieXRlc2FxdWlvazEyMzQ1"},
		{"p=0 faz o argon2 entrar em pânico", "$argon2id$v=19$m=19456,t=2,p=0$c2FsdGRlMTZieXRlcw$aGFzaGRlMzJieXRlc2FxdWlvazEyMzQ1"},
		{"memória absurda viraria alocação de GB", "$argon2id$v=19$m=99999999,t=2,p=1$c2FsdGRlMTZieXRlcw$aGFzaGRlMzJieXRlc2FxdWlvazEyMzQ1"},
	}

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			err := credential.Verify(tt.encoded, senhaBoa)
			if err == nil {
				t.Fatal("Verify aceitou um hash malformado")
			}
			// Malformado é bug/corrupção nossa, não senha errada. Distinguir
			// importa: um ErrMismatch aqui esconderia dado corrompido no banco.
			if !errors.Is(err, credential.ErrMalformedHash) {
				t.Errorf("Verify devolveu %v; quero ErrMalformedHash", err)
			}
		})
	}
}

func TestCheckPolicy(t *testing.T) {
	tests := []struct {
		nome    string
		senha   string
		quebrou bool
	}{
		{"vazia", "", true},
		{"11 caracteres", strings.Repeat("a", 11), true},
		{"12 caracteres é o mínimo", strings.Repeat("a", 12), false},
		{"frase longa", senhaBoa, false},
		{"no limite máximo", strings.Repeat("a", 256), false},
		{"acima do limite máximo", strings.Repeat("a", 257), true},
		{"unicode conta em runas, não em bytes", strings.Repeat("é", 12), false},
	}

	for _, tt := range tests {
		t.Run(tt.nome, func(t *testing.T) {
			err := credential.CheckPolicy(tt.senha)
			if tt.quebrou && !errors.Is(err, credential.ErrWeakPassword) {
				t.Errorf("CheckPolicy devolveu %v; quero ErrWeakPassword", err)
			}
			if !tt.quebrou && err != nil {
				t.Errorf("CheckPolicy recusou senha válida: %v", err)
			}
		})
	}
}
