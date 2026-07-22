#!/bin/bash
# ===================================================
# 文件：deploy/deploy.sh（也是服务器 ~/webook-deploy/deploy.sh）
# 用法：
#   ./deploy.sh <local|dev|staging|prod> [up] [service...]  # 起（指定服务含依赖链；local 先 build；自动 stop 别的 env）
#   ./deploy.sh <env> down                    # 停整个 env（volume 保留，不接服务名，按服务用 stop/rm）
#   ./deploy.sh <env> stop [service...]       # 停全部或指定服务（容器保留）
#   ./deploy.sh <env> rm <service...>         # 停并移除指定服务容器（volume 保留）
#   ./deploy.sh <env> nuke                    # 停 + 清 volume（prod 需确认）
#   ./deploy.sh <env> logs [service...]       # 日志（默认 webook-core）
#   ./deploy.sh <env> status [service...]     # docker compose ps
#   ./deploy.sh <env> pull [service...]       # 拉镜像
#   ./deploy.sh <env> build [service...]      # 构建镜像（local 用）
#   ./deploy.sh <env> restart <service...>    # 重启指定服务
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
SVCS=("${@:3}")   # 第 3 位起都是服务名，可传多个

# list 不需要 env
if [ "$ENV" = "list" ]; then
  docker ps -a --filter "label=com.docker.compose.project" \
    --format "table {{.Names}}\t{{.Label \"com.docker.compose.project\"}}\t{{.Status}}" \
    | grep webook- || echo "（无 webook-* project 容器）"
  exit 0
fi

if [[ ! "$ENV" =~ ^(local|dev|staging|prod)$ ]]; then
  echo "用法：./deploy.sh <local|dev|staging|prod> [up|down|stop|rm|nuke|logs|status|pull|build|restart] [service...(支持正则)]"
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
if ! [[ "$ACTION" =~ ^(up|down|stop|rm|nuke|logs|status|ps|pull|restart|build)$ ]]; then
  echo "❌ 未知操作：$ACTION"
  echo "   支持：up / down / stop / rm / nuke / logs / status / pull / build / restart"
  exit 1
fi

# service 参数支持正则/部分匹配：每个 pattern 用 grep -E 匹配 compose 定义的服务名并展开为具体服务名。
# 例：./deploy.sh dev up '^webook-(core|chat)$'  或  ./deploy.sh dev restart 'webook-c'
# 注：只匹配当前 profile 可见的服务（要匹配 elk 服务需 COMPOSE_PROFILES=elk）。
if [ ${#SVCS[@]} -gt 0 ]; then
  _all=$($COMPOSE config --services 2>/dev/null)
  _matched=""
  for _pat in "${SVCS[@]}"; do
    _hit=$(printf '%s\n' "$_all" | grep -E "$_pat" || true)
    [ -z "$_hit" ] && { echo "❌ 无服务名匹配：$_pat"; echo "   当前可选：$(printf '%s ' $_all)"; exit 1; }
    _matched="$_matched"$'\n'"$_hit"
  done
  SVCS=($(printf '%s\n' "$_matched" | grep -v '^$' | sort -u))
  echo "🔎 服务匹配展开 → ${SVCS[*]}"
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
    echo "📦 启动 $ENV${SVCS[*]:+ / ${SVCS[*]}} (project=$PROJECT, APP_ENV=$(grep ^APP_ENV= $ENV_FILE | cut -d= -f2))"
    # local 必须 build；非 local 依赖 compose 里 pull_policy
    # （镜像缺失才拉）。想显式刷新走 ./deploy.sh <env> pull
    if [ "$ENV" = "local" ]; then
      $COMPOSE build "${SVCS[@]}"
    fi
    # 指定服务时 compose 会把各自 depends_on 链一并起
    $COMPOSE up -d "${SVCS[@]}"
    sleep 3
    $COMPOSE ps "${SVCS[@]}"
    ;;
  down)
    [ -n "$3" ] && { echo "❌ down 停整个 env，不接服务名；停单个用 stop，移除用 rm"; exit 1; }
    $COMPOSE down
    echo "✅ $ENV 已停止（volume 保留在 ${PROJECT}_*）"
    ;;
  stop)
    $COMPOSE stop "${SVCS[@]}"
    echo "✅ 已停止 ${SVCS[*]:-$ENV 全部服务}（容器保留，up 即恢复）"
    ;;
  rm)
    [ ${#SVCS[@]} -eq 0 ] && { echo "用法：./deploy.sh $ENV rm <service...>"; exit 1; }
    $COMPOSE rm -sf "${SVCS[@]}"
    echo "✅ ${SVCS[*]} 容器已移除（volume 保留，up 会重建）"
    ;;
  nuke)
    [ -n "$3" ] && { echo "❌ nuke 清整个 env 的 volume，不接服务名"; exit 1; }
    if [ "$ENV" = "prod" ]; then
      read -p "⚠️  即将删除 PROD 所有数据 volume，输入 yes 确认：" ans
      [ "$ans" != "yes" ] && { echo "已取消"; exit 1; }
    fi
    read -p "确认删除 $ENV 所有数据？(yes) " ans
    [ "$ans" = "yes" ] && $COMPOSE down -v && echo "✅ $ENV 全部清理"
    ;;
  logs)
    [ ${#SVCS[@]} -eq 0 ] && SVCS=(webook-core)
    $COMPOSE logs -f --tail=100 "${SVCS[@]}"
    ;;
  status|ps)
    $COMPOSE ps "${SVCS[@]}"
    ;;
  pull)
    $COMPOSE pull "${SVCS[@]}"
    ;;
  build)
    $COMPOSE build "${SVCS[@]}"
    ;;
  restart)
    [ ${#SVCS[@]} -eq 0 ] && { echo "用法：./deploy.sh $ENV restart <service...>"; exit 1; }
    $COMPOSE restart "${SVCS[@]}"
    ;;
esac
