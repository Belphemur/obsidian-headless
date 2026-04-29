import { defineUserConfig } from 'vuepress'
import { defaultTheme } from '@vuepress/theme-default'
import { viteBundler } from '@vuepress/bundler-vite'
import { searchPlugin } from '@vuepress/plugin-search'
// import mermaidPkg from '@renovamen/vuepress-plugin-mermaid'
// const { mermaidPlugin } = mermaidPkg

export default defineUserConfig({
  bundler: viteBundler(),
  title: 'Obsidian Headless',
  description: 'Command-line client for Obsidian Sync and Obsidian Publish',
  base: '/obsidian-headless/',
  lang: 'en-US',

  theme: defaultTheme({
    logo: '/logo.png',
    logoDark: '/logo.png',

    navbar: [
      { text: 'Home', link: '/' },
      { text: 'Installation', link: '/installation' },
      { text: 'Usage', link: '/usage' },
      { text: 'Architecture', link: '/architecture/' },
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
  }),

  plugins: [
    searchPlugin(),
    // prismjsPlugin() is included by default in @vuepress/theme-default
    // mermaidPlugin(), // commented out due to VuePress 2 RC compatibility
  ],
})
