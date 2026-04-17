package embedding_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webook/config"
	"github.com/webook/internal/service/ai/embedding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaClient_Embed(t *testing.T) {
	t.Run("正常返回向量", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/embeddings", r.URL.Path)
			assert.Equal(t, "POST", r.Method)

			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "bge-m3", body["model"])
			assert.Equal(t, "大白菜", body["prompt"])

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"embedding": []float32{0.1, 0.2, 0.3},
			})
		}))
		defer srv.Close()

		client := embedding.NewOllamaClient(config.OllamaEmbeddingConfig{
			BaseUrl: srv.URL,
			Model:   "bge-m3",
			Timeout: 5,
		})

		vec, err := client.Embed(context.Background(), "大白菜")
		require.NoError(t, err)
		assert.Equal(t, []float32{0.1, 0.2, 0.3}, vec)
	})

	t.Run("空文本报错", func(t *testing.T) {
		client := embedding.NewOllamaClient(config.OllamaEmbeddingConfig{
			BaseUrl: "http://localhost:11434",
			Model:   "bge-m3",
		})
		_, err := client.Embed(context.Background(), "")
		assert.ErrorContains(t, err, "不能为空")
	})

	t.Run("非200状态码", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"model not found"}`))
		}))
		defer srv.Close()

		client := embedding.NewOllamaClient(config.OllamaEmbeddingConfig{
			BaseUrl: srv.URL,
			Model:   "bge-m3",
		})
		_, err := client.Embed(context.Background(), "hello")
		assert.ErrorContains(t, err, "status=500")
	})

	t.Run("连接拒绝", func(t *testing.T) {
		client := embedding.NewOllamaClient(config.OllamaEmbeddingConfig{
			BaseUrl: "http://127.0.0.1:1",
			Model:   "bge-m3",
			Timeout: 1,
		})
		_, err := client.Embed(context.Background(), "hello")
		assert.ErrorContains(t, err, "do request")
	})

	t.Run("响应JSON解析失败", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`not json`))
		}))
		defer srv.Close()

		client := embedding.NewOllamaClient(config.OllamaEmbeddingConfig{
			BaseUrl: srv.URL,
			Model:   "bge-m3",
		})
		_, err := client.Embed(context.Background(), "hello")
		assert.ErrorContains(t, err, "unmarshal")
	})
}
