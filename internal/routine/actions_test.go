package routine

import "testing"

func TestNormalizeAction_LegacyToSkill(t *testing.T) {
	if got := NormalizeAction(LegacyActionNightReflectionEvolution); got != ActionNightReflectionEvolution {
		t.Fatalf("unexpected night normalized action: %q", got)
	}
	if got := NormalizeAction(LegacyActionMorningPlanning); got != ActionMorningPlanning {
		t.Fatalf("unexpected morning normalized action: %q", got)
	}
}

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
	if IsSupportedAction("night_reflection_evolution") == false {
		t.Fatalf("expected legacy action supported via normalization")
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
	skillID, ok = SkillIDFromAction(LegacyActionMorningPlanning)
	if !ok || skillID != ScheduledSkillMorningPlanning {
		t.Fatalf("unexpected parsed morning skill from legacy: id=%q ok=%v", skillID, ok)
	}
	if _, ok := SkillIDFromAction("not-skill-action"); ok {
		t.Fatalf("expected non-skill action rejected")
	}
}
