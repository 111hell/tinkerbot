package siu

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL+"/api/v1/", "test-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestSetWebhookOnlyUpdatesCallback(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/bot/bot.update" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 1 || body["webhook_url"] != "http://localhost:18080/webhook" {
			t.Errorf("unexpected update: %v", body)
		}
		io.WriteString(w, `{"code":"SUCCESS","data":{"id":6888}}`)
	})
	if err := client.SetWebhook(context.Background(), "http://localhost:18080/webhook"); err != nil {
		t.Fatal(err)
	}
}

func TestClientErrorsAndRedirectAuthentication(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				io.WriteString(w, `{"code":"NO_PERMISSION","message":"privileged Bot required","data":null}`)
			})
			_, err := client.WhoAmI(context.Background())
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != status || !IsCode(err, "NO_PERMISSION") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = true
	}))
	defer other.Close()
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	})
	_, err := client.WhoAmI(context.Background())
	if err == nil || leaked || strings.Contains(err.Error(), "test-token") {
		t.Fatalf("redirect leaked authentication or was accepted: %v", err)
	}
}

func TestNullSuccessForNoDataMethods(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"code":"SUCCESS","data":null}`)
	})
	ctx := context.Background()
	if err := client.SendEvent(ctx, SendEventRequest{UserID: 1, EventType: "progress"}); err != nil {
		t.Fatal(err)
	}
	if err := client.RevokeMessage(ctx, RevokeMessageRequest{ToUserID: 1, MessageID: "reply"}); err != nil {
		t.Fatal(err)
	}
	if err := client.EmptyStreamMessage(ctx, EmptyStreamMessageRequest{ToUserID: 1, MessageID: "reply"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.WhoAmI(ctx); err == nil {
		t.Fatal("missing typed response data was accepted")
	}
}

func TestReadEndpointsAndProtobufJSON(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/bot/bot.getUpdates":
			if r.Method != http.MethodGet || r.URL.Query().Get("start_id") != "10-0" || r.URL.Query().Get("timeout") != "30" || r.URL.Query().Get("no_trim") != "true" || r.URL.Query().Get("limit") != "100" {
				t.Errorf("unexpected poll request: %s", r.URL)
			}
			io.WriteString(w, `{"code":"SUCCESS","data":{"events":[{"id":"11-0","event_type":"user_message","data":{"message":{"message_id":"m","user_id":1,"attachment":{"file_stack":[{"id":"f","filename":"notes.txt","url":"https://files.example/notes","content_type":"text/plain","thumbnail_file":{"id":"thumb"}}]}}}}]}}`)
		case "/api/v1/bot/message.listRecentUsers":
			if r.Method != http.MethodGet || r.URL.Query().Get("before_message_id") != "cursor" || r.URL.Query().Get("limit") != "25" {
				t.Errorf("unexpected recent users query: %s", r.URL)
			}
			io.WriteString(w, `{"code":"SUCCESS","data":{"users":[{"user_id":7,"latest_message":{"message_id":"m","from_user_id":7,"to_user_id":8,"created_at":"2026-09-12T00:00:00Z"}}],"has_more":true}}`)
		case "/api/v1/bot/user.queryUser":
			var body QueryUserRequest
			json.NewDecoder(r.Body).Decode(&body)
			if r.Method != http.MethodPost || body.Username != "alice" {
				t.Errorf("unexpected user query: %+v", body)
			}
			io.WriteString(w, `{"code":"SUCCESS","data":{"id":7,"username":"alice","avatar_url":"https://example/avatar"}}`)
		case "/api/v1/bot/message.searchChats":
			var body SearchChatsRequest
			json.NewDecoder(r.Body).Decode(&body)
			if body.UserID != 7 || body.BotID == nil || *body.BotID != 0 {
				t.Errorf("lost zero bot filter: %+v", body)
			}
			io.WriteString(w, `{"code":"SUCCESS","data":{"results":[{"fromUserId":"18446744073709551615","toGroupOrUserId":"8","latestMatchedMsgAt":"2026-09-12T00:00:00Z","matchedMsgs":[{"msgId":"m","snippet":"hello","createdAt":"2026-09-12T00:00:00Z"}]}]}}`)
		case "/api/v1/bot/user.searchContacts":
			io.WriteString(w, `{"code":"SUCCESS","data":{"results":[{"user":{"id":"18446744073709551615","avatarUrl":"https://example/avatar"},"displayName":"Alice","matchedField":"SEARCH_CONTACT_MATCH_FIELD_ALIAS"}]}}`)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	ctx := context.Background()
	updates, err := client.GetUpdates(ctx, GetUpdatesRequest{Limit: 100, Timeout: 30, StartID: "10-0", NoTrim: true})
	if err != nil {
		t.Fatal(err)
	}
	if updates.Events[0].Data.Message.Attachment.FileStack[0].ThumbnailFile.ID != "thumb" {
		t.Fatal("lost file attachment")
	}
	recent, err := client.ListRecentUsers(ctx, "cursor", 25)
	if err != nil || recent == nil || len(recent.Users) != 1 || !recent.HasMore {
		t.Fatalf("recent users: %+v, %v", recent, err)
	}
	user, err := client.QueryUser(ctx, QueryUserRequest{Username: "alice"})
	if err != nil || user == nil || user.ID != 7 || user.AvatarURL == "" {
		t.Fatalf("user: %+v, %v", user, err)
	}
	zero := uint64(0)
	chats, err := client.SearchChats(ctx, SearchChatsRequest{UserID: 7, Query: "hello", BotID: &zero})
	if err != nil {
		t.Fatal(err)
	}
	if chats.Results[0].FromUserID != "18446744073709551615" || chats.Results[0].MatchedMsgs[0].MessageID != "m" {
		t.Fatalf("lost protobuf fields: %+v", chats.Results[0])
	}
	contacts, err := client.SearchContacts(ctx, SearchContactsRequest{UserID: 7, Query: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	if contacts.Results[0].User.ID != "18446744073709551615" || contacts.Results[0].DisplayName != "Alice" {
		t.Fatal("lost contact protobuf fields")
	}
}

func TestFinishStreamPresenceAndRegeneration(t *testing.T) {
	var step int
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		step++
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if _, ok := body["to_group_id"]; ok {
			t.Error("stream sent a group target")
		}
		if _, ok := body["session_id"]; ok {
			t.Error("stream sent legacy session_id")
		}
		if step == 1 {
			if r.URL.Path != "/api/v1/bot/message.emptyStreamMessage" {
				t.Error("regeneration did not clear old reply")
			}
		} else if step == 2 {
			if string(body["message_id"]) != `"reply"` || string(body["user_message_id"]) != `"user-message"` || string(body["topic_id"]) != `"topic"` || string(body["index"]) != "0" {
				t.Errorf("regeneration IDs changed: %s", body)
			}
		} else {
			if _, ok := body["topic_id"]; ok {
				t.Error("finish sent topic_id")
			}
			if step == 3 {
				if _, ok := body["text"]; ok {
					t.Error("omitted text was supplied")
				}
				if string(body["rich_reasoning"]) != "null" {
					t.Error("nil reasoning must merge")
				}
			} else if string(body["text"]) != `""` || string(body["rich_reasoning"]) != "[]" {
				t.Error("explicit empty values were lost")
			}
		}
		io.WriteString(w, `{"code":"SUCCESS","data":{"message_id":"reply"}}`)
	})
	ctx := context.Background()
	if err := client.EmptyStreamMessage(ctx, EmptyStreamMessageRequest{ToUserID: 7, MessageID: "reply"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.SendStreamMessage(ctx, SendStreamMessageRequest{ToUserID: 7, TopicID: "topic", MessageID: "reply", UserMessageID: "user-message", Text: "new"}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.FinishStreamMessage(ctx, FinishStreamMessageRequest{ToUserID: 7, MessageID: "reply", UserMessageID: "user-message"}); err != nil {
		t.Fatal(err)
	}
	empty := ""
	if _, err := client.FinishStreamMessage(ctx, FinishStreamMessageRequest{ToUserID: 7, MessageID: "reply", UserMessageID: "user-message", Text: &empty, RichReasoning: []string{}}); err != nil {
		t.Fatal(err)
	}
}
