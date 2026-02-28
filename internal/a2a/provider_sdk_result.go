package a2a

import (
	"encoding/json"
	"laughing-barnacle/internal/agent"
	"strings"

	a2asdk "github.com/a2aproject/a2a-go/a2a"
)

const sdkUnknownStatus = "unknown"

func parseTaskResultFromSDK(agentID, fallbackTaskID, sdkMethod string, task *a2asdk.Task) agent.A2ATaskResult {
	status, rawStatus := parseSDKTaskStatus(task)
	taskID := strings.TrimSpace(fallbackTaskID)
	message := ""
	artifacts := readSDKArtifactTexts(task)
	if task != nil {
		if id := strings.TrimSpace(string(task.ID)); id != "" {
			taskID = id
		}
		if task.Status.Message != nil {
			message = readSDKMessageText(task.Status.Message)
		}
		if message == "" {
			message = readSDKLastHistoryText(task.History)
		}
	}

	return agent.A2ATaskResult{
		AgentID:     strings.TrimSpace(agentID),
		TaskID:      taskID,
		Status:      status,
		RawStatus:   rawStatus,
		SDKProvider: sdkProviderName,
		SDKMethod:   strings.TrimSpace(sdkMethod),
		Message:     message,
		Artifacts:   artifacts,
	}
}

func parseMessageResultFromSDK(agentID, sdkMethod string, message *a2asdk.Message) agent.A2ATaskResult {
	taskID := ""
	content := ""
	if message != nil {
		taskID = strings.TrimSpace(string(message.TaskID))
		content = readSDKMessageText(message)
	}

	return agent.A2ATaskResult{
		AgentID:     strings.TrimSpace(agentID),
		TaskID:      taskID,
		Status:      string(a2asdk.TaskStateCompleted),
		RawStatus:   "message",
		SDKProvider: sdkProviderName,
		SDKMethod:   strings.TrimSpace(sdkMethod),
		Message:     content,
	}
}

func parseSDKTaskStatus(task *a2asdk.Task) (string, string) {
	if task == nil {
		return sdkUnknownStatus, sdkUnknownStatus
	}
	raw := strings.TrimSpace(string(task.Status.State))
	if raw == "" {
		return sdkUnknownStatus, sdkUnknownStatus
	}
	return raw, raw
}

func readSDKLastHistoryText(history []*a2asdk.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		text := readSDKMessageText(history[i])
		if text != "" {
			return text
		}
	}
	return ""
}

func readSDKMessageText(message *a2asdk.Message) string {
	if message == nil {
		return ""
	}
	parts := readSDKPartsText(message.Parts)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

func readSDKArtifactTexts(task *a2asdk.Task) []string {
	if task == nil || len(task.Artifacts) == 0 {
		return nil
	}
	out := make([]string, 0, len(task.Artifacts))
	for _, item := range task.Artifacts {
		if item == nil {
			continue
		}
		partTexts := readSDKPartsText(item.Parts)
		out = append(out, partTexts...)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func readSDKPartsText(parts a2asdk.ContentParts) []string {
	if len(parts) == 0 {
		return nil
	}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		text := strings.TrimSpace(readSDKPartText(part))
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func readSDKPartText(part a2asdk.Part) string {
	switch v := part.(type) {
	case a2asdk.TextPart:
		return strings.TrimSpace(v.Text)
	case *a2asdk.TextPart:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(v.Text)
	case a2asdk.DataPart:
		return marshalSDKDataPart(v.Data)
	case *a2asdk.DataPart:
		if v == nil {
			return ""
		}
		return marshalSDKDataPart(v.Data)
	case a2asdk.FilePart:
		return describeSDKFilePart(v.File)
	case *a2asdk.FilePart:
		if v == nil {
			return ""
		}
		return describeSDKFilePart(v.File)
	default:
		return ""
	}
}

func marshalSDKDataPart(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func describeSDKFilePart(content a2asdk.FilePartContent) string {
	switch v := content.(type) {
	case a2asdk.FileURI:
		return strings.TrimSpace(v.URI)
	case *a2asdk.FileURI:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(v.URI)
	case a2asdk.FileBytes:
		return strings.TrimSpace(v.Name)
	case *a2asdk.FileBytes:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(v.Name)
	default:
		return ""
	}
}
