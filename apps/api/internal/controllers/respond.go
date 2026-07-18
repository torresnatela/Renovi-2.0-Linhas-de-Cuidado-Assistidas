// Package controllers contém os handlers HTTP (o "C" do MVC): recebem a
// requisição, chamam os models e escrevem a resposta. Regra de negócio e acesso
// a dados ficam nos models — o controller apenas orquestra e traduz para HTTP.
package controllers

import (
	"encoding/json"
	"log/slog"
	"net/http"
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

// Problem é o corpo de erro no formato RFC 7807 (application/problem+json).
type Problem struct {
	Type   string `json:"type,omitempty"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
	// Membro de extensão, previsto pela própria RFC 7807 (§3.2).
	Reason *Reason `json:"reason,omitempty"`
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

func writeProblem(w http.ResponseWriter, p Problem) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}
