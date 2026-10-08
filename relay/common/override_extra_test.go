package common

import (
	"testing"
)

func TestApplyParamOverrideExtraEdges(t *testing.T) {
	// Unknown operation should fail fast.
	if _, err := ApplyParamOverride([]byte(`{"a":1}`), map[string]interface{}{
		"operations": []interface{}{map[string]interface{}{"path": "a", "mode": "nope"}},
	}, nil); err == nil {
		t.Fatal("expected unknown operation error")
	}

	// Missing mode should fall back to legacy top-level merge behavior.
	out, err := ApplyParamOverride([]byte(`{"a":1}`), map[string]interface{}{
		"operations": []interface{}{map[string]interface{}{"path": "a"}},
	}, nil)
	if err != nil {
		t.Fatalf("legacy fallback failed: %v", err)
	}
	assertJSONEqual(t, `{"a":1,"operations":[{"path":"a"}]}`, string(out))

	// Invalid negative index must fail clearly rather than corrupting data.
	if _, err := ApplyParamOverride([]byte(`{"arr":[{"model":"a"}]}`), map[string]interface{}{
		"operations": []interface{}{map[string]interface{}{"path": "arr.-9.model", "mode": "set", "value": "c"}},
	}, nil); err == nil {
		t.Fatal("expected error for out-of-range negative index")
	}

	// move with same source/target is a no-op success.
	out, err = ApplyParamOverride([]byte(`{"a":1}`), map[string]interface{}{
		"operations": []interface{}{map[string]interface{}{"mode": "move", "from": "a", "to": "a"}},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJSONEqual(t, `{"a":1}`, string(out))

	// move with empty from/to must fail clearly.
	if _, err := ApplyParamOverride([]byte(`{"a":1}`), map[string]interface{}{
		"operations": []interface{}{map[string]interface{}{"mode": "move"}},
	}, nil); err == nil {
		t.Fatal("expected move validation error")
	}

	// move should preserve raw JSON types (numbers stay numbers).
	out, err = ApplyParamOverride([]byte(`{"a":1,"b":{}}`), map[string]interface{}{
		"operations": []interface{}{map[string]interface{}{"mode": "move", "from": "a", "to": "b.a"}},
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJSONEqual(t, `{"b":{"a":1}}`, string(out))
}
