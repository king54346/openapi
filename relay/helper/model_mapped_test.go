package helper

import (
	"testing"
)

func TestResolveModelMappingChain(t *testing.T) {
	got, mapped, err := ResolveModelMapping("a", `{"a":"b","b":"c"}`, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "c" || !mapped {
		t.Fatalf("expected c/mapped, got %q/%v", got, mapped)
	}
}

func TestResolveModelMappingNoMapping(t *testing.T) {
	got, mapped, err := ResolveModelMapping("gpt-4o", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "gpt-4o" || mapped {
		t.Fatalf("expected origin/unmapped, got %q/%v", got, mapped)
	}
}

func TestResolveModelMappingCycle(t *testing.T) {
	if _, _, err := ResolveModelMapping("a", `{"a":"b","b":"a"}`, false); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestResolveModelMappingSelfLoop(t *testing.T) {
	got, mapped, err := ResolveModelMapping("a", `{"a":"a"}`, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "a" || mapped {
		t.Fatalf("self loop should be unmapped, got %q/%v", got, mapped)
	}
}

func TestResolveModelMappingTooDeep(t *testing.T) {
	mapping := `{"m0":"m1","m1":"m2","m2":"m3","m3":"m4","m4":"m5","m5":"m6","m6":"m7","m7":"m8","m8":"m9","m9":"m10","m10":"m11","m11":"m12","m12":"m13","m13":"m14","m14":"m15","m15":"m16","m16":"m17","m17":"m18","m18":"m19","m19":"m20","m20":"m21","m21":"m22","m22":"m23","m23":"m24","m24":"m25","m25":"m26","m26":"m27","m27":"m28","m28":"m29","m29":"m30","m30":"m31","m31":"m32","m32":"m33"}`
	if _, _, err := ResolveModelMapping("m0", mapping, false); err == nil {
		t.Fatal("expected too-deep error")
	}
}

func TestResolveModelMappingCompact(t *testing.T) {
	got, _, err := ResolveModelMapping("gpt-4o-openai-compact", "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "gpt-4o" {
		t.Fatalf("expected compact suffix stripped, got %q", got)
	}
}

func TestResolveModelMappingInvalidJSON(t *testing.T) {
	if _, _, err := ResolveModelMapping("a", `{invalid}`, false); err == nil {
		t.Fatal("expected json error")
	}
}
