package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// fakeAnthropic returns an httptest.Server that responds to POST
// /v1/messages with a Messages-API-shaped envelope whose only text
// content block is the given string. Used to drive normalizeWorkQuery
// from a known LLM output without hitting the real API.
func fakeAnthropic(t *testing.T, modelText string) *httptest.Server {
	t.Helper()
	encoded, err := json.Marshal(modelText)
	if err != nil {
		t.Fatalf("marshal canned text: %v", err)
	}
	body := fmt.Sprintf(`{
  "id": "msg_test",
  "type": "message",
  "role": "assistant",
  "model": "claude-opus-4-7",
  "stop_reason": "end_turn",
  "content": [{"type": "text", "text": %s}],
  "usage": {"input_tokens": 100, "output_tokens": 20}
}`, string(encoded))
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("x-api-key") == "" {
			t.Errorf("expected x-api-key header to be set")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Errorf("expected anthropic-version header to be set")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func clientForServer(server *httptest.Server) anthropic.Client {
	return anthropic.NewClient(
		option.WithBaseURL(server.URL),
		option.WithAPIKey("test-key"),
	)
}

func TestNormalizeWorkQuery_ParsesAlternatives(t *testing.T) {
	server := fakeAnthropic(t, `{"alternatives": ["BWV 232", "h-Moll-Messe", "Mass in B Minor"]}`)
	defer server.Close()
	client := clientForServer(server)

	got, err := normalizeWorkQuery(context.Background(), client, "Bach", "Mass in B minor")
	if err != nil {
		t.Fatalf("normalizeWorkQuery: %v", err)
	}
	// "Mass in B Minor" should be deduped against the original
	// "Mass in B minor" (case-insensitive).
	want := []string{"BWV 232", "h-Moll-Messe"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNormalizeWorkQuery_DedupesAndDropsBlanks(t *testing.T) {
	server := fakeAnthropic(t, `{"alternatives": ["BWV 232", "BWV 232", "", "  ", "h-Moll-Messe"]}`)
	defer server.Close()
	client := clientForServer(server)

	got, err := normalizeWorkQuery(context.Background(), client, "Bach", "Mass in B minor")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"BWV 232", "h-Moll-Messe"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNormalizeWorkQuery_EmptyAlternatives(t *testing.T) {
	server := fakeAnthropic(t, `{"alternatives": []}`)
	defer server.Close()
	client := clientForServer(server)

	got, err := normalizeWorkQuery(context.Background(), client, "Bach", "Anything")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty slice", got)
	}
}

func TestNormalizeWorkQuery_InvalidJSON(t *testing.T) {
	// LLM ignored the prompt and returned prose instead of JSON.
	server := fakeAnthropic(t, `Sure! Here are some alternatives: BWV 232, h-Moll-Messe.`)
	defer server.Close()
	client := clientForServer(server)

	_, err := normalizeWorkQuery(context.Background(), client, "Bach", "Mass in B minor")
	if err == nil {
		t.Fatal("expected error for non-JSON LLM output, got nil")
	}
}
