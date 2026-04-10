// tsdown.config.ts
import { defineConfig } from 'tsdown';

export default defineConfig([
  // ESM + CJS: Penpal external — host project provides it (deduplication)
  {
    entry: ['src/index.ts'],
    format: ['esm', 'cjs'],
    outDir: 'dist',
    target: 'es2020',
    platform: 'browser',
    dts: true,
    sourcemap: true,
    clean: true,
    treeshake: true,
    external: ['penpal'],
  },
  // UMD: bundle Penpal in — self-contained for CDN <script> usage
  {
    entry: ['src/index.ts'],
    format: ['umd'],
    outDir: 'dist',
    target: 'es2020',
    platform: 'browser',
    globalName: 'OpenBuroClient',
    minify: true,
    sourcemap: false,
    // penpal NOT in external → bundled into UMD
  },
]);
