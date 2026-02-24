package routine

import "testing"

func TestIsSupportedAction(t *testing.T) {
	if !IsSupportedAction(ActionNightReflectionEvolution) {
		t.Fatalf("expected builtin night action supported")
	}
	if !IsSupportedAction(ActionMorningPlanning) {
		t.Fatalf("expected builtin morning action supported")
	}
	if !IsSupportedAction("skill:demo_skill") {
		t.Fatalf("expected custom skill action supported")
	}
	if IsSupportedAction("night_reflection_evolution") {
		t.Fatalf("expected legacy underscore action rejected")
	}
	if IsSupportedAction("skill:bad/id") {
		t.Fatalf("expected invalid skill action rejected")
	}
}

func TestSkillIDFromAction(t *testing.T) {
	skillID, ok := SkillIDFromAction(ActionNightReflectionEvolution)
	if !ok || skillID != ScheduledSkillNightReflectionEvolution {
		t.Fatalf("unexpected parsed night skill: id=%q ok=%v", skillID, ok)
	}
	if _, ok := SkillIDFromAction("not-skill-action"); ok {
		t.Fatalf("expected non-skill action rejected")
	}
}
