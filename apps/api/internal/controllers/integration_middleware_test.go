package controllers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/renovisaude/renovi-care/internal/controllers"
)

func TestRequireIntegrationToken(t *testing.T) {
	const token = "token-de-integracao-da-gestao"

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := controllers.RequireIntegrationToken(token)(next)
	const path = "/integration/gestao/contracts"

	t.Run("sem header é 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, quero 401", w.Code)
		}
		if !strings.Contains(w.Body.String(), "INTEGRATION_TOKEN_INVALID") {
			t.Errorf("reason ausente: %s", w.Body.String())
		}
	})

	t.Run("token errado é 401 com o mesmo reason", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set(controllers.IntegrationTokenHeader, "token-errado")
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, quero 401", w.Code)
		}
		if !strings.Contains(w.Body.String(), "INTEGRATION_TOKEN_INVALID") {
			t.Errorf("reason ausente: %s", w.Body.String())
		}
	})

	t.Run("token nunca aparece na resposta", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set(controllers.IntegrationTokenHeader, "token-errado")
		handler.ServeHTTP(w, req)
		if strings.Contains(w.Body.String(), token) {
			t.Errorf("o token não pode vazar na resposta: %s", w.Body.String())
		}
	})

	t.Run("token certo chama o next", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set(controllers.IntegrationTokenHeader, token)
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, quero 204 (next chamado)", w.Code)
		}
	})

	t.Run("token vazio (integração desligada) recusa mesmo com header vazio", func(t *testing.T) {
		off := controllers.RequireIntegrationToken("")(next)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set(controllers.IntegrationTokenHeader, "")
		off.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, quero 401 (dois vazios não podem casar)", w.Code)
		}
	})
}
