package careline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

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
	switch ruleType {
	case RuleQuota:
		var p QuotaParams
		if err := decodeStrict(raw, &p); err != nil {
			return nil, err
		}
		if p.Max < 1 {
			return nil, errors.New("quota: max deve ser pelo menos 1")
		}
		switch p.Period {
		case "week", "month":
			switch p.Window {
			case "", "rolling":
				p.Window = "rolling" // vazio assume janela móvel
			case "calendar":
				return nil, errors.New("quota: janela calendar não é suportada")
			default:
				return nil, fmt.Errorf("quota: janela inválida: %q", p.Window)
			}
		case "total":
			p.Window = "" // window é ignorada para period=total
		default:
			return nil, fmt.Errorf("quota: período inválido: %q (use week, month ou total)", p.Period)
		}
		return p, nil

	case RuleMinInterval:
		var p MinIntervalParams
		if err := decodeStrict(raw, &p); err != nil {
			return nil, err
		}
		if p.Days < 1 {
			return nil, errors.New("intervalo mínimo: days deve ser pelo menos 1")
		}
		return p, nil

	case RuleMaxAdvance:
		var p MaxAdvanceParams
		if err := decodeStrict(raw, &p); err != nil {
			return nil, err
		}
		if p.Days < 1 {
			return nil, errors.New("antecedência máxima: days deve ser pelo menos 1")
		}
		return p, nil

	case RulePrerequisite:
		var p PrerequisiteParams
		if err := decodeStrict(raw, &p); err != nil {
			return nil, err
		}
		if p.ItemRef == "" {
			return nil, errors.New("pré-requisito: item_ref é obrigatório")
		}
		if !validAppointmentStatus(p.Status) {
			return nil, fmt.Errorf("pré-requisito: status inválido: %q", p.Status)
		}
		if p.WithinDays < 1 {
			return nil, errors.New("pré-requisito: within_days deve ser pelo menos 1")
		}
		return p, nil

	case RuleVigencia:
		// Vigência é pré-condição intrínseca da matrícula — não tem params.
		if len(bytes.TrimSpace(raw)) == 0 {
			return nil, nil
		}
		var empty struct{}
		if err := decodeStrict(raw, &empty); err != nil {
			return nil, err
		}
		return nil, nil

	default:
		return nil, fmt.Errorf("tipo de regra desconhecido: %q", ruleType)
	}
}

// decodeStrict decodifica rejeitando campos desconhecidos (config de regra
// com typo não pode passar silenciosamente).
func decodeStrict(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("parâmetros inválidos: %v", err)
	}
	return nil
}

func validAppointmentStatus(s string) bool {
	switch s {
	case StatusAgendada, StatusConfirmada, StatusEmAndamento, StatusRealizada, StatusFalta, StatusCancelada:
		return true
	}
	return false
}
