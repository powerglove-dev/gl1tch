import { defineConfig } from 'astro/config';

export default defineConfig({
  output: 'static',
  base: '/',
  trailingSlash: 'never',
  markdown: {
    shikiConfig: {
      theme: 'dracula',
    },
  },
});
