package siu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/111hell/tinker/agent"
	"github.com/111hell/tinkerbot/internal/conversation"
	siuc "github.com/111hell/tinkerbot/internal/siu"
)

func TestPrivateToolsRejectGroupAndForgedOwner(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unauthorized tool reached SIU") }))
	defer server.Close()
	client, err := siuc.NewClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []agent.Tool{NewListMessages(client), NewSearchContacts(client), NewSearchChats(client), NewSendEvent(client)} {
		t.Run(tool.Name, func(t *testing.T) {
			args := `{}`
			if tool.Name == SearchChatsName || tool.Name == SearchContactsName {
				args = `{"query":"hello"}`
			}
			if tool.Name == SendEventName {
				args = `{"event_type":"progress"}`
			}
			group := conversation.WithCurrent(context.Background(), conversation.Current{UserID: 83, GroupID: 42})
			if _, err := tool.Execute(group, args); err == nil {
				t.Fatal("private tool ran in group")
			}
			if _, err := tool.Execute(context.Background(), args); err == nil {
				t.Fatal("tool ran without trusted identity")
			}
			private := conversation.WithCurrent(context.Background(), conversation.Current{UserID: 83})
			if _, err := tool.Execute(private, `{"user_id":999,"query":"hello"}`); err == nil {
				t.Fatal("model owner override accepted")
			}
		})
	}
}

func TestSearchUsesCurrentUserAndMessageToolsUseCurrentTarget(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/api/v1/bot/message.searchChats":
			if string(body["user_id"]) != "83" {
				t.Error("search owner not from context")
			}
			io.WriteString(w, `{"code":"SUCCESS","data":{"results":[]}}`)
		case "/api/v1/bot/message.sendPhoto", "/api/v1/bot/message.editMessage", "/api/v1/bot/message.revokeMessage":
			if string(body["to_group_id"]) != "42" {
				t.Error("wrong group target")
			}
			if _, ok := body["to_user_id"]; ok {
				t.Error("group tool used private target")
			}
			if _, ok := body["topic_id"]; ok {
				t.Error("group tool invented Topic")
			}
			io.WriteString(w, `{"code":"SUCCESS","data":{"message_id":"reply","text":"edited"}}`)
		default:
			t.Errorf("unexpected method: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := siuc.NewClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	private := conversation.WithCurrent(context.Background(), conversation.Current{UserID: 83, TopicID: "topic"})
	if _, err := NewSearchChats(client).Execute(private, `{"query":"hello"}`); err != nil {
		t.Fatal(err)
	}
	group := conversation.WithCurrent(context.Background(), conversation.Current{UserID: 83, GroupID: 42})
	for _, test := range []struct {
		tool agent.Tool
		args string
	}{
		{NewSendPhoto(client), `{"image_url":"https://example/photo.png","width":10,"height":20}`},
		{NewEditMessage(client), `{"message_id":"reply","text":"edited"}`},
		{NewRevokeMessage(client), `{"message_id":"reply"}`},
	} {
		if _, err := test.tool.Execute(group, test.args); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 4 {
		t.Fatalf("got %d API calls", calls)
	}
	if _, err := NewSendPhoto(client).Execute(group, `{"to_user_id":999,"image_url":"https://example/photo.png","width":10,"height":20}`); err == nil {
		t.Fatal("forged recipient accepted")
	}
}

func TestListMessagesKeepsPaginationWhenTopicPageIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("user_id") != "83" {
			t.Error("history owner not from context")
		}
		if r.URL.Query().Get("before_message_id") == "" {
			io.WriteString(w, `{"code":"SUCCESS","data":{"messages":[{"message_id":"other-topic-cursor","topic_id":"other-topic","text":"private other topic"}],"has_more":true}}`)
		} else {
			if r.URL.Query().Get("before_message_id") != "other-topic-cursor" {
				t.Error("wrong cursor")
			}
			io.WriteString(w, `{"code":"SUCCESS","data":{"messages":[{"message_id":"deleted","topic_id":"topic","text":"deleted-text","deleted_at":"2026-09-12T00:00:00Z"},{"message_id":"visible","topic_id":"topic","text":"visible-text"}],"has_more":false}}`)
		}
	}))
	defer server.Close()
	client, err := siuc.NewClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx := conversation.WithCurrent(context.Background(), conversation.Current{UserID: 83, TopicID: "topic"})
	tool := NewListMessages(client)
	first, err := tool.Execute(ctx, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	var page listMessagesResult
	if err := json.Unmarshal([]byte(first), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 0 || !page.HasMore || page.NextBeforeMessageID != "other-topic-cursor" {
		t.Fatalf("filtered page lost cursor: %s", first)
	}
	second, err := tool.Execute(ctx, `{"before_message_id":"other-topic-cursor"}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(second, "deleted-text") || !strings.Contains(second, "visible-text") {
		t.Fatalf("wrong history filter: %s", second)
	}
}

func TestQueryUserDefaultsToCurrentSpeaker(t *testing.T) {
	for _, tt := range []struct {
		name    string
		args    string
		groupID uint64
		want    siuc.QueryUserRequest
	}{
		{"self", `{}`, 0, siuc.QueryUserRequest{UserID: 83}},
		{"explicit self", `{"self":true}`, 0, siuc.QueryUserRequest{UserID: 83}},
		{"explicit group self", `{"self":true}`, 42, siuc.QueryUserRequest{UserID: 83}},
		{"group speaker", `{}`, 42, siuc.QueryUserRequest{UserID: 83}},
		{"blank selector", `{"username":"  "}`, 0, siuc.QueryUserRequest{UserID: 83}},
		{"other username", `{"username":" alice "}`, 0, siuc.QueryUserRequest{Username: "alice"}},
		{"other ID", `{"user_id":99}`, 0, siuc.QueryUserRequest{UserID: 99}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/api/v1/bot/user.queryUser" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				var got siuc.QueryUserRequest
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Error(err)
					return
				}
				if got != tt.want {
					t.Errorf("request = %+v, want %+v", got, tt.want)
				}
				io.WriteString(w, `{"code":"SUCCESS","data":{"id":83,"username":"test","nickname":"Test User"}}`)
			}))
			defer server.Close()
			client, err := siuc.NewClient(server.URL, "token", server.Client())
			if err != nil {
				t.Fatal(err)
			}
			tool := NewQueryUser(client)
			ctx := conversation.WithCurrent(context.Background(), conversation.Current{UserID: 83, GroupID: tt.groupID})
			if _, err := tool.Execute(ctx, tt.args); err != nil {
				t.Fatal(err)
			}
			if _, err := tool.Execute(context.Background(), tt.args); err == nil {
				t.Error("lookup without trusted identity succeeded")
			}
			for _, bad := range []string{`{"self":true,"user_id":99}`, `{"self":true,"username":"alice"}`} {
				if _, err := tool.Execute(ctx, bad); err == nil {
					t.Errorf("conflicting self selector accepted: %s", bad)
				}
			}
			if calls != 1 {
				t.Errorf("API calls = %d, want 1", calls)
			}
		})
	}
}
