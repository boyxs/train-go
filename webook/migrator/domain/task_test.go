package domain

import (
	"encoding/json"
	"testing"
)

func TestMode_Valid(t *testing.T) {
	testCases := []struct {
		name string
		m    Mode
		want bool
	}{
		{"dual_write 合法", ModeDualWrite, true},
		{"cdc 合法", ModeCDC, true},
		{"空字符串不合法", Mode(""), false},
		{"未知值不合法", Mode("foo"), false},
		{"大小写敏感不合法", Mode("DUAL_WRITE"), false},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.m.Valid(); got != c.want {
				t.Errorf("Mode(%q).Valid() = %v, want %v", c.m, got, c.want)
			}
		})
	}
}

func TestKind_Valid(t *testing.T) {
	testCases := []struct {
		name string
		k    Kind
		want bool
	}{
		{"cross_dc 合法", KindCrossDC, true},
		{"sharding 合法", KindSharding, true},
		{"schema 合法", KindSchema, true},
		{"heterogeneous 合法", KindHeterogeneous, true},
		{"空字符串不合法", Kind(""), false},
		{"未知值不合法", Kind("bar"), false},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.k.Valid(); got != c.want {
				t.Errorf("Kind(%q).Valid() = %v, want %v", c.k, got, c.want)
			}
		})
	}
}

func TestSourceType_Valid(t *testing.T) {
	testCases := []struct {
		name string
		s    SourceType
		want bool
	}{
		{"mysql 合法", SourceTypeMySQL, true},
		{"mongo 合法", SourceTypeMongo, true},
		{"空字符串不合法", SourceType(""), false},
		{"未知值不合法", SourceType("redis"), false},
		{"大小写敏感不合法", SourceType("MySQL"), false},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.s.Valid(); got != c.want {
				t.Errorf("SourceType(%q).Valid() = %v, want %v", c.s, got, c.want)
			}
		})
	}
}

func TestSourceType_Normalize(t *testing.T) {
	testCases := []struct {
		name string
		s    SourceType
		want SourceType
	}{
		{"空 → 默认 mysql", SourceType(""), SourceTypeMySQL},
		{"mysql 原样", SourceTypeMySQL, SourceTypeMySQL},
		{"mongo 原样", SourceTypeMongo, SourceTypeMongo},
		{"未知值原样（交给 Valid 拦）", SourceType("redis"), SourceType("redis")},
	}
	for _, c := range testCases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.s.Normalize(); got != c.want {
				t.Errorf("SourceType(%q).Normalize() = %q, want %q", c.s, got, c.want)
			}
		})
	}
}

func TestTask_PickTable(t *testing.T) {
	mustJSON := func(tables []TableMapping) string {
		b, _ := json.Marshal(tables)
		return string(b)
	}
	t.Run("正常单表", func(t *testing.T) {
		task := Task{Id: 42, TablesJSON: mustJSON([]TableMapping{
			{Src: "a", Dst: "b", PartitionKey: "uid"},
		})}
		tm, err := task.PickTable(0)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if tm.Src != "a" || tm.Dst != "b" || tm.PartitionKey != "uid" {
			t.Errorf("unexpected mapping: %+v", tm)
		}
	})
	t.Run("PartitionKey 空 → 兜底 id", func(t *testing.T) {
		task := Task{TablesJSON: mustJSON([]TableMapping{{Src: "a", Dst: "b"}})}
		tm, err := task.PickTable(0)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if tm.PartitionKey != "id" {
			t.Errorf("PartitionKey = %q, want %q", tm.PartitionKey, "id")
		}
	})
	t.Run("多表只取第一张", func(t *testing.T) {
		task := Task{TablesJSON: mustJSON([]TableMapping{
			{Src: "a", Dst: "b", PartitionKey: "id"},
			{Src: "c", Dst: "d", PartitionKey: "id"},
		})}
		tm, _ := task.PickTable(0)
		if tm.Src != "a" {
			t.Errorf("Src = %q, want a", tm.Src)
		}
	})
	t.Run("TablesJSON 空 → error", func(t *testing.T) {
		_, err := Task{Id: 1}.PickTable(0)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("TablesJSON 损坏 → error", func(t *testing.T) {
		_, err := Task{TablesJSON: "not-json"}.PickTable(0)
		if err == nil {
			t.Fatal("expected unmarshal error")
		}
	})
	t.Run("TablesJSON [] → error", func(t *testing.T) {
		_, err := Task{TablesJSON: "[]"}.PickTable(0)
		if err == nil {
			t.Fatal("expected no-entries error")
		}
	})
	t.Run("Src 缺失 → error", func(t *testing.T) {
		_, err := Task{TablesJSON: mustJSON([]TableMapping{{Dst: "x"}})}.PickTable(0)
		if err == nil {
			t.Fatal("expected missing src error")
		}
	})
}
