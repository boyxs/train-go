package viperx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

func TestExpandEnv(t *testing.T) {
	t.Setenv("VIPERX_TEST_PASS", "s3cr3t")
	t.Setenv("VIPERX_TEST_NESTED", "a${VIPERX_TEST_PASS}b") // 值里含 ${...},验证不二次展开

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"展开 ${NAME}", "password: ${VIPERX_TEST_PASS}", "password: s3cr3t"},
		{"未设置 → 空", "password: ${VIPERX_TEST_UNSET}", "password: "},
		{"裸 $$ 原样保留", "dsn: root:p@$$w0rd@tcp", "dsn: root:p@$$w0rd@tcp"},
		{"裸 $FOO 原样保留", "path: $HOME/x", "path: $HOME/x"},
		{"单遍替换不二次展开", "v: ${VIPERX_TEST_NESTED}", "v: a${VIPERX_TEST_PASS}b"},
		{"一行多个占位", "u: ${VIPERX_TEST_PASS}-${VIPERX_TEST_PASS}", "u: s3cr3t-s3cr3t"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := expandEnvValue(tc.in); got != tc.want {
				t.Fatalf("expandEnvValue(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestExpandTreeInjectionSafe 证明「解析后再展开」结构上不可注入:密钥值含 yaml 特殊字符
// (冒号空格 / # / 换行 / 引号 / 花括号)时,原样落到对应键,不破坏结构、不注入伪键,slice 元素也覆盖到。
func TestExpandTreeInjectionSafe(t *testing.T) {
	nasty := "p@ss: w0rd #x\nINJECTED: true\n\"q\" {flow} ${NOT_EXPANDED}"
	t.Setenv("VIPERX_NASTY", nasty)

	raw := []byte(`
data:
  redis:
    password: ${VIPERX_NASTY}
  mysql:
    dsn: root:${VIPERX_NASTY}@tcp
llm:
  providers:
    - name: a
      api_key: ${VIPERX_NASTY}
`)
	var tree map[string]any
	if err := yaml.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	expandTree(tree)

	v := viper.New()
	if err := v.MergeConfigMap(tree); err != nil {
		t.Fatal(err)
	}
	if got := v.GetString("data.redis.password"); got != nasty {
		t.Fatalf("password 应原样落地\n got=%q\nwant=%q", got, nasty)
	}
	if got := v.GetString("data.mysql.dsn"); got != "root:"+nasty+"@tcp" {
		t.Fatalf("嵌入式 dsn 应原样\n got=%q", got)
	}
	providers, ok := v.Get("llm.providers").([]any)
	if !ok || len(providers) != 1 {
		t.Fatalf("providers 结构丢失: %#v", v.Get("llm.providers"))
	}
	if got := providers[0].(map[string]any)["api_key"]; got != nasty {
		t.Fatalf("slice 元素 api_key 应展开为原样\n got=%v", got)
	}
	// 恶意值里的换行/伪键绝不能变成真的配置键（结构未被污染）
	if v.IsSet("injected") || v.IsSet("INJECTED") {
		t.Fatal("恶意值不得注入出新键 INJECTED")
	}
}

func TestLoadDotEnv(t *testing.T) {
	t.Run("文件不存在→nil 无错", func(t *testing.T) {
		if err := loadDotEnv(filepath.Join(t.TempDir(), "nope.env")); err != nil {
			t.Fatalf("缺文件应返回 nil, 实得 %v", err)
		}
	})

	t.Run("注入未设置的键 + 跳过注释/空行 + 剥引号 + 值含=保留", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), ".env")
		content := "" +
			"# 注释行\n" +
			"\n" +
			"VIPERX_DOTENV_A=hello\n" +
			"export VIPERX_DOTENV_B=\"quoted val\"\n" +
			"VIPERX_DOTENV_DSN=root:p@tcp(h:3306)/db?a=b&c=d\n"
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"VIPERX_DOTENV_A", "VIPERX_DOTENV_B", "VIPERX_DOTENV_DSN"} {
			os.Unsetenv(k)
			t.Cleanup(func() { os.Unsetenv(k) })
		}
		if err := loadDotEnv(p); err != nil {
			t.Fatal(err)
		}
		if got := os.Getenv("VIPERX_DOTENV_A"); got != "hello" {
			t.Fatalf("A=%q want hello", got)
		}
		if got := os.Getenv("VIPERX_DOTENV_B"); got != "quoted val" {
			t.Fatalf("B=%q want 'quoted val'（export 前缀 + 引号应剥）", got)
		}
		if got := os.Getenv("VIPERX_DOTENV_DSN"); got != "root:p@tcp(h:3306)/db?a=b&c=d" {
			t.Fatalf("DSN=%q 值里的 = 应原样保留", got)
		}
	})

	t.Run("已有真实环境变量优先, 不被 .env 覆盖", func(t *testing.T) {
		t.Setenv("VIPERX_DOTENV_WIN", "real")
		p := filepath.Join(t.TempDir(), ".env")
		if err := os.WriteFile(p, []byte("VIPERX_DOTENV_WIN=fromfile\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := loadDotEnv(p); err != nil {
			t.Fatal(err)
		}
		if got := os.Getenv("VIPERX_DOTENV_WIN"); got != "real" {
			t.Fatalf("WIN=%q, 真实 env 应优先于 .env", got)
		}
	})
}
