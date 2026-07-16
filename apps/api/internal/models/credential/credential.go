// Package credential cuida da senha do paciente: política, hash e verificação.
//
// É um pacote PURO (mesma regra do models/eligibility): sem I/O, sem banco, sem
// time.Now(). A única fonte externa é o gerador de aleatoriedade do salt.
//
// Algoritmo: Argon2id, com os parâmetros mínimos recomendados pela OWASP
// (19 MiB, 2 iterações, 1 thread). O hash é serializado no formato PHC, que
// carrega os próprios parâmetros — por isso o Verify os LÊ do hash em vez de
// assumir os atuais. Sem isso, subir o custo amanhã invalidaria toda senha já
// cadastrada.
package credential

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

var (
	// ErrMismatch: a senha não confere. É o caminho normal de um login errado.
	ErrMismatch = errors.New("senha não confere")
	// ErrMalformedHash: o hash gravado é ilegível — bug ou corrupção nossa, não
	// senha errada. Distinguir importa: colapsar isto em ErrMismatch esconderia
	// dado corrompido no banco atrás de um "401" cotidiano.
	ErrMalformedHash = errors.New("hash de senha malformado")
	// ErrWeakPassword: a senha não atende à política.
	ErrWeakPassword = errors.New("senha fora da política")
)

// Limites da política, em RUNAS (não bytes) — senão "é" custaria o dobro de "e"
// e uma senha acentuada de 12 caracteres seria injustamente barrada.
const (
	MinLength = 12
	// MaxLength existe por segurança, não por usabilidade: sem teto, uma senha
	// de vários MB faria o Argon2 queimar CPU sob demanda do atacante.
	MaxLength = 256
)

// Parâmetros do Argon2id para hashes NOVOS (OWASP, perfil de 19 MiB).
// Alterá-los é seguro: hashes antigos seguem verificando pelos seus próprios.
const (
	hashMemory  = 19 * 1024 // KiB
	hashTime    = 2
	hashThreads = 1
	saltLength  = 16
	keyLength   = 32
)

// CheckPolicy diz se a senha é aceitável. Só tamanho: regras de composição
// (maiúscula, símbolo...) empurram o usuário para "Senha1!" e a OWASP
// desaconselha. Comprimento é o que paga.
func CheckPolicy(password string) error {
	n := utf8.RuneCountInString(password)
	switch {
	case n < MinLength:
		return fmt.Errorf("%w: mínimo de %d caracteres", ErrWeakPassword, MinLength)
	case n > MaxLength:
		return fmt.Errorf("%w: máximo de %d caracteres", ErrWeakPassword, MaxLength)
	}
	return nil
}

// Hash gera o hash PHC de uma senha, com salt aleatório novo a cada chamada.
// Não valida a política — quem cadastra chama CheckPolicy antes.
func Hash(password string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("gerar salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, hashTime, hashMemory, hashThreads, keyLength)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, hashMemory, hashTime, hashThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// Verify confere a senha contra um hash PHC. Devolve nil no acerto,
// ErrMismatch no erro de senha e ErrMalformedHash se o hash não for legível.
func Verify(encoded, password string) error {
	p, salt, want, err := decode(encoded)
	if err != nil {
		return err
	}

	got := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, uint32(len(want)))

	// Comparação em tempo constante: um bytes.Equal aqui vazaria, pelo tempo,
	// quantos bytes do hash o atacante já acertou.
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrMismatch
	}
	return nil
}

type params struct {
	memory  uint32
	time    uint32
	threads uint8
}

// decode desmonta "$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>".
func decode(encoded string) (params, []byte, []byte, error) {
	var zero params

	// O split de uma string iniciada por "$" produz um primeiro campo vazio.
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return zero, nil, nil, fmt.Errorf("%w: esperava 6 campos, veio %d", ErrMalformedHash, len(parts))
	}
	if parts[1] != "argon2id" {
		return zero, nil, nil, fmt.Errorf("%w: algoritmo %q não suportado", ErrMalformedHash, parts[1])
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return zero, nil, nil, fmt.Errorf("%w: versão ilegível", ErrMalformedHash)
	}
	if version != argon2.Version {
		return zero, nil, nil, fmt.Errorf("%w: versão %d não suportada", ErrMalformedHash, version)
	}

	var p params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return zero, nil, nil, fmt.Errorf("%w: parâmetros ilegíveis", ErrMalformedHash)
	}

	// Faixa segura ANTES de chamar o argon2. Ele valida `time >= 1` e
	// `threads >= 1` com PÂNICO, não com erro — um hash corrompido no banco
	// derrubaria a goroutine do request em vez de virar um 500 limpo. O teto de
	// memória evita que um `m=99999999` vire uma alocação de ~95 GiB.
	const maxMemory = 1 << 20 // 1 GiB em KiB
	if p.time < 1 || p.threads < 1 || p.memory < 8 || p.memory > maxMemory {
		return zero, nil, nil, fmt.Errorf("%w: parâmetros fora da faixa segura (m=%d,t=%d,p=%d)",
			ErrMalformedHash, p.memory, p.time, p.threads)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return zero, nil, nil, fmt.Errorf("%w: salt não é base64", ErrMalformedHash)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return zero, nil, nil, fmt.Errorf("%w: hash não é base64", ErrMalformedHash)
	}

	// Guarda contra hash truncado: sem isto, um salt/hash vazio chegaria ao
	// argon2.IDKey, que entra em pânico em vez de devolver erro.
	if len(salt) < 8 || len(want) < 16 {
		return zero, nil, nil, fmt.Errorf("%w: salt ou hash curto demais", ErrMalformedHash)
	}

	return p, salt, want, nil
}
