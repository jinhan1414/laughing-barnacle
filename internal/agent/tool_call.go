package agent

import (
	"context"
	"fmt"
	"laughing-barnacle/internal/llm"
	"strings"
)

func (a *Agent) callTool(ctx context.Context, call llm.ToolCall) (string, error) {
	if result, err, handled := a.callBuiltinTool(ctx, call); handled {
		return result, err
	}
	if a.tools == nil {
		return "", fmt.Errorf("unknown tool %q", strings.TrimSpace(call.Function.Name))
	}
	return a.tools.CallTool(ctx, call)
}

func (a *Agent) callBuiltinTool(ctx context.Context, call llm.ToolCall) (result string, err error, handled bool) {
	name := strings.TrimSpace(call.Function.Name)
	switch name {
	case builtinLinuxBashToolName:
		req, err := parseLinuxBashArguments(call.Function.Arguments)
		if err != nil {
			return "", err, true
		}
		out, err := runLinuxBashFn(ctx, req)
		return out, err, true
	case builtinAsyncTaskSubmitToolName:
		out, err := a.callAsyncTaskSubmit(ctx, call.Function.Arguments)
		return out, err, true
	case builtinAsyncTaskGetToolName:
		out, err := a.callAsyncTaskGet(ctx, call.Function.Arguments)
		return out, err, true
	case builtinAsyncTaskCancelToolName:
		out, err := a.callAsyncTaskCancel(ctx, call.Function.Arguments)
		return out, err, true
	default:
		return "", nil, false
	}
}
