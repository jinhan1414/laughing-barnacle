package routine

import "strings"

const (
	ActionSkillPrefix = "skill:"
)

func IsSupportedAction(action string) bool {
	action = NormalizeAction(action)
	if action == "" {
		return false
	}
	skillID, ok := SkillIDFromAction(action)
	if !ok {
		return false
	}
	return validateActionIdentifier(skillID)
}

func NormalizeAction(action string) string {
	return strings.TrimSpace(action)
}

func SkillIDFromAction(action string) (string, bool) {
	action = NormalizeAction(action)
	if !strings.HasPrefix(action, ActionSkillPrefix) {
		return "", false
	}
	skillID := strings.TrimSpace(strings.TrimPrefix(action, ActionSkillPrefix))
	if !validateActionIdentifier(skillID) {
		return "", false
	}
	return skillID, true
}

func validateActionIdentifier(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' ||
			r == '_' {
			continue
		}
		return false
	}
	return true
}
