package grpcx

import (
	"strings"
	"testing"
	"time"
)

func TestClientConfig_serviceConfigJSON(t *testing.T) {
	tests := []struct {
		name     string
		cfg      ClientConfig
		empty    bool
		want     string   // 精确匹配(单键 map,确定性序列化)
		contains []string // 子串匹配(多键 map 序不定时用)
	}{
		{name: "无任何段 → 空串", cfg: ClientConfig{Target: "x"}, empty: true},
		{
			name: "仅 balancer",
			cfg:  ClientConfig{Balancer: "breaker_swrr"},
			want: `{"loadBalancingConfig":[{"breaker_swrr":{}}]}`,
		},
		{
			name:     "balancer + timeout → methodConfig 默认条目",
			cfg:      ClientConfig{Balancer: "breaker_swrr", Timeout: 3 * time.Second},
			contains: []string{`"loadBalancingConfig":[{"breaker_swrr":{}}]`, `"methodConfig":[{"name":[{}],"timeout":"3s"}]`},
		},
		{
			name:     "healthCheck",
			cfg:      ClientConfig{Balancer: "breaker_swrr", HealthCheck: true},
			contains: []string{`"healthCheckConfig":{"serviceName":""}`},
		},
		{
			name: "retry 全方法 → 并入默认条目",
			cfg: ClientConfig{Timeout: 2 * time.Second, Retry: &GRPCRetry{
				MaxAttempts: 3, InitialBackoff: 10 * time.Millisecond, MaxBackoff: 100 * time.Millisecond,
				BackoffMultiplier: 2, RetryableCodes: []string{"UNAVAILABLE"},
			}},
			contains: []string{`"name":[{}]`, `"timeout":"2s"`, `"retryPolicy":{"maxAttempts":3,"initialBackoff":"0.01s","maxBackoff":"0.1s","backoffMultiplier":2,"retryableStatusCodes":["UNAVAILABLE"]}`},
		},
		{
			name: "retry 限定方法 → 独立条目带 timeout(不丢超时) + 默认条目仅 timeout",
			cfg: ClientConfig{Balancer: "breaker_swrr", Timeout: 3 * time.Second, Retry: &GRPCRetry{
				MaxAttempts: 3, InitialBackoff: 10 * time.Millisecond, MaxBackoff: 100 * time.Millisecond,
				BackoffMultiplier: 2, RetryableCodes: []string{"UNAVAILABLE"}, Methods: []string{"pkg.Svc/M"},
			}},
			contains: []string{
				`"name":[{}],"timeout":"3s"`,          // 默认条目:所有方法仅 timeout(retry 限定到具体方法,不在此)
				`"service":"pkg.Svc"`, `"method":"M"`, // 方法条目命中 pkg.Svc/M
				`"timeout":"3s","retryPolicy":`, // 方法条目带 timeout 再接 retry,证明不丢超时
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cfg.serviceConfigJSON()
			if err != nil {
				t.Fatal(err)
			}
			if tt.empty {
				if got != "" {
					t.Fatalf("want empty, got %q", got)
				}
				return
			}
			if tt.want != "" && got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			for _, sub := range tt.contains {
				if !strings.Contains(got, sub) {
					t.Fatalf("got %q, missing %q", got, sub)
				}
			}
		})
	}
}

func TestDurString(t *testing.T) {
	cases := map[time.Duration]string{
		3 * time.Second:         "3s",
		250 * time.Millisecond:  "0.25s",
		10 * time.Millisecond:   "0.01s",
		1500 * time.Millisecond: "1.5s",
	}
	for d, want := range cases {
		if got := durString(d); got != want {
			t.Errorf("durString(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestParseMethodNames(t *testing.T) {
	got, err := parseMethodNames([]string{"pkg.Svc", "pkg.Svc/Method"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	if got[0]["service"] != "pkg.Svc" || len(got[0]) != 1 {
		t.Errorf("entry0 = %v, want {service:pkg.Svc}", got[0])
	}
	if got[1]["service"] != "pkg.Svc" || got[1]["method"] != "Method" {
		t.Errorf("entry1 = %v, want {service:pkg.Svc, method:Method}", got[1])
	}
	if _, err := parseMethodNames([]string{"/M"}); err == nil {
		t.Error("want error for empty service")
	}
}

func TestGRPCRetry_policy(t *testing.T) {
	// 缺省字段就地兜底:attempts clamp 到 [2,5];backoff / mult / codes 补默认。
	got := (&GRPCRetry{MaxAttempts: 1}).policy() // 1 < 2 → 3
	if got.MaxAttempts != 3 {
		t.Errorf("MaxAttempts=1 应兜底 3, got %d", got.MaxAttempts)
	}
	if got.InitialBackoff != "0.1s" || got.MaxBackoff != "1s" || got.BackoffMultiplier != 2 {
		t.Errorf("backoff 默认应 0.1s/1s/2, got %s/%s/%v", got.InitialBackoff, got.MaxBackoff, got.BackoffMultiplier)
	}
	if len(got.RetryableStatusCodes) != 1 || got.RetryableStatusCodes[0] != "UNAVAILABLE" {
		t.Errorf("codes 默认应 [UNAVAILABLE], got %v", got.RetryableStatusCodes)
	}
	if capped := (&GRPCRetry{MaxAttempts: 9}).policy(); capped.MaxAttempts != 5 { // 9 > 5 → 封顶 5
		t.Errorf("MaxAttempts=9 应封顶 5, got %d", capped.MaxAttempts)
	}
}
