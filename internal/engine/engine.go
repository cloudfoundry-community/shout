package engine

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// RulesConfig is the top-level YAML rules file structure.
type RulesConfig struct {
	Vars  map[string]any `yaml:"vars"`
	Rules []RuleDef      `yaml:"rules"`
}

// RuleDef is a single FOR clause with its WHEN clauses.
type RuleDef struct {
	For  string     `yaml:"for"`
	When []WhenDef  `yaml:"when"`
}

// WhenDef is a single WHEN clause with its match condition and actions.
type WhenDef struct {
	Match  string      `yaml:"match"`
	Remind string      `yaml:"remind,omitempty"`
	Do     []ActionDef `yaml:"do"`
}

// ActionDef is a handler invocation: the key is the handler name,
// the value is a map of arguments (with template strings).
type ActionDef map[string]map[string]string

// Engine evaluates compiled rules against events.
type Engine struct {
	rules []compiledRule
	vars  map[string]any
	funcs template.FuncMap
}

type compiledRule struct {
	matcher topicMatcher
	whens   []compiledWhen
}

type compiledWhen struct {
	condition *vm.Program // nil means wildcard (always match)
	remind    time.Duration
	actions   []compiledAction
}

type compiledAction struct {
	handler string
	args    map[string]*template.Template
}

type topicMatcher interface {
	match(topic string) bool
}

type wildcardMatcher struct{}
type literalMatcher struct{ value string }
type globMatcher struct{ pattern string }
type regexMatcher struct{ re *regexp.Regexp }
type exprMatcher struct{ program *vm.Program }

func (wildcardMatcher) match(string) bool            { return true }
func (m literalMatcher) match(topic string) bool      { return m.value == topic }
func (m globMatcher) match(topic string) bool {
	ok, _ := filepath.Match(m.pattern, topic)
	return ok
}
func (m regexMatcher) match(topic string) bool        { return m.re.MatchString(topic) }
func (m exprMatcher) match(topic string) bool {
	out, err := expr.Run(m.program, map[string]any{"topic": topic})
	if err != nil {
		return false
	}
	b, ok := out.(bool)
	return ok && b
}

// ExprEnv is the environment passed to expr condition evaluations.
type ExprEnv struct {
	Topic   string `expr:"topic"`
	OK      bool   `expr:"ok"`
	Status  string `expr:"status"`
	Weekday string `expr:"weekday"`
	Time    string `expr:"time"`
	Hour    int    `expr:"hour"`
	Minute  int    `expr:"minute"`
}

// TemplateContext is passed to Go templates for string interpolation.
type TemplateContext struct {
	Topic   string
	OK      bool
	Status  string
	Message string
	Link    string
	Meta    map[string]string
	Vars    map[string]any
}

// Compile parses and compiles a RulesConfig into a ready-to-evaluate Engine.
func Compile(cfg *RulesConfig) (*Engine, error) {
	funcs := template.FuncMap{
		"lookup": tmplLookup,
	}

	var rules []compiledRule
	for i, rd := range cfg.Rules {
		matcher, err := compileMatcher(rd.For)
		if err != nil {
			return nil, fmt.Errorf("rule %d: %w", i, err)
		}

		var whens []compiledWhen
		for j, wd := range rd.When {
			cw, err := compileWhen(wd, funcs)
			if err != nil {
				return nil, fmt.Errorf("rule %d when %d: %w", i, j, err)
			}
			whens = append(whens, cw)
		}

		rules = append(rules, compiledRule{matcher: matcher, whens: whens})
	}

	return &Engine{rules: rules, vars: cfg.Vars, funcs: funcs}, nil
}

func compileMatcher(forStr string) (topicMatcher, error) {
	if forStr == "*" {
		return wildcardMatcher{}, nil
	}
	if strings.HasPrefix(forStr, "re:") {
		pattern := strings.TrimPrefix(forStr, "re:")
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex %q: %w", pattern, err)
		}
		return regexMatcher{re: re}, nil
	}
	if strings.ContainsAny(forStr, "*?[") {
		return globMatcher{pattern: forStr}, nil
	}
	// Check if it looks like an expr expression (contains operators).
	if strings.ContainsAny(forStr, "()&|!=<>") {
		program, err := expr.Compile(forStr, expr.Env(ExprEnv{}), expr.AsBool())
		if err != nil {
			return nil, fmt.Errorf("compiling topic expression: %w", err)
		}
		return exprMatcher{program: program}, nil
	}
	return literalMatcher{value: forStr}, nil
}

func compileWhen(wd WhenDef, funcs template.FuncMap) (compiledWhen, error) {
	var cw compiledWhen

	if wd.Match != "" && wd.Match != "*" {
		program, err := expr.Compile(wd.Match, expr.Env(ExprEnv{}), expr.AsBool())
		if err != nil {
			return cw, fmt.Errorf("compiling match expression: %w", err)
		}
		cw.condition = program
	}

	if wd.Remind != "" {
		d, err := time.ParseDuration(wd.Remind)
		if err != nil {
			return cw, fmt.Errorf("parsing remind duration: %w", err)
		}
		cw.remind = d
	}

	for k, ad := range wd.Do {
		for handler, args := range ad {
			ca := compiledAction{handler: handler, args: make(map[string]*template.Template)}
			for argName, argVal := range args {
				tmpl, err := template.New(fmt.Sprintf("r%d-a%d-%s", k, k, argName)).
					Funcs(funcs).
					Parse(argVal)
				if err != nil {
					return cw, fmt.Errorf("parsing template for %s.%s: %w", handler, argName, err)
				}
				ca.args[argName] = tmpl
			}
			cw.actions = append(cw.actions, ca)
		}
	}

	return cw, nil
}

// EvalResult is returned by Evaluate with the resolved handler actions.
type EvalResult struct {
	Handler  string
	Args     map[string]string
	Reminder time.Duration
}

// Evaluate runs the rules engine against a topic event, returning actions
// for the first matching FOR/WHEN combination (first-match-wins).
func (e *Engine) Evaluate(topic, status, message, link string, ok bool, meta map[string]string) []EvalResult {
	now := time.Now()
	weekday := strings.ToLower(now.Weekday().String())[:3]

	env := ExprEnv{
		Topic:   topic,
		OK:      ok,
		Status:  status,
		Weekday: weekday,
		Time:    now.Format("15:04"),
		Hour:    now.Hour(),
		Minute:  now.Minute(),
	}

	ctx := TemplateContext{
		Topic:   topic,
		OK:      ok,
		Status:  status,
		Message: message,
		Link:    link,
		Meta:    meta,
		Vars:    e.vars,
	}

	for _, rule := range e.rules {
		if !rule.matcher.match(topic) {
			continue
		}

		// First matching FOR — now find first matching WHEN.
		for _, when := range rule.whens {
			if when.condition != nil {
				out, err := expr.Run(when.condition, env)
				if err != nil || out != true {
					continue
				}
			}

			// Matched — evaluate actions.
			var results []EvalResult
			for _, action := range when.actions {
				args := make(map[string]string)
				for name, tmpl := range action.args {
					var buf strings.Builder
					if err := tmpl.Execute(&buf, ctx); err != nil {
						args[name] = fmt.Sprintf("<error: %v>", err)
					} else {
						args[name] = buf.String()
					}
				}
				results = append(results, EvalResult{
					Handler:  action.handler,
					Args:     args,
					Reminder: when.remind,
				})
			}
			return results
		}

		// FOR matched but no WHEN matched — stop (first-match-wins for FOR).
		return nil
	}

	return nil
}

func tmplLookup(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}
