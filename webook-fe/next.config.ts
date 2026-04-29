import type { NextConfig } from 'next';

// dev rewrites：浏览器统一调 /api/*，dev server 按前缀分发到不同后端服务
// 与生产环境 nginx 路由规则保持完全一致（参考 deploy/nginx/conf.d/default.conf）
// 加新服务时只需在这里追加一条 rewrites 规则，前端代码零改动
const CORE_BACKEND =
  process.env.NEXT_PUBLIC_DEV_CORE_URL || 'http://localhost:8089';
const CHAT_BACKEND =
  process.env.NEXT_PUBLIC_DEV_CHAT_URL || 'http://localhost:8189';

const nextConfig: NextConfig = {
  // 企业级部署：standalone 输出，Docker 镜像只装最小 node_modules
  output: 'standalone',
  poweredByHeader: false,
  compress: true,
  productionBrowserSourceMaps: false,

  // 仅在 dev 模式生效；生产构建走 nginx，rewrites 不参与
  async rewrites() {
    return [
      // /api/chat/* → webook-chat（剥 /api 前缀）
      {
        source: '/api/chat/:path*',
        destination: `${CHAT_BACKEND}/chat/:path*`,
      },
      // /api/* → webook-core（剥 /api 前缀）
      {
        source: '/api/:path*',
        destination: `${CORE_BACKEND}/:path*`,
      },
    ];
  },
};

export default nextConfig;
