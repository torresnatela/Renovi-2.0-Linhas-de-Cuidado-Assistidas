// Package cpf valida e normaliza o CPF (Cadastro de Pessoa Física).
//
// É um pacote PURO, na mesma regra do models/careline: sem I/O, sem banco,
// sem time.Now(). Tudo entra por parâmetro.
//
// Por que um tipo em vez de `func Validate(string) error`: com um tipo, o CPF
// inválido é irrepresentável. Quem recebe um cpf.CPF sabe que ele já passou
// pelo Parse — não dá para uma string crua do request escorrer até a DAV sem
// validação.
package cpf

import (
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalid classifica toda recusa do Parse. Use errors.Is para detectá-la; o
// texto que a acompanha explica o motivo e serve ao log, não ao usuário final
// (a resposta HTTP é genérica).
var ErrInvalid = errors.New("cpf inválido")

// ErrNoPepper recusa um HMAC sem chave. Um HMAC de chave vazia é um digest
// público (o espaço de CPF é pequeno e qualquer um o recalcula), então gravá-lo
// derrotaria o propósito do pepper. Falhar é melhor que gravar hash reversível.
var ErrNoPepper = errors.New("cpf: pepper vazio")

// CPF é um CPF já validado: exatamente 11 dígitos, com os dois dígitos
// verificadores conferidos. Guarde-o sem formatação — é assim que a DAV o
// espera (pattern ^\d{3}\d{3}\d{3}\d{2}$) e é assim que ele vai para o banco.
type CPF string

// String devolve os 11 dígitos, sem pontuação.
func (c CPF) String() string { return string(c) }

// HMAC devolve o HMAC-SHA256 dos 11 dígitos do CPF sob o pepper dado (32 bytes).
//
// É a chave pseudônima da pessoa na integração com a Gestão: o CPF em claro nunca
// trafega nem sai de patient_identity, mas dois lados que compartilham o mesmo
// pepper chegam ao mesmo digest e conseguem casar a pessoa (ADR-043).
//
// O pepper entra por parâmetro para manter o pacote puro (mesma regra do Parse):
// quem tem o segredo é a camada de config, não este tipo. Pepper vazio é erro
// (ErrNoPepper), não um HMAC sem chave.
func (c CPF) HMAC(pepper []byte) ([]byte, error) {
	if len(pepper) == 0 {
		return nil, ErrNoPepper
	}
	mac := hmac.New(sha256.New, pepper)
	mac.Write([]byte(c))
	return mac.Sum(nil), nil
}

// formatting são os únicos caracteres que aceitamos e descartamos. Não usamos
// "remova tudo que não for dígito" de propósito: isso transformaria "9481908984a"
// num CPF de 10 dígitos e o erro viraria "tamanho errado", escondendo a sujeira
// do dado de entrada.
var formatting = strings.NewReplacer(".", "", "-", "", " ", "")

// Parse valida e normaliza um CPF vindo do usuário. Aceita "948.190.898-46",
// "94819089846" e espaços nas bordas.
func Parse(raw string) (CPF, error) {
	s := formatting.Replace(strings.TrimSpace(raw))

	if len(s) != 11 {
		return "", fmt.Errorf("%w: esperava 11 dígitos, veio %d", ErrInvalid, len(s))
	}

	digits := make([]int, 11)
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return "", fmt.Errorf("%w: contém caractere não numérico", ErrInvalid)
		}
		digits[i] = int(s[i] - '0')
	}

	// Sequências repetidas (000..., 111...) satisfazem o algoritmo do DV, mas
	// não são CPF. Sem esta regra explícita, "11111111111" passaria.
	if allSame(digits) {
		return "", fmt.Errorf("%w: todos os dígitos são iguais", ErrInvalid)
	}

	if checkDigit(digits[:9], 10) != digits[9] || checkDigit(digits[:10], 11) != digits[10] {
		return "", fmt.Errorf("%w: dígito verificador não confere", ErrInvalid)
	}

	return CPF(s), nil
}

// checkDigit calcula um dígito verificador do CPF: soma ponderada com pesos
// decrescentes a partir de `weight`, resto de (soma*10) por 11, e o resto 10
// colapsa em 0.
func checkDigit(digits []int, weight int) int {
	sum := 0
	for i, d := range digits {
		sum += d * (weight - i)
	}
	if r := (sum * 10) % 11; r < 10 {
		return r
	}
	return 0
}

func allSame(digits []int) bool {
	for _, d := range digits[1:] {
		if d != digits[0] {
			return false
		}
	}
	return true
}
