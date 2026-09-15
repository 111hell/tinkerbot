package siu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamWireContract(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if _, ok := body["session_id"]; ok {
			t.Error("removed session_id was sent")
		}
		if string(body["user_message_id"]) != `"user-message"` {
			t.Error("missing original user message ID")
		}
		if strings.HasSuffix(r.URL.Path, "message.sendStreamMessage") && string(body["topic_id"]) != `"topic"` {
			t.Error("missing topic ID")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"SUCCESS","data":{"message_id":"reply"}}`))
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "test-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	first, err := c.SendStreamMessage(context.Background(), SendStreamMessageRequest{ToUserID: 83, TopicID: "topic", UserMessageID: "user-message", Text: "one"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.SendStreamMessage(context.Background(), SendStreamMessageRequest{ToUserID: 83, TopicID: "topic", UserMessageID: "user-message", MessageID: first.MessageID, Index: 1, Text: "two"})
	if err != nil {
		t.Fatal(err)
	}
	answer := "onetwo"
	_, err = c.FinishStreamMessage(context.Background(), FinishStreamMessageRequest{ToUserID: 83, UserMessageID: "user-message", MessageID: first.MessageID, Text: &answer})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("got %d calls", calls)
	}
}

func TestStreamClosedErrorOnHTTP400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"CHAT_MESSAGE_STREAM_CLOSED","message":"Invalid argument.","data":null}`))
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "test-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.SendStreamMessage(context.Background(), SendStreamMessageRequest{ToUserID: 83, TopicID: "topic", UserMessageID: "user-message"})
	if !IsCode(err, "CHAT_MESSAGE_STREAM_CLOSED") {
		t.Fatalf("lost stream closed code: %v", err)
	}
	_, err = c.FinishStreamMessage(context.Background(), FinishStreamMessageRequest{ToUserID: 83, UserMessageID: "user-message", MessageID: "reply"})
	if !IsCode(err, "CHAT_MESSAGE_STREAM_CLOSED") {
		t.Fatalf("lost finish closed code: %v", err)
	}
}
