package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
)

func decodeRPCResponse(respBytes []byte, contentType string) (rpcResponse, error) {
	trimmed := bytes.TrimSpace(respBytes)
	if len(trimmed) == 0 {
		return rpcResponse{}, fmt.Errorf("decode rpc response: empty response")
	}
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") ||
		bytes.HasPrefix(trimmed, []byte("event:")) ||
		bytes.HasPrefix(trimmed, []byte("data:")) {
		return decodeRPCResponseFromSSE(trimmed, nil)
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(trimmed, &rpcResp); err != nil {
		return rpcResponse{}, fmt.Errorf("decode rpc response: %w", err)
	}
	return rpcResp, nil
}

func decodeRPCResponseFromSSE(payload []byte, expectID any) (rpcResponse, error) {
	reader := bufio.NewReader(bytes.NewReader(payload))
	return waitRPCResponseFromSSE(reader, expectID)
}

func waitRPCResponseFromSSE(reader *bufio.Reader, expectID any) (rpcResponse, error) {
	for {
		event, err := readSSEEvent(reader)
		if err != nil {
			if err == io.EOF {
				return rpcResponse{}, fmt.Errorf("decode rpc response: no rpc message in sse stream")
			}
			return rpcResponse{}, fmt.Errorf("decode rpc response: %w", err)
		}

		data := strings.TrimSpace(event.Data)
		if data == "" {
			continue
		}

		var rpcResp rpcResponse
		if unmarshalErr := json.Unmarshal([]byte(data), &rpcResp); unmarshalErr != nil {
			continue
		}
		if expectID != nil && !sameRPCID(expectID, rpcResp.ID) {
			continue
		}
		return rpcResp, nil
	}
}

func waitRPCResponseFromSTDIO(decoder *json.Decoder, expectID any) (rpcResponse, error) {
	for {
		var envelope map[string]json.RawMessage
		if err := decoder.Decode(&envelope); err != nil {
			if err == io.EOF {
				return rpcResponse{}, fmt.Errorf("decode rpc response: eof")
			}
			return rpcResponse{}, fmt.Errorf("decode rpc response: %w", err)
		}

		methodField, hasMethod := envelope["method"]
		if hasMethod {
			var method string
			if err := json.Unmarshal(methodField, &method); err == nil && strings.TrimSpace(method) != "" {
				// Server initiated request/notification; ignore for this lightweight client.
				continue
			}
		}

		idField, hasID := envelope["id"]
		if !hasID {
			continue
		}
		var id any
		_ = json.Unmarshal(idField, &id)
		if expectID != nil && !sameRPCID(expectID, id) {
			continue
		}

		raw, err := json.Marshal(envelope)
		if err != nil {
			return rpcResponse{}, fmt.Errorf("decode rpc response: %w", err)
		}
		var resp rpcResponse
		if err := json.Unmarshal(raw, &resp); err != nil {
			return rpcResponse{}, fmt.Errorf("decode rpc response: %w", err)
		}
		return resp, nil
	}
}

type sseEvent struct {
	Name string
	Data string
}

func readSSEEvent(reader *bufio.Reader) (sseEvent, error) {
	var event sseEvent
	hasData := false

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return sseEvent{}, err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if hasData {
				return event, nil
			}
		} else if strings.HasPrefix(line, ":") {
			// ignore comment/heartbeat
		} else if strings.HasPrefix(line, "event:") {
			event.Name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			hasData = true
		} else if strings.HasPrefix(line, "data:") {
			part := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if event.Data == "" {
				event.Data = part
			} else {
				event.Data += "\n" + part
			}
			hasData = true
		}

		if err == io.EOF {
			if hasData {
				return event, nil
			}
			return sseEvent{}, io.EOF
		}
	}
}

func resolveSSEEndpoint(baseEndpoint, eventData string) (string, error) {
	if eventData == "" {
		return "", fmt.Errorf("empty sse endpoint event")
	}
	baseURL, err := url.Parse(baseEndpoint)
	if err != nil {
		return "", fmt.Errorf("parse base endpoint: %w", err)
	}
	ref, err := url.Parse(eventData)
	if err != nil {
		return "", fmt.Errorf("parse sse endpoint: %w", err)
	}
	return baseURL.ResolveReference(ref).String(), nil
}

func sameRPCID(a, b any) bool {
	return strings.TrimSpace(fmt.Sprintf("%v", a)) == strings.TrimSpace(fmt.Sprintf("%v", b))
}
