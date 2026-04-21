#!/bin/bash
# ===================================================
# 文件：deploy/deploy.sh（也是服务器 ~/webook-deploy/deploy.sh）
# 用法：
#   ./deploy.sh <local|dev|staging|prod>     # 起（会自动 stop 别的 env）
#   ./deploy.sh <env> down                    # 停（volume 保留）
#   ./deploy.sh <env> nuke                    # 停 + 清 volume（prod 需确认）
#   ./deploy.sh <env> logs [service]          # 日志（默认 webook）
#   ./deploy.sh <env> status                  # docker compose ps
#   ./deploy.sh <env> pull                    # 只拉镜像（local 模式是 build）
#   ./deploy.sh <env> restart <service>       # 重启某服务
#   ./deploy.sh list                          # 看所有 env 残留
# 可选 flag：
#   --ghcr <host>   覆盖 .env.<env> 的 GHCR_REGISTRY（仅本次生效，不改文件）
#                   例：./deploy.sh prod --ghcr ghcr.nju.edu.cn
# 环境隔离：project 名 webook-<env>，volume 按 env 独立保留
# 同时只跑一套：container_name 全局唯一，强制切换前停旧的
# local vs dev/staging/prod：
#   local  → 本地 build + 暴露宿主端口（override 文件 docker-compose.local.yaml）
#   其他   → 从 ghcr pull
# 启用 LLM：COMPOSE_PROFILES=llm ./deploy.sh <env>
# ===================================================
set -e

cd "$(dirname "$0")"

# ── 预解析 --ghcr 可选 flag（可出现在任意位置，不打乱位置参数）────────
GHCR_OVERRIDE=""
_args=()
while [ $# -gt 0 ]; do
  case "$1" in
    --ghcr)
      [ -z "$2" ] && { echo "❌ --ghcr 需要参数，如 --ghcr ghcr.nju.edu.cn"; exit 1; }
      GHCR_OVERRIDE="$2"
      shift 2
      ;;
    --ghcr=*)
      GHCR_OVERRIDE="${1#--ghcr=}"
      [ -z "$GHCR_OVERRIDE" ] && { echo "❌ --ghcr= 需要非空值"; exit 1; }
      shift
      ;;
    *)
      _args+=("$1")
      shift
      ;;
  esac
done
set -- "${_args[@]}"
# 去尾斜杠规范化（支持 ghcr.io// 这种多余写法）
while [[ "$GHCR_OVERRIDE" == */ ]]; do GHCR_OVERRIDE="${GHCR_OVERRIDE%/}"; done

# 导出到子进程：docker compose 的 shell env 会覆盖 --env-file 同名项
if [ -n "$GHCR_OVERRIDE" ]; then
  export GHCR_REGISTRY="$GHCR_OVERRIDE"
  echo "ℹ️  ghcr 源覆盖：$GHCR_OVERRIDE"
fi

ENV=$1
ACTION=${2:-up}

# list 不需要 env
if [ "$ENV" = "list" ]; then
  docker ps -a --filter "label=com.docker.compose.project" \
    --format "table {{.Names}}\t{{.Label \"com.docker.compose.project\"}}\t{{.Status}}" \
    | grep webook- || echo "（无 webook-* project 容器）"
  exit 0
fi

if [[ ! "$ENV" =~ ^(local|dev|staging|prod)$ ]]; then
  echo "用法：./deploy.sh <local|dev|staging|prod> [down|nuke|logs|status|pull|restart]"
  echo "     ./deploy.sh list"
  echo "     可选: --ghcr <host>   覆盖 ghcr 源（如 ghcr.nju.edu.cn），单次生效不改 env 文件"
  exit 1
fi

ENV_FILE=".env.$ENV"
[ ! -f "$ENV_FILE" ] && { echo "❌ $ENV_FILE 不存在（从 $ENV_FILE.example 复制并改）"; exit 1; }

PROJECT="webook-$ENV"

# local 模式叠加 override 文件
if [ "$ENV" = "local" ]; then
  COMPOSE="docker compose -p $PROJECT --env-file $ENV_FILE -f docker-compose.yaml -f docker-compose.local.yaml"
else
  COMPOSE="docker compose -p $PROJECT --env-file $ENV_FILE -f docker-compose.yaml"
fi

# 裸操作子命令拦截（防止 `./deploy.sh dev down` 被当 tag 或 unknown）
if ! [[ "$ACTION" =~ ^(up|down|nuke|logs|status|ps|pull|restart|build)$ ]]; then
  echo "❌ 未知操作：$ACTION"
  echo "   支持：up / down / nuke / logs / status / pull / build / restart"
  exit 1
fi

case "$ACTION" in
  up)
    # 切换 env 前把别的 env 停掉（container_name 全局唯一，同时跑会冲突）
    for other in local dev staging prod; do
      if [ "$other" != "$ENV" ] && [ -f ".env.$other" ]; then
        local_compose="docker compose -p webook-$other"
        [ "$other" = "local" ] && local_compose="$local_compose -f docker-compose.yaml -f docker-compose.local.yaml"
        $local_compose stop 2>/dev/null || true
      fi
    done
    echo "📦 启动 $ENV (project=$PROJECT, APP_ENV=$(grep ^APP_ENV= $ENV_FILE | cut -d= -f2))"
    # local 必须 build；非 local 依赖 compose 里 pull_policy
    # （镜像缺失才拉）。想显式刷新走 ./deploy.sh <env> pull
    if [ "$ENV" = "local" ]; then
      $COMPOSE build
    fi
    $COMPOSE up -d
    sleep 3
    $COMPOSE ps
    ;;
  down)
    $COMPOSE down
    echo "✅ $ENV 已停止（volume 保留在 ${PROJECT}_*）"
    ;;
  nuke)
    if [ "$ENV" = "prod" ]; then
      read -p "⚠️  即将删除 PROD 所有数据 volume，输入 yes 确认：" ans
      [ "$ans" != "yes" ] && { echo "已取消"; exit 1; }
    fi
    read -p "确认删除 $ENV 所有数据？(yes) " ans
    [ "$ans" = "yes" ] && $COMPOSE down -v && echo "✅ $ENV 全部清理"
    ;;
  logs)
    $COMPOSE logs -f --tail=100 "${3:-webook}"
    ;;
  status|ps)
    $COMPOSE ps
    ;;
  pull)
    $COMPOSE pull
    ;;
  build)
    $COMPOSE build
    ;;
  restart)
    [ -z "$3" ] && { echo "用法：./deploy.sh $ENV restart <service>"; exit 1; }
    $COMPOSE restart "$3"
    ;;
esac
