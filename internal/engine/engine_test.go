package engine

import (
	"testing"
	"time"
)

func compileRules(t *testing.T, cfg *RulesConfig) *Engine {
	t.Helper()
	eng, err := Compile(cfg)
	if err != nil {
		t.Fatalf("Compile() error: %v", err)
	}
	return eng
}

func TestTopicMatcherWildcard(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Rules: []RuleDef{
			{For: "*", When: []WhenDef{{Match: "*", Do: []ActionDef{{"slack": {"text": "hit"}}}}}},
		},
	})
	results := eng.Evaluate("anything", "broken", "msg", "", false, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Args["text"] != "hit" {
		t.Errorf("expected text=hit, got %q", results[0].Args["text"])
	}
}

func TestTopicMatcherLiteral(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Rules: []RuleDef{
			{For: "my-pipeline", When: []WhenDef{{Match: "*", Do: []ActionDef{{"slack": {"text": "hit"}}}}}},
		},
	})

	results := eng.Evaluate("my-pipeline", "broken", "msg", "", false, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for matching topic, got %d", len(results))
	}

	results = eng.Evaluate("other-pipeline", "broken", "msg", "", false, nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-matching topic, got %d", len(results))
	}
}

func TestTopicMatcherGlob(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Rules: []RuleDef{
			{For: "shield-*", When: []WhenDef{{Match: "*", Do: []ActionDef{{"slack": {"text": "hit"}}}}}},
		},
	})

	if r := eng.Evaluate("shield-prod", "broken", "", "", false, nil); len(r) != 1 {
		t.Errorf("expected glob match for shield-prod, got %d results", len(r))
	}
	if r := eng.Evaluate("bosh-prod", "broken", "", "", false, nil); len(r) != 0 {
		t.Errorf("expected no match for bosh-prod, got %d results", len(r))
	}
}

func TestTopicMatcherRegex(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Rules: []RuleDef{
			{For: "re:^prod-.*-deploy$", When: []WhenDef{{Match: "*", Do: []ActionDef{{"slack": {"text": "hit"}}}}}},
		},
	})

	if r := eng.Evaluate("prod-api-deploy", "broken", "", "", false, nil); len(r) != 1 {
		t.Errorf("expected regex match, got %d results", len(r))
	}
	if r := eng.Evaluate("prod-api-test", "broken", "", "", false, nil); len(r) != 0 {
		t.Errorf("expected no regex match, got %d results", len(r))
	}
}

func TestTopicMatcherExpr(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Rules: []RuleDef{
			{For: `topic == "special"`, When: []WhenDef{{Match: "*", Do: []ActionDef{{"slack": {"text": "hit"}}}}}},
		},
	})

	if r := eng.Evaluate("special", "broken", "", "", false, nil); len(r) != 1 {
		t.Errorf("expected expr match, got %d results", len(r))
	}
	if r := eng.Evaluate("normal", "broken", "", "", false, nil); len(r) != 0 {
		t.Errorf("expected no expr match, got %d results", len(r))
	}
}

func TestAllFireSemantics(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Rules: []RuleDef{
			{For: "*", When: []WhenDef{{Match: "*", Do: []ActionDef{{"slack": {"text": "first"}}}}}},
			{For: "*", When: []WhenDef{{Match: "*", Do: []ActionDef{{"webhook": {"url": "second"}}}}}},
			{For: "no-match", When: []WhenDef{{Match: "*", Do: []ActionDef{{"slack": {"text": "skip"}}}}}},
		},
	})

	results := eng.Evaluate("test", "broken", "msg", "", false, nil)
	if len(results) != 2 {
		t.Fatalf("expected 2 results (all-fire), got %d", len(results))
	}
	if results[0].Handler != "slack" || results[0].Args["text"] != "first" {
		t.Errorf("first result: handler=%q text=%q", results[0].Handler, results[0].Args["text"])
	}
	if results[1].Handler != "webhook" || results[1].Args["url"] != "second" {
		t.Errorf("second result: handler=%q url=%q", results[1].Handler, results[1].Args["url"])
	}
}

