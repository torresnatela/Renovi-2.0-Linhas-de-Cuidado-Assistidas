// Package controllers contém os handlers HTTP (o "C" do MVC): recebem a
// requisição, chamam os models e escrevem a resposta. Regra de negócio e acesso
// a dados ficam nos models — o controller apenas orquestra e traduz para HTTP.
package controllers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// WriteJSON serializa v como JSON com o status informado.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("falha ao serializar resposta JSON", "error", err)
	}
}

// Reason é o motivo máquina-legível de um erro.
//
// Existe porque "cedo demais" e "horário tomado" podem ser o MESMO status HTTP, e
// o front reage diferente a cada um. Casar pelo `detail` seria casar por frase
// escrita para humano — que muda no dia em que alguém melhorar o texto. É o mesmo
// `Reason` do motor de elegibilidade: uma tabela de tradução só no front, servindo
// erro e veredito.
type Reason struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

// ProblemBlock é um bloqueio do motor de elegibilidade já no formato de resposta.
//
// É um tipo local, e NÃO o `api.EligibilityBlock` gerado, de propósito: respond.go
// escreve erros e não deve depender do pacote de tipos gerado (nem do ciclo de
// geração) só para montar um corpo. O JSON é idêntico ao do schema EligibilityBlock
// do contrato — quem consome não vê a diferença.
type ProblemBlock struct {
	RuleType      string     `json:"rule_type"`
	Reason        string     `json:"reason"`
	AvailableFrom *time.Time `json:"available_from,omitempty"`
}

// Problem é o corpo de erro no formato RFC 7807 (application/problem+json).
type Problem struct {
	Type   string `json:"type,omitempty"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
	// Membro de extensão, previsto pela própria RFC 7807 (§3.2).
	Reason *Reason `json:"reason,omitempty"`
	// Blocks são os bloqueios do motor de elegibilidade — presente no 422 de
	// agendamento barrado. Também é membro de extensão (§3.2).
	Blocks []ProblemBlock `json:"blocks,omitempty"`
	// Errors é a lista de erros de validação — presente no 400 de publicação de
	// linha, onde o admin quer corrigir tudo de uma vez. Membro de extensão (§3.2).
	Errors []string `json:"errors,omitempty"`
}

// WriteProblem escreve um erro padronizado (RFC 7807).
func WriteProblem(w http.ResponseWriter, status int, title, detail string) {
	writeProblem(w, Problem{Title: title, Status: status, Detail: detail})
}

// WriteProblemReason é o WriteProblem com um motivo máquina-legível. Use quando o
// cliente precisar DECIDIR algo a partir do erro, e não só exibi-lo.
func WriteProblemReason(w http.ResponseWriter, status int, title, detail string, reason Reason) {
	writeProblem(w, Problem{Title: title, Status: status, Detail: detail, Reason: &reason})
}

// WriteProblemFull escreve um Problem já montado pelo chamador, INCLUSIVE com os
// membros de extensão (`blocks`, `errors`, `reason`). Use quando os writers acima
// não bastarem: o 422 de elegibilidade (com `Blocks`) e o 400 de publicação (com
// `Errors`). Reusa o writer interno para manter content-type e serialização num só
// lugar.
func WriteProblemFull(w http.ResponseWriter, p Problem) {
	writeProblem(w, p)
}

func writeProblem(w http.ResponseWriter, p Problem) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}
