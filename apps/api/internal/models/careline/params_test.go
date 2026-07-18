package careline_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/renovisaude/renovi-care/internal/models/careline"
)

func TestParseRuleParams(t *testing.T) {
	tests := []struct {
		name        string
		ruleType    string
		raw         string
		wantErr     bool
		errContains string
		check       func(t *testing.T, v any)
	}{
		// --- QUOTA ---
		{
			name: "quota_month_rolling_valida", ruleType: careline.RuleQuota,
			raw: `{"max":4,"period":"month","window":"rolling"}`,
			check: func(t *testing.T, v any) {
				p, ok := v.(careline.QuotaParams)
				require.True(t, ok, "quero QuotaParams, veio %T", v)
				assert.Equal(t, 4, p.Max)
				assert.Equal(t, "month", p.Period)
				assert.Equal(t, "rolling", p.Window)
			},
		},
		{
			name: "quota_window_vazia_assume_rolling", ruleType: careline.RuleQuota,
			raw: `{"max":4,"period":"week"}`,
			check: func(t *testing.T, v any) {
				p, ok := v.(careline.QuotaParams)
				require.True(t, ok)
				assert.Equal(t, "rolling", p.Window)
			},
		},
		{
			name: "quota_total_aceita_sem_window", ruleType: careline.RuleQuota,
			raw: `{"max":1,"period":"total"}`,
			check: func(t *testing.T, v any) {
				p, ok := v.(careline.QuotaParams)
				require.True(t, ok)
				assert.Equal(t, "total", p.Period)
			},
		},
		{
			name: "quota_json_invalido", ruleType: careline.RuleQuota,
			raw: `{"max":`, wantErr: true,
		},
		{
			name: "quota_campo_desconhecido", ruleType: careline.RuleQuota,
			raw: `{"max":4,"period":"month","foo":1}`, wantErr: true,
		},
		{
			name: "quota_max_zero", ruleType: careline.RuleQuota,
			raw: `{"max":0,"period":"month"}`, wantErr: true,
		},
		{
			name: "quota_period_invalido", ruleType: careline.RuleQuota,
			raw: `{"max":4,"period":"year"}`, wantErr: true,
		},
		{
			name: "quota_window_calendar_rejeitada", ruleType: careline.RuleQuota,
			raw:     `{"max":4,"period":"month","window":"calendar"}`,
			wantErr: true, errContains: "calendar não é suportada",
		},

		// --- MIN_INTERVAL ---
		{
			name: "min_interval_valido", ruleType: careline.RuleMinInterval,
			raw: `{"days":7}`,
			check: func(t *testing.T, v any) {
				p, ok := v.(careline.MinIntervalParams)
				require.True(t, ok)
				assert.Equal(t, 7, p.Days)
			},
		},
		{
			name: "min_interval_days_zero", ruleType: careline.RuleMinInterval,
			raw: `{"days":0}`, wantErr: true,
		},

		// --- MAX_ADVANCE ---
		{
			name: "max_advance_valido", ruleType: careline.RuleMaxAdvance,
			raw: `{"days":14}`,
			check: func(t *testing.T, v any) {
				p, ok := v.(careline.MaxAdvanceParams)
				require.True(t, ok)
				assert.Equal(t, 14, p.Days)
			},
		},
		{
			name: "max_advance_days_zero", ruleType: careline.RuleMaxAdvance,
			raw: `{"days":0}`, wantErr: true,
		},

		// --- PREREQUISITE ---
		{
			name: "prerequisite_valido", ruleType: careline.RulePrerequisite,
			raw: `{"item_ref":"psiquiatria","status":"realizada","within_days":90}`,
			check: func(t *testing.T, v any) {
				p, ok := v.(careline.PrerequisiteParams)
				require.True(t, ok)
				assert.Equal(t, "psiquiatria", p.ItemRef)
				assert.Equal(t, careline.StatusRealizada, p.Status)
				assert.Equal(t, 90, p.WithinDays)
			},
		},
		{
			name: "prerequisite_item_ref_vazio", ruleType: careline.RulePrerequisite,
			raw: `{"item_ref":"","status":"realizada","within_days":90}`, wantErr: true,
		},
		{
			name: "prerequisite_status_invalido", ruleType: careline.RulePrerequisite,
			raw: `{"item_ref":"x","status":"feita","within_days":90}`, wantErr: true,
		},
		{
			name: "prerequisite_within_days_zero", ruleType: careline.RulePrerequisite,
			raw: `{"item_ref":"x","status":"realizada","within_days":0}`, wantErr: true,
		},

		// --- tipo desconhecido: falha (fail-closed no Evaluate) ---
		{
			name: "tipo_de_regra_desconhecido", ruleType: "FOO",
			raw: `{}`, wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := careline.ParseRuleParams(tt.ruleType, json.RawMessage(tt.raw))

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, v)
			if tt.check != nil {
				tt.check(t, v)
			}
		})
	}
}
