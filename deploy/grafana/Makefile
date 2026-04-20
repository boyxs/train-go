# Grafana provisioning 运维命令
# 用法：cd deploy/grafana && make ENV=<local|dev|staging|prod> <target>
#   默认 ENV=dev。restart / test-email-real 依赖 ENV 来定位对应 project 的容器。
# 环境变量可覆盖：
#   ENV=dev                          # 目标部署环境
#   GRAFANA_HOST=192.168.150.101:3001
#   GRAFANA_USER=admin
#   GRAFANA_PASS=admin

ENV          ?= dev
GRAFANA_HOST ?= 192.168.150.101:3001
GRAFANA_USER ?= admin
GRAFANA_PASS ?= admin
GRAFANA_URL  := http://$(GRAFANA_HOST)
AUTH         := -u $(GRAFANA_USER):$(GRAFANA_PASS)

# docker compose 命令带上 project 名和对应 env 文件，确保操作到正确 env 的容器
COMPOSE      := docker compose -p webook-$(ENV) --env-file ../.env.$(ENV) -f ../docker-compose.yaml

.PHONY: help reload reload-alerting reload-datasources reload-dashboards reload-all test-email test-email-ui test-email-real restart

help:
	@echo "make <target>"
	@echo ""
	@echo "  reload             reload 告警配置（最常用）"
	@echo "  reload-alerting    reload alerting（rules + contactpoints + policies）"
	@echo "  reload-datasources reload 数据源"
	@echo "  reload-dashboards  reload dashboards（通常不用，自动扫 30s）"
	@echo "  reload-all         一次 reload 所有"
	@echo "  test-email         ≡ test-email-ui（推荐）"
	@echo "  test-email-ui      打印 UI Test 按钮路径，最快验证 contact point"
	@echo "  test-email-real    停 webook 90s 触发真告警，端到端验证（有服务中断）"
	@echo "  restart            重启 Grafana 容器（改 contactpoints 后必用）"
	@echo ""
	@echo "环境变量覆盖："
	@echo "  GRAFANA_HOST=$(GRAFANA_HOST)"
	@echo "  GRAFANA_USER=$(GRAFANA_USER)"

# 最常用：改了告警配置后 reload
reload: reload-alerting

reload-alerting:
	@echo "📢 reload alerting..."
	@curl -sS -X POST $(AUTH) $(GRAFANA_URL)/api/admin/provisioning/alerting/reload
	@echo ""
	@echo "✅ alerting reloaded"

reload-datasources:
	@echo "📊 reload datasources..."
	@curl -sS -X POST $(AUTH) $(GRAFANA_URL)/api/admin/provisioning/datasources/reload
	@echo ""
	@echo "✅ datasources reloaded"

reload-dashboards:
	@echo "📈 reload dashboards..."
	@curl -sS -X POST $(AUTH) $(GRAFANA_URL)/api/admin/provisioning/dashboards/reload
	@echo ""
	@echo "✅ dashboards reloaded"

reload-all: reload-alerting reload-datasources reload-dashboards

# test-email 有两种模式：
#
# 1. test-email-ui (推荐首次)：打印 UI 测试路径。provisioned 的 contact point 虽然
#    整体只读，但"Test"按钮仍然可用，能立刻看到邮件。
#
# 2. test-email-real：停掉 webook 1 分多钟，触发真告警 webook-up → 走完整 policy +
#    SMTP 链路。端到端最可靠，但会临时断服务。适合 staging / 非业务时段。
#
# ❌ 不用 POST /api/alertmanager/grafana/api/v2/alerts：Grafana 11 该 endpoint
#    payload schema 与标准 AM 不完全兼容，返回 400 bad request data（已验证）。
#    也不用 /api/.../receivers/test：需要传完整 receiver config，CLI 不方便。

test-email: test-email-ui

test-email-ui:
	@echo "📧 前往 Grafana UI 手动点 Test 按钮："
	@echo ""
	@echo "  http://$(GRAFANA_HOST)/alerting/notifications"
	@echo ""
	@echo "  → 找到 webook-email → 右侧三点菜单 → Test"
	@echo "  点后立即收到一封模板渲染邮件（含示例 alert 数据）"

test-email-real:
	@echo "🚨 即将停 webook 触发真告警（webook-up 规则，for=1m）"
	@echo "   期间 webook 服务中断约 90 秒。生产环境慎用！"
	@read -p "确认继续? [y/N] " ans; [ "$$ans" = "y" ] || [ "$$ans" = "Y" ] || { echo "已取消"; exit 0; }
	@$(COMPOSE) stop webook
	@echo "⏳ 等 90s 让告警 firing + policy group_wait + SMTP 发送..."
	@sleep 90
	@$(COMPOSE) up -d webook
	@echo "✅ webook 已恢复。邮箱应收到告警 + 稍后的 resolved 通知"
	@echo "   Alerting 状态：http://$(GRAFANA_HOST)/alerting/list"

# 兜底：API reload 失败 / 想重建容器时用
restart:
	@echo "🔄 重启 Grafana 容器..."
	@$(COMPOSE) restart grafana
	@echo "✅ 重启完成"
