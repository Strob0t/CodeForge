import tseslint from "typescript-eslint";
import solid from "eslint-plugin-solid/configs/typescript";
import simpleImportSort from "eslint-plugin-simple-import-sort";

export default tseslint.config(
  ...tseslint.configs.strict,
  ...tseslint.configs.stylistic,
  solid,
  {
    plugins: {
      "simple-import-sort": simpleImportSort,
    },
    languageOptions: {
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      "simple-import-sort/imports": "error",
      "simple-import-sort/exports": "error",
    },
  },
  {
    ignores: ["dist/", "node_modules/", "eslint.config.js", "vite.config.ts"],
  },
);
