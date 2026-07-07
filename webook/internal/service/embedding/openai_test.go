package embedding_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/webook/internal/service/embedding"
)

func newEmbedServer(statusCode int, body any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func TestOpenAIClient_Embed(t *testing.T) {
	testCases := []struct {
		name       string
		text       string
		serverResp func() *httptest.Server
		wantErr    string
		wantDims   int
	}{
		{
			name: "成功返回向量",
			text: "健身饮食推荐",
			serverResp: func() *httptest.Server {
				vec := make([]float32, 1024)
				for i := range vec {
					vec[i] = 0.01
				}
				return newEmbedServer(http.StatusOK, map[string]any{
					"data": []map[string]any{
						{"embedding": vec, "index": 0},
					},
				})
			},
			wantDims: 1024,
		},
		{
			name: "API 返回 4xx",
			text: "test",
			serverResp: func() *httptest.Server {
				return newEmbedServer(http.StatusUnauthorized, map[string]any{
					"error": map[string]any{"message": "invalid api key"},
				})
			},
			wantErr: "status=401",
		},
		{
			name: "API 返回 5xx",
			text: "test",
			serverResp: func() *httptest.Server {
				return newEmbedServer(http.StatusInternalServerError, map[string]any{
					"error": map[string]any{"message": "internal error"},
				})
			},
			wantErr: "status=500",
		},
		{
			name: "响应 JSON 解析失败",
			text: "test",
			serverResp: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("not json"))
				}))
			},
			wantErr: "unmarshal",
		},
		{
			name: "响应 embeddings 为空",
			text: "test",
			serverResp: func() *httptest.Server {
				return newEmbedServer(http.StatusOK, map[string]any{
					"data": []map[string]any{},
				})
			},
			wantErr: "empty",
		},
		{
			name:    "text 为空字符串",
			text:    "",
			wantErr: "text 不能为空",
			serverResp: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
			},
		},
		{
			name: "ctx 已取消",
			text: "test",
			serverResp: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
			},
			wantErr: "context canceled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			srv := tc.serverResp()
			defer srv.Close()

			client := embedding.NewOpenAIClient(embedding.Config{
				BaseUrl: srv.URL,
				ApiKey:  "test-key",
				Model:   "text-embedding-v3",
				Dims:    1024,
				Timeout: 5 * time.Second,
			})

			ctx := context.Background()
			if tc.name == "ctx 已取消" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			vec, err := client.Embed(ctx, tc.text)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErr)
				assert.Nil(t, vec)
			} else {
				require.NoError(t, err)
				assert.Len(t, vec, tc.wantDims)
			}
		})
	}
}
