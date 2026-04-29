import { defineUserConfig } from 'vuepress'
import { defaultTheme } from '@vuepress/theme-default'
import { viteBundler } from '@vuepress/bundler-vite'
import { searchPlugin } from '@vuepress/plugin-search'

export default defineUserConfig({
  bundler: viteBundler(),
  title: 'Obsidian Headless',
  description: 'Headless CLI client for Obsidian Sync and Obsidian Publish — run sync and publish from the command line, in Docker, or on servers',
  base: '/obsidian-headless/',
  lang: 'en-US',

  head: [
    ['link', { rel: 'icon', href: '/logo.svg', type: 'image/svg+xml' }],
    ['meta', { name: 'theme-color', content: '#A88BFA' }],
    ['meta', { property: 'og:title', content: 'Obsidian Headless' }],
    ['meta', { property: 'og:description', content: 'Headless CLI client for Obsidian Sync and Obsidian Publish' }],
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
      { text: 'Installation', link: '/installation' },
      { text: 'Usage', link: '/usage' },
      {
        text: 'Architecture',
        children: [
          { text: 'Overview', link: '/architecture/' },
          { text: 'Sync Protocol', link: '/architecture/sync-protocol' },
          { text: 'Encryption', link: '/architecture/encryption' },
          { text: 'REST API', link: '/architecture/rest-api' },
        ],
      },
      {
        text: 'GitHub',
        link: 'https://github.com/Belphemur/obsidian-headless',
      },
    ],

    sidebar: {
      '/': [''],
      '/installation': [''],
      '/usage': [''],
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
  ],
})
