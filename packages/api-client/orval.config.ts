import { defineConfig } from 'orval';

// Gera hooks TanStack Query tipados a partir do OpenAPI (fonte da verdade).
// Rode: `npm --prefix packages/api-client run generate` (após `npm install` aqui).
export default defineConfig({
  renoviCare: {
    input: '../contracts/openapi.yaml',
    output: {
      mode: 'tags-split',
      target: './src/generated',
      client: 'react-query',
      baseUrl: '/api/v1',
      clean: true,
    },
  },
});
