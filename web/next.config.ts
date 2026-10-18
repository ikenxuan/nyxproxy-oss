import type { NextConfig } from 'next';

const isExport = process.env.NEXT_EXPORT === '1';

const nextConfig: NextConfig = {
  output: isExport ? 'export' : undefined,
  trailingSlash: isExport,
  images: { unoptimized: true },
  // `next dev` otherwise writes AGENTS.md on every start.
  agentRules: false,
  async rewrites() {
    if (isExport) {
      return [];
    }
    const api = process.env.NYXPROXY_API ?? 'http://127.0.0.1:18081';
    // This rewrite changes the Host header to the backend address while the
    // browser still sends an Origin of http://localhost:3000, so the backend
    // rejects login/config mutations unless local development sets
    // `web.trust_proxy_headers: true`.
    return [{ source: '/api/:path*', destination: `${api}/api/:path*` }];
  },
};

export default nextConfig;
