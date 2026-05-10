// @ts-check
import withNuxt from "./.nuxt/eslint.config.mjs";
import prettier from "eslint-plugin-prettier";
import prettierConfig from "eslint-config-prettier";

export default withNuxt(
    {
        files: ["**/*.ts", "**/*.tsx", "**/*.vue"],
        languageOptions: {
            parserOptions: {
                projectService: {
                    allowDefaultProject: ["composables/*.ts", "utils/*.ts"],
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
