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

// Problem é o corpo de erro no formato RFC 7807 (application/problem+json).
type Problem struct {
	Type   string `json:"type,omitempty"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// WriteProblem escreve um erro padronizado (RFC 7807).
func WriteProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{Title: title, Status: status, Detail: detail})
}
