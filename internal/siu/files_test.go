package siu

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUploadImageThenSendPhoto(t *testing.T) {
	data := []byte("sample image bytes")
	digest := md5.Sum(data)
	wantMD5 := base64.StdEncoding.EncodeToString(digest[:])
	var uploaded bool
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.Header.Get("X-Api-Key") != "" || r.ContentLength != int64(len(data)) || r.Header.Get("Content-Type") != "image/png" || r.Header.Get("Content-MD5") != wantMD5 {
			t.Errorf("incorrect signed upload headers: %v", r.Header)
		}
		got, err := io.ReadAll(r.Body)
		if err != nil || string(got) != string(data) {
			t.Errorf("upload bytes changed: %s %v", got, err)
		}
		uploaded = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer storage.Close()
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/bot/file.uploadMedia":
			var metadata UploadMediaRequest
			if err := json.NewDecoder(r.Body).Decode(&metadata); err != nil {
				t.Error(err)
			}
			if metadata.ContentLength != uint64(len(data)) || metadata.ContentType != "image/png" || metadata.ContentMD5 != wantMD5 {
				t.Errorf("incorrect upload metadata: %+v", metadata)
			}
			json.NewEncoder(w).Encode(map[string]any{"code": "SUCCESS", "data": UploadMediaResponse{FileID: "uploaded-image", UploadURL: storage.URL + "/file?signature=secret"}})
		case "/api/v1/bot/message.sendPhoto":
			var body map[string]json.RawMessage
			json.NewDecoder(r.Body).Decode(&body)
			if !uploaded || string(body["file_id"]) != `"uploaded-image"` || string(body["to_group_id"]) != "42" {
				t.Errorf("invalid photo request: %v", body)
			}
			if _, ok := body["to_user_id"]; ok {
				t.Error("photo sent two targets")
			}
			if _, ok := body["topic_id"]; ok {
				t.Error("group photo has topic_id")
			}
			io.WriteString(w, `{"code":"SUCCESS","data":{"message_id":"photo"}}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	})
	ctx := context.Background()
	file, err := client.UploadImage(ctx, "image/png", data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SendPhoto(ctx, SendPhotoRequest{ToGroupID: 42, FileID: file.FileID, Width: 10, Height: 20}); err != nil {
		t.Fatal(err)
	}
}
