import type { NextConfig } from 'next';

const nextConfig: NextConfig = {
  // 企业级部署：standalone 输出，Docker 镜像只装最小 node_modules
  output: 'standalone',
  poweredByHeader: false,
  compress: true,
  productionBrowserSourceMaps: false,
};

export default nextConfig;
