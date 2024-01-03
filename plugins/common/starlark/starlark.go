package starlark

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"go.starlark.net/lib/json"
	"go.starlark.net/lib/math"
	startime "go.starlark.net/lib/time"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

type Starlark struct {
	alias string
	Code  string `mapstructure:"code"`
	File  string `mapstructure:"file"`

	thread  *starlark.Thread
	globals starlark.StringDict

	log *slog.Logger
}

func (p *Starlark) Init(alias string, log *slog.Logger) error {
	p.alias = alias
	p.log = log

	if len(p.Code) == 0 && len(p.File) == 0 {
		return errors.New("code or file required")
	}

	if len(p.Code) > 0 {
		p.File = p.alias + ".star"
		goto SCRIPT_LOADED
	}

	if len(p.File) > 0 {
		script, err := os.ReadFile(p.File)
		if err != nil {
			return fmt.Errorf("failed to load %v script: %v", p.File, err)
		}
		p.Code = string(script)
	}
SCRIPT_LOADED:

	builtins := starlark.StringDict{
		"newEvent": starlark.NewBuiltin("newEvent", newEvent),
		"error":    starlark.NewBuiltin("error", newError),
		"struct":   starlark.NewBuiltin("struct", starlarkstruct.Make),
	}

	p.thread = &starlark.Thread{
		Print: func(_ *starlark.Thread, msg string) {
			p.log.Debug(fmt.Sprintf("from starlark: %v", msg))
		},
		Load: func(thread *starlark.Thread, module string) (starlark.StringDict, error) {
			switch module {
			case "math.star":
				return starlark.StringDict{
					"math": math.Module,
				}, nil
			case "time.star":
				return starlark.StringDict{
					"time": startime.Module,
				}, nil
			case "json.star":
				return starlark.StringDict{
					"json": json.Module,
				}, nil
			default:
				script, err := os.ReadFile(module)
				if err != nil {
					return nil, err
				}

				entries, err := starlark.ExecFile(thread, module, script, builtins)
				if err != nil {
					return nil, err
				}

				return entries, nil
			}
		},
	}

	_, program, err := starlark.SourceProgram(p.File, p.Code, builtins.Has)
	if err != nil {
		return fmt.Errorf("compilation failed: %v", err)
	}

	globals, err := program.Init(p.thread, builtins)
	if err != nil {
		return fmt.Errorf("initialization failed: %v", err)
	}
	p.globals = globals

	return nil
}

func (p *Starlark) Thread() *starlark.Thread {
	return p.thread
}

func (p *Starlark) Func(name string) (*starlark.Function, error) {
	stVal, ok := p.globals[name]
	if !ok {
		return nil, fmt.Errorf("%v function not found in starlark script", name)
	}

	stFunc, ok := stVal.(*starlark.Function)
	if !ok {
		return nil, fmt.Errorf("%v is not a function", name)
	}

	return stFunc, nil
}
