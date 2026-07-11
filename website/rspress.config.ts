import { defineConfig } from 'rspress/config';

const isGHPages = process.env.GH_PAGES === 'true';

export default defineConfig({
  title: 'HnsX — Harness as a Service',
  description:
    'HnsX 让企业安全、可控、可评估地驾驭 Claude Code、Codex、OpenAI 等最强 Agent。',
  lang: 'zh-CN',
  base: isGHPages ? '/HnsX/' : '/',
  outDir: 'dist',
  route: {
    cleanUrls: true,
  },
  globalStyles: new URL('./styles/index.css', import.meta.url).pathname,
  head: [
    ['meta', { property: 'og:title', content: 'HnsX — Harness as a Service' }],
    [
      'meta',
      {
        property: 'og:description',
        content:
          'HnsX 让企业安全、可控、可评估地驾驭 Claude Code、Codex、OpenAI 等最强 Agent。',
      },
    ],
    ['meta', { property: 'og:type', content: 'website' }],
  ],
  themeConfig: {
    nav: [
      { text: '首页', link: '/', position: 'left' },
      { text: '博客', link: '/blog/', position: 'left' },
      { text: '文档', link: '/design/vision', position: 'left' },
      { text: 'API', link: '/design/server-design/api-design', position: 'left' },
      {
        text: 'GitHub',
        link: 'https://github.com/narcilee7/HnsX',
        position: 'right',
      },
    ],
    socialLinks: [
      {
        icon: 'github',
        mode: 'link',
        content: 'https://github.com/narcilee7/HnsX',
      },
    ],
  },
});
