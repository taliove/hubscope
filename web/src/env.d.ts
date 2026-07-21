/// <reference types="vite/client" />

// Type shim so TypeScript understands single-file component imports.
declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}
