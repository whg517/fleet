package audit

import (
	"testing"
)

func TestSanitize_TopLevel(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"username": "alice",
		"password": "supersecret",
	}
	got := Sanitize(in)
	if got["username"] != "alice" {
		t.Errorf("username should be unchanged, got %v", got["username"])
	}
	if got["password"] != "***" {
		t.Errorf("password should be masked, got %v", got["password"])
	}
}

func TestSanitize_CaseInsensitive(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"APIKey":    "abc123",
		"Authorization": "Bearer xyz",
	}
	got := Sanitize(in)
	if got["APIKey"] != "***" {
		t.Errorf("APIKey should be masked, got %v", got["APIKey"])
	}
	if got["Authorization"] != "Bearer xyz" {
		t.Errorf("Authorization should be unchanged (not in sensitive list), got %v", got["Authorization"])
	}
}

func TestSanitize_NestedMap(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"user": map[string]any{
			"name":     "bob",
			"password": "hunter2",
			"token":    "tok-abc",
		},
	}
	got := Sanitize(in)
	user, ok := got["user"].(map[string]any)
	if !ok {
		t.Fatal("user should be a map")
	}
	if user["name"] != "bob" {
		t.Errorf("name should be unchanged, got %v", user["name"])
	}
	if user["password"] != "***" {
		t.Errorf("password should be masked, got %v", user["password"])
	}
	if user["token"] != "***" {
		t.Errorf("token should be masked, got %v", user["token"])
	}
}

func TestSanitize_NestedSlice(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"items": []any{
			map[string]any{"name": "item1", "secret": "s"},
			map[string]any{"name": "item2", "secret": "s2"},
		},
	}
	got := Sanitize(in)
	items, ok := got["items"].([]any)
	if !ok {
		t.Fatal("items should be a slice")
	}
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("item %d should be a map", i)
		}
		if m["secret"] != "***" {
			t.Errorf("item %d secret should be masked, got %v", i, m["secret"])
		}
	}
}

func TestSanitize_SliceOfScalars(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"keys":  []any{"a", "b", "c"},
		"tags":  []any{"prod", "staging"},
	}
	got := Sanitize(in)
	// "keys" contains "key" → sensitive, entire value masked
	if got["keys"] != "***" {
		t.Errorf("keys should be masked entirely, got %v", got["keys"])
	}
	// "tags" is not sensitive → unchanged
	tags, ok := got["tags"].([]any)
	if !ok {
		t.Fatal("tags should be a slice")
	}
	if tags[0] != "prod" {
		t.Errorf("tags[0] should be unchanged, got %v", tags[0])
	}
}

func TestSanitize_Nil(t *testing.T) {
	t.Parallel()
	got := Sanitize(nil)
	if got != nil {
		t.Errorf("nil input should return nil, got %v", got)
	}
}

func TestSanitize_Empty(t *testing.T) {
	t.Parallel()
	got := Sanitize(map[string]any{})
	if len(got) != 0 {
		t.Errorf("empty input should return empty map, got %v", got)
	}
}

func TestSanitize_AllSensitiveKeywords(t *testing.T) {
	t.Parallel()
	keywords := []string{"password", "secret", "token", "key", "kubeconfig", "credentials"}
	for _, kw := range keywords {
		in := map[string]any{kw: "value"}
		got := Sanitize(in)
		if got[kw] != "***" {
			t.Errorf("key %q should be masked, got %v", kw, got[kw])
		}
	}
}

func TestSanitize_PartialMatch(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"api_token":    "tok",
		"access_key":    "key-val",
		"refresh_token": "rt",
		"description":   "normal field",
	}
	got := Sanitize(in)
	if got["api_token"] != "***" {
		t.Errorf("api_token should be masked")
	}
	if got["access_key"] != "***" {
		t.Errorf("access_key should be masked")
	}
	if got["refresh_token"] != "***" {
		t.Errorf("refresh_token should be masked")
	}
	if got["description"] != "normal field" {
		t.Errorf("description should be unchanged")
	}
}

func TestSanitize_DeepNesting(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"password": "deep_secret",
					"name":     "deep",
				},
			},
		},
	}
	got := Sanitize(in)
	l1 := got["level1"].(map[string]any)
	l2 := l1["level2"].(map[string]any)
	l3 := l2["level3"].(map[string]any)
	if l3["password"] != "***" {
		t.Errorf("deeply nested password should be masked, got %v", l3["password"])
	}
	if l3["name"] != "deep" {
		t.Errorf("deeply nested name should be unchanged, got %v", l3["name"])
	}
}

func TestSanitize_DoesNotMutateInput(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"password": "original",
	}
	_ = Sanitize(in)
	if in["password"] != "original" {
		t.Errorf("Sanitize should not mutate the input map")
	}
}
