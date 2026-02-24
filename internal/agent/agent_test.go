package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"laughing-barnacle/internal/conversation"
	"laughing-barnacle/internal/llm"
)

type mockLLM struct {
	mu        sync.Mutex
	calls     []llm.ChatRequest
	responses map[string][]string
	toolCalls map[string][][]llm.ToolCall
	usages    map[string][]llm.TokenUsage
	errors    map[string][]error
}

func (m *mockLLM) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, req)
	if errQueue := m.errors[req.Purpose]; len(errQueue) > 0 {
		nextErr := errQueue[0]
		m.errors[req.Purpose] = errQueue[1:]
		if nextErr != nil {
			return llm.ChatResponse{}, nextErr
		}
	}

	queue := m.responses[req.Purpose]
	if len(queue) == 0 {
		return llm.ChatResponse{Content: "fallback"}, nil
	}
	out := queue[0]
	m.responses[req.Purpose] = queue[1:]
	var toolCalls []llm.ToolCall
	if tcQueue := m.toolCalls[req.Purpose]; len(tcQueue) > 0 {
		toolCalls = tcQueue[0]
		m.toolCalls[req.Purpose] = tcQueue[1:]
	}
	var usage llm.TokenUsage
	if usageQueue := m.usages[req.Purpose]; len(usageQueue) > 0 {
		usage = usageQueue[0]
		m.usages[req.Purpose] = usageQueue[1:]
	}

	return llm.ChatResponse{Content: out, ToolCalls: toolCalls, Usage: usage}, nil
}

type mockTools struct {
	mu       sync.Mutex
	listed   []llm.ToolDefinition
	calls    []llm.ToolCall
	response map[string]string
}

type mockSkills struct {
	prompts    []string
	indexLines []string
	promptByID map[string]string
	upserts    []evolvedSkill
}

type mockMemory struct {
	indexLines []string
	appends    int
	lastUser   string
	lastReply  string
}

func (m *mockSkills) ListEnabledSkillIndex() []string {
	if len(m.indexLines) > 0 {
		return m.indexLines
	}
	out := make([]string, 0, len(m.prompts))
	for i, prompt := range m.prompts {
		id := fmt.Sprintf("skill-%d", i+1)
		out = append(out, fmt.Sprintf("skill_id=%s | name=%s | description=%s", id, id, prompt))
	}
	return out
}

func (m *mockSkills) ReadEnabledSkillPrompt(skillID string) (string, bool) {
	if m.promptByID == nil {
		return "", false
	}
	prompt, ok := m.promptByID[strings.TrimSpace(skillID)]
	return prompt, ok && strings.TrimSpace(prompt) != ""
}

func (m *mockSkills) UpsertAutoSkill(name, prompt string) error {
	m.upserts = append(m.upserts, evolvedSkill{
		Name:   strings.TrimSpace(name),
		Prompt: strings.TrimSpace(prompt),
	})
	return nil
}

func (m *mockMemory) ListIndexLines(_ int) []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.indexLines...)
}

func (m *mockMemory) AppendTurn(user, assistant string, _ []conversation.ToolCall, _ time.Time) error {
	m.appends++
	m.lastUser = strings.TrimSpace(user)
	m.lastReply = strings.TrimSpace(assistant)
	return nil
}

type mockPromptProvider struct {
	systemPrompt            string
	compressionSystemPrompt string
}

func (m *mockPromptProvider) GetSystemPrompt() string {
	return m.systemPrompt
}

func (m *mockPromptProvider) GetCompressionSystemPrompt() string {
	return m.compressionSystemPrompt
}

type mockPromptUpdater struct {
	systemPrompt            string
	compressionSystemPrompt string
	calls                   int
}

func (m *mockPromptUpdater) UpdateAgentPrompts(systemPrompt, compressionSystemPrompt string) error {
	m.systemPrompt = systemPrompt
	m.compressionSystemPrompt = compressionSystemPrompt
	m.calls++
	return nil
}

type mockHabits struct {
	lastSleepReviewDate     string
	lastWakePlanDate        string
	lastPromptEvolutionDate string
}

func (m *mockHabits) GetLastSleepReviewDate() string {
	return m.lastSleepReviewDate
}

func (m *mockHabits) GetLastWakePlanDate() string {
	return m.lastWakePlanDate
}

func (m *mockHabits) GetLastPromptEvolutionDate() string {
	return m.lastPromptEvolutionDate
}

func (m *mockHabits) SetLastSleepReviewDate(date string) error {
	m.lastSleepReviewDate = date
	return nil
}

func (m *mockHabits) SetLastWakePlanDate(date string) error {
	m.lastWakePlanDate = date
	return nil
}

func (m *mockHabits) SetLastPromptEvolutionDate(date string) error {
	m.lastPromptEvolutionDate = date
	return nil
}

func (m *mockTools) ListTools(_ context.Context) ([]llm.ToolDefinition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listed, nil
}

func (m *mockTools) CallTool(_ context.Context, call llm.ToolCall) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, call)
	key := call.Function.Name + ":" + call.Function.Arguments
	if out, ok := m.response[key]; ok {
		return out, nil
	}
	return "{}", nil
}
