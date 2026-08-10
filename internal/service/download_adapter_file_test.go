package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAria2AdapterRemoveClearsTaskAndResult(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req aria2Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode aria2 request: %v", err)
			return
		}
		if req.Method != "aria2.getVersion" {
			methods = append(methods, req.Method)
		}
		result := interface{}("OK")
		if req.Method == "aria2.getVersion" {
			result = map[string]interface{}{"version": "1.37"}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer server.Close()

	adapter := NewAria2Adapter()
	if err := adapter.Initialize(t.Context(), DownloadClientConfig{Host: server.URL}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Remove(t.Context(), "aria2-gid", true); err != nil {
		t.Fatal(err)
	}
	if len(methods) != 2 || methods[0] != "aria2.remove" || methods[1] != "aria2.removeDownloadResult" {
		t.Fatalf("methods = %#v", methods)
	}
}

func TestAria2AdapterListReportsConnectionFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req aria2Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode aria2 request: %v", err)
			return
		}
		response := map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": map[string]interface{}{"version": "1.37"}}
		if req.Method != "aria2.getVersion" {
			response = map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "error": map[string]interface{}{"code": 1, "message": "offline"}}
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	adapter := NewAria2Adapter()
	if err := adapter.Initialize(t.Context(), DownloadClientConfig{Host: server.URL}); err != nil {
		t.Fatal(err)
	}
	_, err := adapter.List(t.Context(), "")
	if err == nil || !errors.Is(err, errAria2ListUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestTransmissionAdapterAddsTorrentFileAsMetainfo(t *testing.T) {
	payload := []byte("d4:infod4:name5:movieee")
	var added map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("X-Transmission-Session-Id", "session-test")
			w.WriteHeader(http.StatusConflict)
			return
		}
		var req transmissionRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode transmission request: %v", err)
			return
		}
		if req.Method != "torrent-add" {
			t.Errorf("transmission method = %q, want torrent-add", req.Method)
			return
		}
		added = req.Arguments
		_ = json.NewEncoder(w).Encode(transmissionRPCResponse{
			Result: "success",
			Arguments: map[string]interface{}{
				"torrent-added": map[string]interface{}{"hashString": "transmission-file-hash"},
			},
		})
	}))
	defer server.Close()

	adapter := NewTransmissionAdapter()
	if err := adapter.Initialize(t.Context(), DownloadClientConfig{Host: server.URL}); err != nil {
		t.Fatal(err)
	}
	hash, err := adapter.AddTorrentFile(t.Context(), payload, "movie.torrent", "/downloads/movies")
	if err != nil {
		t.Fatal(err)
	}
	if hash != "transmission-file-hash" {
		t.Fatalf("hash = %q", hash)
	}
	if added["metainfo"] != base64.StdEncoding.EncodeToString(payload) {
		t.Fatalf("metainfo = %#v", added["metainfo"])
	}
	if added["download-dir"] != "/downloads/movies" {
		t.Fatalf("download-dir = %#v", added["download-dir"])
	}
	if _, ok := added["filename"]; ok {
		t.Fatalf("torrent file request unexpectedly included filename: %#v", added)
	}
}

func TestAria2AdapterAddsTorrentFileWithAddTorrent(t *testing.T) {
	payload := []byte("d4:infod4:name5:movieee")
	var addParams []interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req aria2Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode aria2 request: %v", err)
			return
		}
		result := interface{}(map[string]interface{}{"version": "1.37"})
		if req.Method == "aria2.addTorrent" {
			addParams = req.Params
			result = "aria2-gid"
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		})
	}))
	defer server.Close()

	adapter := NewAria2Adapter()
	if err := adapter.Initialize(t.Context(), DownloadClientConfig{Host: server.URL, Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	gid, err := adapter.AddTorrentFile(t.Context(), payload, "movie.torrent", "/downloads/movies")
	if err != nil {
		t.Fatal(err)
	}
	if gid != "aria2-gid" {
		t.Fatalf("gid = %q", gid)
	}
	if len(addParams) != 4 {
		t.Fatalf("aria2 params = %#v", addParams)
	}
	if addParams[0] != "token:secret" || addParams[1] != base64.StdEncoding.EncodeToString(payload) {
		t.Fatalf("aria2 params = %#v", addParams)
	}
	options, ok := addParams[3].(map[string]interface{})
	if !ok || options["dir"] != "/downloads/movies" {
		t.Fatalf("aria2 options = %#v", addParams[3])
	}
}

func TestQBitAdapterAddsTorrentFileWithCategory(t *testing.T) {
	payload := []byte("d4:infod4:name5:movieee")
	var gotCategory, gotSavePath, gotName string
	var gotPayload []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			reader, err := r.MultipartReader()
			if err != nil {
				t.Errorf("multipart reader: %v", err)
				return
			}
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("multipart next part: %v", err)
					return
				}
				value, _ := io.ReadAll(part)
				switch part.FormName() {
				case "torrents":
					gotName = part.FileName()
					gotPayload = value
				case "savepath":
					gotSavePath = string(value)
				case "category":
					gotCategory = string(value)
				}
			}
			_, _ = w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter := NewQBitAdapter()
	if err := adapter.Initialize(t.Context(), DownloadClientConfig{Host: server.URL}); err != nil {
		t.Fatal(err)
	}
	hash, err := adapter.AddTorrentFileWithCategory(t.Context(), payload, "movie.torrent", "/downloads/movies", "Movies")
	if err != nil {
		t.Fatal(err)
	}
	if hash != torrentInfoHash(payload) {
		t.Fatalf("hash = %q, want %q", hash, torrentInfoHash(payload))
	}
	if gotName != "movie.torrent" || string(gotPayload) != string(payload) || gotSavePath != "/downloads/movies" || gotCategory != "Movies" {
		t.Fatalf("multipart = name %q payload %q savepath %q category %q", gotName, gotPayload, gotSavePath, gotCategory)
	}
}
