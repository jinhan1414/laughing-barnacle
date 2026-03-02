package mcp

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"laughing-barnacle/internal/routine"
	"laughing-barnacle/internal/scheduler"
)

func validateService(service Service) error {
	if service.ID == "" {
		return fmt.Errorf("service id is required")
	}
	if !serviceIDPattern.MatchString(service.ID) {
		return fmt.Errorf("service id must match [a-zA-Z0-9_-]+")
	}
	if err := validateServiceConfigInputs(service.Transport, service.Env, service.Headers); err != nil {
		return err
	}
	switch service.Transport {
	case ServiceTransportStreamableHTTP, ServiceTransportSSE:
		if service.Endpoint == "" {
			return fmt.Errorf("service endpoint is required")
		}
		if !strings.HasPrefix(service.Endpoint, "http://") && !strings.HasPrefix(service.Endpoint, "https://") {
			return fmt.Errorf("service endpoint must start with http:// or https://")
		}
	case ServiceTransportStdio:
		if strings.TrimSpace(service.Command) == "" {
			return fmt.Errorf("service command is required for stdio transport")
		}
	default:
		return fmt.Errorf("service transport must be streamable_http, sse or stdio")
	}
	for _, state := range service.ToolStates {
		if strings.TrimSpace(state.Name) == "" {
			return fmt.Errorf("service tool state name is required")
		}
	}
	return nil
}

func validateServiceConfigInputs(transport string, env map[string]string, headers map[string]string) error {
	switch normalizeServiceTransport(transport) {
	case ServiceTransportStdio:
		if len(headers) > 0 {
			return fmt.Errorf("stdio transport does not support headers")
		}
		return validateServiceEnvInputs(env)
	case ServiceTransportStreamableHTTP, ServiceTransportSSE:
		if len(env) > 0 {
			return fmt.Errorf("%s transport does not support env", normalizeServiceTransport(transport))
		}
		return validateServiceHeaderInputs(headers)
	default:
		return nil
	}
}

func validateServiceEnvInputs(env map[string]string) error {
	for key, value := range env {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("service env key must be non-empty")
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("service env value for %q must be non-empty", strings.TrimSpace(key))
		}
	}
	return nil
}

func validateServiceHeaderInputs(headers map[string]string) error {
	for key, value := range headers {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			return fmt.Errorf("service header key must be non-empty")
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("service header value for %q must be non-empty", http.CanonicalHeaderKey(trimmedKey))
		}
		if isProtectedHTTPHeader(trimmedKey) {
			return fmt.Errorf("service headers must not override %s", http.CanonicalHeaderKey(trimmedKey))
		}
	}
	return nil
}

func validateA2AAgent(agent A2AAgent) error {
	if agent.ID == "" {
		return fmt.Errorf("agent id is required")
	}
	if !serviceIDPattern.MatchString(agent.ID) {
		return fmt.Errorf("agent id must match [a-zA-Z0-9_-]+")
	}
	if strings.TrimSpace(agent.Name) == "" {
		return fmt.Errorf("agent name is required")
	}
	if strings.TrimSpace(agent.Endpoint) == "" {
		return fmt.Errorf("agent endpoint is required")
	}
	if !strings.HasPrefix(agent.Endpoint, "http://") && !strings.HasPrefix(agent.Endpoint, "https://") {
		return fmt.Errorf("agent endpoint must start with http:// or https://")
	}
	if cardURL := strings.TrimSpace(agent.AgentCardURL); cardURL != "" {
		if !strings.HasPrefix(cardURL, "http://") && !strings.HasPrefix(cardURL, "https://") {
			return fmt.Errorf("agent card url must start with http:// or https://")
		}
	}
	return nil
}

func validateSkill(skill Skill) error {
	if skill.ID == "" {
		return fmt.Errorf("skill id is required")
	}
	if !serviceIDPattern.MatchString(skill.ID) {
		return fmt.Errorf("skill id must match [a-zA-Z0-9_-]+")
	}
	if strings.TrimSpace(skill.Name) == "" {
		return fmt.Errorf("skill name is required")
	}
	if strings.TrimSpace(skill.Description) == "" {
		return fmt.Errorf("skill description is required")
	}
	if strings.TrimSpace(skill.Prompt) == "" {
		return fmt.Errorf("skill prompt is required")
	}
	return nil
}

func validateScheduledTask(task scheduler.Task) error {
	task.ID = strings.TrimSpace(task.ID)
	task.Name = strings.TrimSpace(task.Name)
	task.Action = routine.NormalizeAction(strings.TrimSpace(task.Action))
	task.CronExpr = strings.TrimSpace(task.CronExpr)

	if task.ID == "" {
		return fmt.Errorf("task id is required")
	}
	if !serviceIDPattern.MatchString(task.ID) {
		return fmt.Errorf("task id must match [a-zA-Z0-9_-]+")
	}
	if task.Name == "" {
		return fmt.Errorf("task name is required")
	}
	if !routine.IsSupportedAction(task.Action) {
		return fmt.Errorf("unsupported task action: %s", strings.TrimSpace(task.Action))
	}
	if task.CronExpr == "" {
		return fmt.Errorf("task cron expression is required")
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(task.CronExpr); err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}
	return nil
}

func validateAgentPromptConfig(cfg AgentPromptConfig) error {
	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	compressionPrompt := strings.TrimSpace(cfg.CompressionSystemPrompt)
	if systemPrompt == "" && compressionPrompt == "" {
		return nil
	}
	if systemPrompt == "" {
		return fmt.Errorf("agent system prompt is required")
	}
	if compressionPrompt == "" {
		return fmt.Errorf("agent compression system prompt is required")
	}
	return nil
}

func validateAgentHabitState(state AgentHabitState) error {
	if err := validateOptionalDate(strings.TrimSpace(state.LastSleepReviewDate)); err != nil {
		return fmt.Errorf("last_sleep_review_date: %w", err)
	}
	if err := validateOptionalDate(strings.TrimSpace(state.LastWakePlanDate)); err != nil {
		return fmt.Errorf("last_wake_plan_date: %w", err)
	}
	if err := validateOptionalDate(strings.TrimSpace(state.LastPromptEvolutionDate)); err != nil {
		return fmt.Errorf("last_prompt_evolution_date: %w", err)
	}
	if err := validateOptionalDate(strings.TrimSpace(state.LastChatGreetingDate)); err != nil {
		return fmt.Errorf("last_chat_greeting_date: %w", err)
	}
	if err := validateOptionalTimestamp(strings.TrimSpace(state.LastChatGreetingAt)); err != nil {
		return fmt.Errorf("last_chat_greeting_at: %w", err)
	}
	return nil
}

func validateOptionalDate(v string) error {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return fmt.Errorf("must be YYYY-MM-DD")
	}
	return nil
}

func validateOptionalTimestamp(v string) error {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, v); err != nil {
		return fmt.Errorf("must be RFC3339")
	}
	return nil
}
