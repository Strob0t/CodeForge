import tseslint from "typescript-eslint";
import solid from "eslint-plugin-solid/configs/typescript";
import simpleImportSort from "eslint-plugin-simple-import-sort";
import jsxA11y from "eslint-plugin-jsx-a11y";

export default tseslint.config(
  ...tseslint.configs.strict,
  ...tseslint.configs.stylistic,
  solid,
  jsxA11y.flatConfigs.recommended,
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
      "jsx-a11y/alt-text": "error",
      "jsx-a11y/aria-props": "error",
      "jsx-a11y/aria-proptypes": "error",
      "jsx-a11y/role-has-required-aria-props": "error",
      "jsx-a11y/click-events-have-key-events": "warn",
      "jsx-a11y/no-static-element-interactions": "warn",
      "jsx-a11y/no-noninteractive-element-interactions": "warn",
      "jsx-a11y/no-noninteractive-tabindex": "warn",
      "jsx-a11y/label-has-associated-control": "warn",
    },
  },
  {
    ignores: [
      "dist/",
      "node_modules/",
      "eslint.config.js",
      "vite.config.ts",
      "e2e/",
      "playwright.config.ts",
      "playwright.*.config.ts",
      "scripts/",
    ],
  },
);
