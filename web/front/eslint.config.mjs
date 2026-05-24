// @ts-check
import withNuxt from "./.nuxt/eslint.config.mjs";
import prettier from "eslint-plugin-prettier";
import prettierConfig from "eslint-config-prettier";

export default withNuxt(
    {
        files: ["**/*.ts", "**/*.tsx", "**/*.vue"],
        languageOptions: {
            parserOptions: {
                // composables/, utils/, test/setup.ts and vitest.config.ts live
                // at the project root, outside the Nuxt srcDir (app/), so no
                // generated tsconfig includes them — typed linting falls back to
                // the "default project". Its default cap is 8 files and we have
                // more, hence the explicit higher limit (the slowdown is
                // negligible at this scale). The config/setup files are listed so
                // they aren't flagged as "not found by the project service".
                // test/*.ts is scoped to the top level so it never overlaps with
                // test/nuxt/** (which a generated tsconfig already includes).
                projectService: {
                    allowDefaultProject: [
                        "composables/*.ts",
                        "utils/*.ts",
                        "test/*.ts",
                        "vitest.config.ts",
                    ],
                    maximumDefaultProjectFileMatchCount_THIS_WILL_SLOW_DOWN_LINTING: 50,
                },
                tsconfigRootDir: import.meta.dirname,
            },
        },
        plugins: { prettier },
        rules: {
            "@typescript-eslint/consistent-type-imports": [
                "error",
                { prefer: "type-imports", fixStyle: "separate-type-imports" },
            ],
            "vue/multi-word-component-names": "off",
            "prettier/prettier": "error",
        },
    },
    prettierConfig,
    {
        files: ["app/api/generated/**/*.ts"],
        rules: {
            "@typescript-eslint/no-unused-vars": "off",
            "@typescript-eslint/no-explicit-any": "off",
            "@typescript-eslint/no-empty-object-type": "off",
            "@typescript-eslint/no-redundant-type-constituents": "off",
            "@typescript-eslint/no-unsafe-function-type": "off",
            "@typescript-eslint/ban-ts-comment": "off",
            "@typescript-eslint/no-invalid-void-type": "off",
            "vue/multi-word-component-names": "off",
            "prettier/prettier": "off",
        },
    },
    {
        ignores: [".nuxt/**", ".output/**", "dist/**", "node_modules/**", "eslint.config.mjs"],
    },
);
