package es

import (
	"context"

	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/refresh"
)

// ── B 文档 CRUD ──────────────────────────────────────────
// 写操作 Refresh(true)：demo/测试要写完立即可搜；生产勿这么用（性能差）。

// Index 按 Id upsert 文档（存在即覆盖）。
func (s *DocStore) Index(ctx context.Context, d Doc) error {
	_, err := s.client.Index(s.index).Id(docId(d.Id)).Document(d).Refresh(refresh.True).Do(ctx)
	return err
}

// Create 严格新建：文档已存在则返回 409 冲突错误（用 IsConflict 判定）。
func (s *DocStore) Create(ctx context.Context, d Doc) error {
	_, err := s.client.Create(s.index, docId(d.Id)).Document(d).Refresh(refresh.True).Do(ctx)
	return err
}

// Get 按 Id 取文档；不存在返回 (Doc{}, false, nil)。
func (s *DocStore) Get(ctx context.Context, id int64) (Doc, bool, error) {
	resp, err := s.client.Get(s.index, docId(id)).Do(ctx)
	if err != nil {
		if IsNotFound(err) {
			return Doc{}, false, nil
		}
		return Doc{}, false, err
	}
	if !resp.Found {
		return Doc{}, false, nil
	}
	d, err := decodeDoc(resp.Source_)
	if err != nil {
		return Doc{}, false, err
	}
	return d, true, nil
}

// Update 部分更新（partial doc merge）；文档不存在返回 404 错误（用 IsNotFound 判定）。
func (s *DocStore) Update(ctx context.Context, id int64, partial map[string]any) error {
	_, err := s.client.Update(s.index, docId(id)).Doc(partial).Refresh(refresh.True).Do(ctx)
	return err
}

// Delete 删除文档，返回是否确实删除了（文档本不存在返回 false, nil）。
func (s *DocStore) Delete(ctx context.Context, id int64) (bool, error) {
	resp, err := s.client.Delete(s.index, docId(id)).Refresh(refresh.True).Do(ctx)
	if err != nil {
		if IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return resp.Result.Name == "deleted", nil
}

// DocExists 判断文档是否存在。
func (s *DocStore) DocExists(ctx context.Context, id int64) (bool, error) {
	return s.client.Exists(s.index, docId(id)).Do(ctx)
}
