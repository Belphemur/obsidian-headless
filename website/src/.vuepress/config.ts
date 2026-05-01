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
  title: 'Headless Go client for Obsidian',
  description: 'Headless Go CLI client for Obsidian Sync and Obsidian Publish — sync and publish your vaults from the command line, in Docker, or on servers',
  base: '/obsidian-headless/',
  lang: 'en-US',

  head: [
    // Standard favicons
    ['link', { rel: 'icon', href: '/favicon.ico', sizes: '48x48' }],
    ['link', { rel: 'icon', href: '/favicon-32x32.png', type: 'image/png', sizes: '32x32' }],
    ['link', { rel: 'icon', href: '/favicon-16x16.png', type: 'image/png', sizes: '16x16' }],
    // Apple Touch Icon
    ['link', { rel: 'apple-touch-icon', href: '/apple-touch-icon.png' }],
    // Android Chrome
    ['link', { rel: 'icon', href: '/android-chrome-192x192.png', type: 'image/png', sizes: '192x192' }],
    ['link', { rel: 'icon', href: '/android-chrome-512x512.png', type: 'image/png', sizes: '512x512' }],
    // Web Manifest
    ['link', { rel: 'manifest', href: '/site.webmanifest' }],
    // Meta
    ['meta', { name: 'theme-color', content: '#A88BFA' }],
    ['meta', { property: 'og:title', content: 'Headless Go client for Obsidian' }],
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
          { text: 'Circuit Breaker', link: '/architecture/circuit-breaker' },
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
        'circuit-breaker',
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
