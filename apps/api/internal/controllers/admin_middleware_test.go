package controllers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/renovisaude/renovi-care/internal/controllers"
)

func TestRequireAdminToken(t *testing.T) {
	const token = "token-secreto-de-operacao"

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := controllers.RequireAdminToken(token)(next)

	t.Run("sem header é 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/care-lines", nil))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, quero 401", w.Code)
		}
		if !strings.Contains(w.Body.String(), "ADMIN_TOKEN_INVALID") {
			t.Errorf("reason ausente: %s", w.Body.String())
		}
	})

	t.Run("token errado é 401", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/care-lines", nil)
		req.Header.Set(controllers.AdminTokenHeader, "token-errado")
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, quero 401", w.Code)
		}
		// Mesmo reason do caso "sem header": não damos oráculo de qual dos dois foi.
		if !strings.Contains(w.Body.String(), "ADMIN_TOKEN_INVALID") {
			t.Errorf("reason ausente: %s", w.Body.String())
		}
	})

	t.Run("token nunca aparece na resposta", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/care-lines", nil)
		req.Header.Set(controllers.AdminTokenHeader, "token-errado")
		handler.ServeHTTP(w, req)
		if strings.Contains(w.Body.String(), token) {
			t.Errorf("o token não pode vazar na resposta: %s", w.Body.String())
		}
	})

	t.Run("token certo chama o next", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/care-lines", nil)
		req.Header.Set(controllers.AdminTokenHeader, token)
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d, quero 204 (next chamado)", w.Code)
		}
	})

	t.Run("token vazio (admin desligado) recusa mesmo com header vazio", func(t *testing.T) {
		off := controllers.RequireAdminToken("")(next)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/admin/care-lines", nil)
		req.Header.Set(controllers.AdminTokenHeader, "")
		off.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, quero 401 (dois vazios não podem casar)", w.Code)
		}
	})
}
