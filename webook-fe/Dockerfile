# syntax=docker/dockerfile:1.7
# 企业级 Next.js 镜像：多阶段 + standalone + 非 root + healthcheck

# 用 20-alpine 滚动 tag（Node 20 LTS，至 2026-04 维护）：国内镜像源缓存命中率最高
# 想要可复现构建，CI 里传 --build-arg NODE_VERSION=20.x.y-alpine 显式钉死
ARG NODE_VERSION=20-alpine

# ── deps：仅装依赖，最大化复用缓存 ───────────────────────────
FROM node:${NODE_VERSION} AS deps
WORKDIR /app
RUN apk add --no-cache libc6-compat
COPY package.json package-lock.json ./
# npm 镜像源按需切换：
#   CI（海外 runner）走默认官方源
#   本地/国内虚拟机可传 --build-arg NPM_REGISTRY=https://registry.npmmirror.com
ARG NPM_REGISTRY=https://registry.npmjs.org
RUN npm config set registry "$NPM_REGISTRY" \
 && npm ci --no-audit --no-fund

# ── builder：build standalone 产物 ──────────────────────────
FROM node:${NODE_VERSION} AS builder
WORKDIR /app
ENV NEXT_TELEMETRY_DISABLED=1
ARG NEXT_PUBLIC_API_BASE_URL=/api
ENV NEXT_PUBLIC_API_BASE_URL=${NEXT_PUBLIC_API_BASE_URL}
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

# ── runner：仅运行所需文件，非 root ──────────────────────────
FROM node:${NODE_VERSION} AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
ENV HOSTNAME=0.0.0.0

RUN addgroup --system --gid 1001 nodejs \
 && adduser  --system --uid 1001 nextjs

# standalone 包含最小化 node_modules + server.js
COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static    ./.next/static
COPY --from=builder --chown=nextjs:nodejs /app/public          ./public

USER nextjs
EXPOSE 3000

# 健康检查：next standalone 没有 /healthz，用首页 200 判活
HEALTHCHECK --interval=10s --timeout=5s --start-period=20s --retries=5 \
  CMD wget -qO- http://127.0.0.1:3000/ >/dev/null || exit 1

# OCI 标签（企业仓库会读取）
LABEL org.opencontainers.image.title="webook-fe" \
      org.opencontainers.image.description="Webook Next.js frontend (standalone)" \
      org.opencontainers.image.source="https://github.com/webook" \
      org.opencontainers.image.licenses="MIT"

CMD ["node", "server.js"]
