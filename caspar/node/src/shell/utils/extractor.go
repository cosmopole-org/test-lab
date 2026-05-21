package utils

import (
	"encoding/json"
	"kasper/src/abstract/models/action"
	"kasper/src/abstract/models/core"
	"kasper/src/abstract/models/input"
	"kasper/src/abstract/state"
	mainaction "kasper/src/core/module/actor/model/base"
	"kasper/src/core/module/actor/model/secured"
	"kasper/src/shell/utils/vaidate"
	"strings"
)

func ExtractAction[T input.IInput](app core.ICore, actionFunc func(state.IState, T) (any, error)) action.IAction {
	key, _ := ExtractActionMetadata(actionFunc)
	action := mainaction.NewAction(app.ModifyState, key, func(state state.IState, input input.IInput) (any, error) {
		return actionFunc(state, input.(T))
	})
	return action
}

func ExtractSecureAction[T input.IInput](app core.ICore, actionFunc func(state.IState, T) (any, error)) action.IAction {
	key, guard := ExtractActionMetadata(actionFunc)
	action := mainaction.NewAction(app.ModifyState, key, func(state state.IState, input input.IInput) (any, error) {
		return actionFunc(state, input.(T))
	})
	return secured.NewSecureAction(action, guard, app, map[string]secured.Parse{
		"tcp": func(i interface{}) (input.IInput, error) {
			input := new(T)
			err := json.Unmarshal(i.([]byte), input)
			if err == nil {
				err2 := vaidate.Validate.Struct(input)
				if err2 == nil {
					return *input, nil
				}
				return nil, err2
			}
			return nil, err
		},
		"chain": func(i interface{}) (input.IInput, error) {
			input := new(T)
			err := json.Unmarshal(i.([]byte), input)
			if err == nil {
				err2 := vaidate.Validate.Struct(input)
				if err2 == nil {
					return *input, nil
				}
				return nil, err2
			}
			return nil, err
		},
		"fed": func(i interface{}) (input.IInput, error) {
			input := new(T)
			err := json.Unmarshal(i.([]byte), input)
			if err == nil {
				err2 := vaidate.Validate.Struct(input)
				if err2 == nil {
					return *input, nil
				}
				return nil, err2
			}
			return nil, err
		},
	})
}

func ExtractActionMetadata(function interface{}) (string, *secured.Guard) {
	var ts = strings.Split(FuncDescription(function), " ")
	var tokens []string
	for _, token := range ts {
		if len(strings.Trim(token, " ")) > 0 {
			tokens = append(tokens, token)
		}
	}
	var key = tokens[0]
	var guard *secured.Guard
	if len(tokens) > 5 && tokens[1] == "check" && tokens[2] == "[" {
		if tokens[5] == "]" {
			guard = &secured.Guard{IsUser: tokens[3] == "true", IsInStore: tokens[4] == "true"}
		} else if len(tokens) > 6 && tokens[6] == "]" {
			guard = &secured.Guard{IsUser: tokens[3] == "true", IsInStore: tokens[4] == "true"}
		}
	}
	return key, guard
}
