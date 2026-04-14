package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func newTestBuilder(reg *prometheus.Registry) *PrometheusBuilder {
	return NewPrometheusBuilder("webook", "http", "requests", "test").Registry(reg)
}

func TestBuildCounter_Increment(t *testing.T) {
	reg := prometheus.NewRegistry()
	server := gin.New()
	server.Use(newTestBuilder(reg).WithCounter().Build())
	server.GET("/hello", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hello", nil))

	count := testutil.CollectAndCount(reg, "webook_http_requests_total")
	assert.Equal(t, 1, count)
}

func TestBuildCounter_UsesFullPath(t *testing.T) {
	reg := prometheus.NewRegistry()
	server := gin.New()
	server.Use(newTestBuilder(reg).WithCounter().Build())
	server.GET("/article/:id", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	for _, id := range []string{"1", "2", "3"} {
		server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/article/"+id, nil))
	}

	metrics := gatherMetricsText(t, reg)
	assert.Contains(t, metrics, `value:"/article/:id"`)
	assert.NotContains(t, metrics, `value:"/article/1"`)
}

func TestBuildCounter_StatusLabel(t *testing.T) {
	reg := prometheus.NewRegistry()
	server := gin.New()
	server.Use(newTestBuilder(reg).WithCounter().Build())
	server.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })
	server.GET("/bad", func(c *gin.Context) { c.Status(http.StatusBadRequest) })

	server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ok", nil))
	server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/bad", nil))

	metrics := gatherMetricsText(t, reg)
	assert.Contains(t, metrics, `value:"200"`)
	assert.Contains(t, metrics, `value:"400"`)
}

func TestBuildHistogram(t *testing.T) {
	reg := prometheus.NewRegistry()
	server := gin.New()
	server.Use(newTestBuilder(reg).WithHistogram().Build())
	server.GET("/hello", func(c *gin.Context) { c.Status(http.StatusOK) })

	server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hello", nil))

	metrics := gatherMetricsText(t, reg)
	assert.Contains(t, metrics, "webook_http_requests_duration_seconds")
	assert.Contains(t, metrics, "type:HISTOGRAM")
	assert.Contains(t, metrics, "sample_count:1")
}

func TestBuildSummary(t *testing.T) {
	reg := prometheus.NewRegistry()
	server := gin.New()
	server.Use(newTestBuilder(reg).WithSummary().Build())
	server.GET("/hello", func(c *gin.Context) { c.Status(http.StatusOK) })

	server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hello", nil))

	metrics := gatherMetricsText(t, reg)
	assert.Contains(t, metrics, "webook_http_requests_duration_seconds_summary")
	assert.Contains(t, metrics, "type:SUMMARY")
	assert.Contains(t, metrics, "sample_count:1")
}

func TestBuildInFlight(t *testing.T) {
	reg := prometheus.NewRegistry()
	server := gin.New()
	server.Use(newTestBuilder(reg).WithInFlight().Build())
	server.GET("/hello", func(c *gin.Context) { c.Status(http.StatusOK) })

	server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hello", nil))

	metrics := gatherMetricsText(t, reg)
	assert.Contains(t, metrics, "webook_http_requests_in_flight")
}

func TestBuild_OnlyRegistersEnabled(t *testing.T) {
	reg := prometheus.NewRegistry()
	server := gin.New()
	server.Use(newTestBuilder(reg).WithCounter().Build())
	server.GET("/hello", func(c *gin.Context) { c.Status(http.StatusOK) })

	server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hello", nil))

	names := gatherNames(t, reg)
	assert.Contains(t, names, "webook_http_requests_total")
	assert.NotContains(t, names, "webook_http_requests_duration_seconds")
	assert.NotContains(t, names, "webook_http_requests_duration_seconds_summary")
	assert.NotContains(t, names, "webook_http_requests_in_flight")
}

func TestBuild_NoneEnabled(t *testing.T) {
	reg := prometheus.NewRegistry()
	server := gin.New()
	server.Use(newTestBuilder(reg).Build())
	server.GET("/hello", func(c *gin.Context) { c.Status(http.StatusOK) })

	server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hello", nil))

	mfs, err := reg.Gather()
	assert.NoError(t, err)
	assert.Empty(t, mfs)
}

func gatherMetricsText(t *testing.T, reg *prometheus.Registry) string {
	mfs, err := reg.Gather()
	assert.NoError(t, err)
	var sb strings.Builder
	for _, mf := range mfs {
		sb.WriteString(mf.String())
		sb.WriteString("\n")
	}
	return sb.String()
}

func gatherNames(t *testing.T, reg *prometheus.Registry) []string {
	mfs, err := reg.Gather()
	assert.NoError(t, err)
	names := make([]string, 0, len(mfs))
	for _, mf := range mfs {
		names = append(names, mf.GetName())
	}
	return names
}