func TestFirstMatchWhenWithinFOR(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Rules: []RuleDef{
			{For: "*", When: []WhenDef{
				{Match: `status == "now broken"`, Do: []ActionDef{{"slack": {"text": "broken-handler"}}}},
				{Match: "*", Do: []ActionDef{{"slack": {"text": "catch-all"}}}},
			}},
		},
	})

	// Should match first WHEN
	results := eng.Evaluate("test", "now broken", "msg", "", false, nil)
	if len(results) != 1 || results[0].Args["text"] != "broken-handler" {
		t.Errorf("expected broken-handler, got %v", results)
	}

	// Should fall through to second WHEN
	results = eng.Evaluate("test", "now fixed", "msg", "", true, nil)
	if len(results) != 1 || results[0].Args["text"] != "catch-all" {
		t.Errorf("expected catch-all, got %v", results)
	}
}

func TestTemplateInterpolation(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Vars: map[string]any{"channel": "#ops"},
		Rules: []RuleDef{
			{For: "*", When: []WhenDef{{Match: "*", Do: []ActionDef{
				{"slack": {
					"text": "{{ .Topic }} is {{ .Status }}: {{ .Message }}",
					"link": "{{ .Link }}",
					"chan": "{{ .Vars.channel }}",
				}},
			}}}},
		},
	})

	results := eng.Evaluate("prod/deploy", "now broken", "build failed", "http://ci/42", false, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Args["text"] != "prod/deploy is now broken: build failed" {
		t.Errorf("text = %q", r.Args["text"])
	}
	if r.Args["link"] != "http://ci/42" {
		t.Errorf("link = %q", r.Args["link"])
	}
	if r.Args["chan"] != "#ops" {
		t.Errorf("chan = %q", r.Args["chan"])
	}
}

func TestMissingTemplateVars(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Rules: []RuleDef{
			{For: "*", When: []WhenDef{{Match: "*", Do: []ActionDef{
				{"slack": {"text": "{{ .Meta.nonexistent }} end"}},
			}}}},
		},
	})

	results := eng.Evaluate("test", "broken", "msg", "", false, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// With missingkey=zero, missing keys produce zero value (empty string)
	if results[0].Args["text"] != " end" {
		t.Errorf("expected ' end' for missing key, got %q", results[0].Args["text"])
	}
}

func TestBetweenSameDay(t *testing.T) {
	// Same-day window
	if !timeBetween("12:00", "08:00", "17:00") {
		t.Error("12:00 should be between 08:00 and 17:00")
	}
	if timeBetween("07:00", "08:00", "17:00") {
		t.Error("07:00 should not be between 08:00 and 17:00")
	}
	if timeBetween("18:00", "08:00", "17:00") {
		t.Error("18:00 should not be between 08:00 and 17:00")
	}
}

func TestBetweenOvernight(t *testing.T) {
	// Overnight window (start > end)
	if !timeBetween("23:30", "23:00", "02:00") {
		t.Error("23:30 should be between 23:00 and 02:00 (overnight)")
	}
	if !timeBetween("01:00", "23:00", "02:00") {
		t.Error("01:00 should be between 23:00 and 02:00 (overnight)")
	}
	if timeBetween("12:00", "23:00", "02:00") {
		t.Error("12:00 should not be between 23:00 and 02:00 (overnight)")
	}
}

func TestBetweenBoundary(t *testing.T) {
	if !timeBetween("08:00", "08:00", "17:00") {
		t.Error("start boundary should be inclusive")
	}
	if !timeBetween("17:00", "08:00", "17:00") {
		t.Error("end boundary should be inclusive")
	}
}

func TestRemindParsing(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Rules: []RuleDef{
			{For: "*", When: []WhenDef{{Match: "*", Remind: "30m", Do: []ActionDef{{"slack": {"text": "x"}}}}}},
		},
	})

	results := eng.Evaluate("test", "broken", "msg", "", false, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Reminder != 30*time.Minute {
		t.Errorf("expected 30m reminder, got %v", results[0].Reminder)
	}
}

func TestCompileErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  RulesConfig
	}{
		{
			name: "invalid regex",
			cfg:  RulesConfig{Rules: []RuleDef{{For: "re:[invalid"}}},
		},
		{
			name: "invalid when expr",
			cfg: RulesConfig{Rules: []RuleDef{
				{For: "*", When: []WhenDef{{Match: "not_a_valid_expression(!!!"}}},
			}},
		},
		{
			name: "invalid remind duration",
			cfg: RulesConfig{Rules: []RuleDef{
				{For: "*", When: []WhenDef{{Match: "*", Remind: "notaduration"}}},
			}},
		},
		{
			name: "invalid template",
			cfg: RulesConfig{Rules: []RuleDef{
				{For: "*", When: []WhenDef{{Match: "*", Do: []ActionDef{{"slack": {"text": "{{ .Broken"}}}}}},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(&tt.cfg)
			if err == nil {
				t.Error("expected compile error, got nil")
			}
		})
	}
}

func TestLookupFunc(t *testing.T) {
	m := map[string]any{
		"shield":  "https://shield.example.com",
		"default": "https://default.example.com",
	}

	if v := tmplLookup(m, "shield"); v != "https://shield.example.com" {
		t.Errorf("expected shield URL, got %q", v)
	}
	if v := tmplLookup(m, "missing", "default"); v != "https://default.example.com" {
		t.Errorf("expected default URL, got %q", v)
	}
	if v := tmplLookup(m, "missing"); v != "" {
		t.Errorf("expected empty string for missing key, got %q", v)
	}
}

func TestNoRulesReturnsNil(t *testing.T) {
	eng := compileRules(t, &RulesConfig{})
	results := eng.Evaluate("test", "broken", "msg", "", false, nil)
	if results != nil {
		t.Errorf("expected nil results for no rules, got %v", results)
	}
}

func TestNoMatchingFOR(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Rules: []RuleDef{
			{For: "specific-topic", When: []WhenDef{{Match: "*", Do: []ActionDef{{"slack": {"text": "hit"}}}}}},
		},
	})

	results := eng.Evaluate("other-topic", "broken", "msg", "", false, nil)
	if results != nil {
		t.Errorf("expected nil results for non-matching topic, got %v", results)
	}
}

func TestFORMatchesNoWHEN(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Rules: []RuleDef{
			{For: "*", When: []WhenDef{
				{Match: `status == "impossible"`, Do: []ActionDef{{"slack": {"text": "never"}}}},
			}},
		},
	})

	results := eng.Evaluate("test", "broken", "msg", "", false, nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results when no WHEN matches, got %d", len(results))
	}
}

func TestMetadataInTemplate(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Rules: []RuleDef{
			{For: "*", When: []WhenDef{{Match: "*", Do: []ActionDef{
				{"slack": {"text": "env={{ .Meta.env }}"}},
			}}}},
		},
	})

	meta := map[string]string{"env": "production"}
	results := eng.Evaluate("test", "broken", "msg", "", false, meta)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Args["text"] != "env=production" {
		t.Errorf("expected env=production, got %q", results[0].Args["text"])
	}
}

func TestMultipleActionsInWhen(t *testing.T) {
	eng := compileRules(t, &RulesConfig{
		Rules: []RuleDef{
			{For: "*", When: []WhenDef{{Match: "*", Do: []ActionDef{
				{"slack": {"text": "slack-msg"}},
				{"webhook": {"url": "http://example.com"}},
			}}}},
		},
	})

	results := eng.Evaluate("test", "broken", "msg", "", false, nil)
	if len(results) != 2 {
		t.Fatalf("expected 2 results for 2 actions, got %d", len(results))
	}
	if results[0].Handler != "slack" {
		t.Errorf("first handler should be slack, got %q", results[0].Handler)
	}
	if results[1].Handler != "webhook" {
		t.Errorf("second handler should be webhook, got %q", results[1].Handler)
	}
}
