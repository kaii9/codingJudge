import { defineConfig, globalIgnores } from "eslint/config";
import react from "eslint-plugin-react";
import hooks from "eslint-plugin-react-hooks";
import a11y from "eslint-plugin-jsx-a11y";
import imports from "eslint-plugin-import";
import ts from "typescript-eslint";
import globals from "globals";

// App Router guardrails not supplied by Biome's Next domain. Use core AST
// selectors, not a fork of the Next plugin or its vulnerable glob dependency.
const internalAnchor = "JSXOpeningElement[name.name='a']:not(:has(JSXAttribute[name.name='download']))";
// Allow schemes, protocol-relative URLs, fragments and same-page queries.
const internalPath = "/^(?!([a-z][a-z0-9+.-]*:|\\/\\/|#|\\?)).+/i";

const eslintConfig = defineConfig([
  {
    files: ["**/*.{js,jsx,mjs,ts,tsx,mts,cts}"],
    plugins: { react, "react-hooks": hooks, "jsx-a11y": a11y, import: imports },
    languageOptions: {
      parserOptions: { ecmaFeatures: { jsx: true } },
      globals: { ...globals.browser, ...globals.node },
    },
    settings: { react: { version: "detect" } },
    rules: {
      ...react.configs.recommended.rules,
      ...hooks.configs.recommended.rules,
      "react/no-unknown-property": "off",
      "react/react-in-jsx-scope": "off",
      "react/prop-types": "off",
      "react/jsx-no-target-blank": "off",
      "import/no-anonymous-default-export": "warn",
      "jsx-a11y/alt-text": ["warn", { elements: ["img"], img: ["Image"] }],
      "jsx-a11y/aria-props": "warn",
      "jsx-a11y/aria-proptypes": "warn",
      "jsx-a11y/aria-unsupported-elements": "warn",
      "jsx-a11y/role-has-required-aria-props": "warn",
      "jsx-a11y/role-supports-aria-props": "warn",
      "no-restricted-imports": ["error", {
        paths: [
          { name: "next/document", message: "App Router uses app/layout.tsx, not next/document." },
          { name: "next/head", message: "App Router uses the Metadata API, not next/head." },
        ],
      }],
      "no-restricted-syntax": ["error",
        { selector: "VariableDeclarator[id.name='module']", message: "Do not declare Next's reserved module variable." },
        { selector: `${internalAnchor}:has(JSXAttribute[name.name='href'] > Literal[value=${internalPath}])`, message: "Use next/link for internal navigation (download links are allowed)." },
        { selector: `${internalAnchor}:has(JSXAttribute[name.name='href'] > JSXExpressionContainer > Literal[value=${internalPath}])`, message: "Use next/link for internal navigation (download links are allowed)." },
        { selector: `${internalAnchor}:has(JSXAttribute[name.name='href'] > JSXExpressionContainer > TemplateLiteral > TemplateElement:first-child[value.cooked=${internalPath}])`, message: "Use next/link for internal navigation (download links are allowed)." },
      ],
    },
  },
  ...ts.configs.recommended,
  { rules: { "@typescript-eslint/no-unused-vars": "warn", "@typescript-eslint/no-unused-expressions": "warn" } },
  globalIgnores([
    ".next/**",
    "out/**",
    "build/**",
    "public/monaco/**",
    "next-env.d.ts",
  ]),
]);

export default eslintConfig;
