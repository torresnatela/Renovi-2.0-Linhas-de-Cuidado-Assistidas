package careline

import "encoding/json"

// QuotaParams limita quantas consultas do item cabem num período.
// period: week | month | total. window: só "rolling" (vazio assume rolling);
// para period == total a window é ignorada.
type QuotaParams struct {
	Max    int    `json:"max"`
	Period string `json:"period"`
	Window string `json:"window"`
}

// MinIntervalParams exige distância mínima (em dias) entre consultas do item.
type MinIntervalParams struct {
	Days int `json:"days"`
}

// MaxAdvanceParams limita a antecedência máxima (em dias) do agendamento.
type MaxAdvanceParams struct {
	Days int `json:"days"`
}

// PrerequisiteParams exige consulta de outro item com certo status dentro de
// uma janela retroativa de within_days.
type PrerequisiteParams struct {
	ItemRef    string `json:"item_ref"`
	Status     string `json:"status"`
	WithinDays int    `json:"within_days"`
}

// ParseRuleParams decodifica e valida os params de uma regra. Erros são
// mensagens em PT-BR — sobem até o 400 do publish.
func ParseRuleParams(ruleType string, raw json.RawMessage) (any, error) {
	return nil, nil
}
