import { defineUserConfig } from 'vuepress'
import { defaultTheme } from '@vuepress/theme-default'
import { viteBundler } from '@vuepress/bundler-vite'
import { searchPlugin } from '@vuepress/plugin-search'
import { iconPlugin } from '@vuepress/plugin-icon'
import { markdownChartPlugin } from '@vuepress/plugin-markdown-chart'
import { path } from '@vuepress/utils'

export default defineUserConfig({
  bundler: viteBundler({
    viteOptions: {
      resolve: {
        alias: {
          '@theme/VPHomeFeatures.vue': path.resolve(__dirname, 'components/VPHomeFeatures.vue'),
        },
      },
    },
  }),
  title: 'Obsidian Headless Go',
  description: 'Headless Go CLI client for Obsidian Sync and Obsidian Publish — run sync and publish from the command line, in Docker, or on servers',
  base: '/obsidian-headless/',
  lang: 'en-US',

  head: [
    ['link', { rel: 'icon', href: '/logo.svg', type: 'image/svg+xml' }],
    ['meta', { name: 'theme-color', content: '#A88BFA' }],
    ['meta', { property: 'og:title', content: 'Obsidian Headless Go' }],
    ['meta', { property: 'og:description', content: 'Headless Go CLI client for Obsidian Sync and Obsidian Publish' }],
  ],

  theme: defaultTheme({
    logo: '/logo.svg',
    logoDark: '/logo.svg',

    repo: 'Belphemur/obsidian-headless',
    docsRepo: 'Belphemur/obsidian-headless',
    docsDir: 'website/src',
    docsBranch: 'main',
    editLink: true,
    editLinkText: 'Edit this page on GitHub',
    lastUpdated: true,

    navbar: [
      { text: 'Home', link: '/' },
      { text: 'Getting Started', link: '/getting-started' },
      {
        text: 'Installation',
        children: [
          { text: 'Overview', link: '/installation/' },
          { text: 'macOS', link: '/installation/macos' },
          { text: 'Linux', link: '/installation/linux' },
          { text: 'Windows', link: '/installation/windows' },
          { text: 'Docker', link: '/installation/docker' },
          { text: 'From Source', link: '/installation/from-source' },
        ],
      },
      {
        text: 'Usage',
        children: [
          { text: 'Overview', link: '/usage/' },
          { text: 'Authentication', link: '/usage/authentication' },
          { text: 'Sync', link: '/usage/sync' },
          { text: 'Publish', link: '/usage/publish' },
          { text: 'Configuration', link: '/usage/configuration' },
        ],
      },
      {
        text: 'Architecture',
        children: [
          { text: 'Overview', link: '/architecture/' },
          { text: 'Sync Protocol', link: '/architecture/sync-protocol' },
          { text: 'Encryption', link: '/architecture/encryption' },
          { text: 'REST API', link: '/architecture/rest-api' },
        ],
      },
    ],

    sidebar: {
      '/': ['', '/getting-started'],
      '/installation/': [
        '',
        'macos',
        'linux',
        'windows',
        'docker',
        'from-source',
      ],
      '/usage/': [
        '',
        'authentication',
        'sync',
        'publish',
        'configuration',
      ],
      '/architecture/': [
        '',
        'sync-protocol',
        'encryption',
        'rest-api',
      ],
    },

    sidebarDepth: 3,

    themePlugins: {
      git: true,
    },
  }),

  plugins: [
    searchPlugin({
      maxSuggestions: 10,
      locales: {
        '/': {
          placeholder: 'Search',
        },
      },
    }),
    iconPlugin({
      // Font Awesome 6 free icons
      assets: 'https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css',
      type: 'fontawesome',
      component: 'Icon',
    }),
    markdownChartPlugin({
      // Enable mermaid
      mermaid: true,
    }),
  ],
})
