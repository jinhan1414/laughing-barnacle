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
}

type mockMemory struct {
	indexLines []string
	appends    int
	lastUser   string
	lastReply  string
}

type mockA2A struct {
	indexLines []string
	details    map[string]A2AAgentDetail
	register   A2AAgentDetail
	send       A2ATaskResult
	get        A2ATaskResult
	cancel     A2ATaskResult
	err        error
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

func (m *mockA2A) ListIndexLines(_ int) []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.indexLines...)
}

func (m *mockA2A) ReadAgentDetail(agentID string) (A2AAgentDetail, bool) {
	if m == nil || m.details == nil {
		return A2AAgentDetail{}, false
	}
	item, ok := m.details[strings.TrimSpace(agentID)]
	return item, ok
}

func (m *mockA2A) Register(_ context.Context, _ A2ARegisterRequest) (A2AAgentDetail, error) {
	if m != nil && m.err != nil {
		return A2AAgentDetail{}, m.err
	}
	if m == nil {
		return A2AAgentDetail{}, nil
	}
	return m.register, nil
}

func (m *mockA2A) Send(_ context.Context, _ A2ASendRequest) (A2ATaskResult, error) {
	if m != nil && m.err != nil {
		return A2ATaskResult{}, m.err
	}
	if m == nil {
		return A2ATaskResult{}, nil
	}
	return m.send, nil
}

func (m *mockA2A) GetTask(_ context.Context, _ A2ATaskQuery) (A2ATaskResult, error) {
	if m != nil && m.err != nil {
		return A2ATaskResult{}, m.err
	}
	if m == nil {
		return A2ATaskResult{}, nil
	}
	return m.get, nil
}

func (m *mockA2A) CancelTask(_ context.Context, _ A2ATaskQuery) (A2ATaskResult, error) {
	if m != nil && m.err != nil {
		return A2ATaskResult{}, m.err
	}
	if m == nil {
		return A2ATaskResult{}, nil
	}
	return m.cancel, nil
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
