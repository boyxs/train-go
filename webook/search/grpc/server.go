package grpc

import (
	searchv1 "github.com/boyxs/train-go/webook/api/gen/search/v1"
	"github.com/boyxs/train-go/webook/search/service"
)

// SearchServer 实现 SearchService gRPC，聚合各 biz 的检索能力。
// RPC 方法按 biz 分文件（article.go，将来 user.go 等）；pb↔domain 映射与方法同文件。
type SearchServer struct {
	searchv1.UnimplementedSearchServiceServer
	articleSvc service.ArticleService
}

func NewSearchServer(articleSvc service.ArticleService) *SearchServer {
	return &SearchServer{articleSvc: articleSvc}
}
