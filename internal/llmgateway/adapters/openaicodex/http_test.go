package openaicodex

import "testing"

func TestParseEventStreamResponse_UsesCompletedEventPayload(t *testing.T) {
	stream := "" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"O\"}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"K\"}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"OK\"}]}],\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5}}}\n\n"

	resp, err := parseEventStreamResponse([]byte(stream))
	if err != nil {
		t.Fatalf("parseEventStreamResponse error: %v", err)
	}
	if resp.Content != "OK" {
		t.Fatalf("expected content OK, got %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 5 {
		t.Fatalf("expected usage total 5, got %+v", resp.Usage)
	}
}
