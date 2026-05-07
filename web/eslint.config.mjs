import path from 'node:path';
import fs from 'node:fs';

import { includeIgnoreFile } from '@eslint/compat';
import js from '@eslint/js';
import { createTypeScriptImportResolver } from 'eslint-import-resolver-typescript';
import pluginVitest from '@vitest/eslint-plugin';
import skipFormatting from '@vue/eslint-config-prettier/skip-formatting';
import { defineConfigWithVueTs, vueTsConfigs } from '@vue/eslint-config-typescript';
import { defineConfig, globalIgnores } from 'eslint/config';
import { configs, plugins } from 'eslint-config-airbnb-extended';
import { rules as prettierConfigRules } from 'eslint-config-prettier';
import { createNodeResolver } from 'eslint-plugin-import-x';
import prettierPlugin from 'eslint-plugin-prettier';
import simpleImportSort from 'eslint-plugin-simple-import-sort';
import pluginVue from 'eslint-plugin-vue';

const autoImportConfig = JSON.parse(
  fs.readFileSync(new URL('./.eslintrc-auto-import.json', import.meta.url)),
);

const gitignorePath = path.resolve('.', '.gitignore');
const tsResolverProjects = [
  './tsconfig.app.json',
  './tsconfig.node.json',
  './tsconfig.vitest.json',
];

const jsConfig = defineConfig([
  // ESLint recommended config
  {
    name: 'js/config',
    ...js.configs.recommended,
  },
  // Stylistic plugin
  plugins.stylistic,
  // Import X plugin
  plugins.importX,
  // Airbnb base recommended config
  ...configs.base.recommended,
]);

const nodeConfig = defineConfig([
  // Node plugin
  plugins.node,
  // Airbnb Node recommended config
  ...configs.node.recommended,
]);

const typescriptConfig = defineConfig([
  // TypeScript ESLint plugin
  // plugins.typescriptEslint, // plugin loaded by vue below

  // Airbnb base TypeScript config
  ...configs.base.typescript,
]);

const prettierConfig = defineConfig([
  // Prettier plugin
  {
    name: 'prettier/plugin/config',
    plugins: {
      prettier: prettierPlugin,
    },
  },
  // Prettier config
  {
    name: 'prettier/config',
    rules: {
      ...prettierConfigRules,
      'prettier/prettier': 'error',
    },
  },
]);

export default defineConfig([
  {
    name: 'app/files-to-lint',
    files: ['**/*.{vue,ts,mts,tsx}'],
  },

  {
    languageOptions: autoImportConfig,
  },

  // Ignore files and folders listed in .gitignore
  includeIgnoreFile(gitignorePath),
  globalIgnores(['**/dist/**', '**/dist-ssr/**', '**/coverage/**', 'src/types/models.ts']),

  // JavaScript config
  ...jsConfig,
  // Node config
  ...nodeConfig,
  // TypeScript config
  ...typescriptConfig,
  {
    name: 'app/import-resolver',
    files: ['**/*.{vue,ts,mts,tsx}'],
    settings: {
      'import-x/resolver-next': [
        createNodeResolver({
          extensions: ['.ts', '.cts', '.mts', '.d.ts', '.json'],
        }),
        createTypeScriptImportResolver({
          alwaysTryTypes: true,
          bun: true,
          noWarnOnMultipleProjects: true,
          project: tsResolverProjects,
        }),
      ],
    },
  },
  {
    languageOptions: {
      parserOptions: {
        projectService: true,
      },
    },
  },
  // Prettier config
  ...prettierConfig,

  ...pluginVue.configs['flat/essential'],
  ...defineConfigWithVueTs(vueTsConfigs.recommended),

  {
    ...pluginVitest.configs.recommended,
    files: ['src/**/__tests__/*'],
  },

  {
    plugins: {
      'simple-import-sort': simpleImportSort,
    },
    rules: {
      'simple-import-sort/imports': 'error',
      'simple-import-sort/exports': 'error',
    },
  },

  skipFormatting,

  {
    rules: {
      'import-x/prefer-default-export': 'off',
      'no-void': 'off', // allow void operator for top-level await
      '@typescript-eslint/no-use-before-define': 'warn',
      '@typescript-eslint/no-explicit-any': 'warn',
      curly: ['error', 'all'],
      'vue/no-undef-components': [
        'error',
        {
          ignorePatterns: [
            'router-link',
            'router-view',
            // primevue components pulled from llms.txt
            'Accordion',
            'AutoComplete',
            'Avatar',
            'Badge',
            'BlockUI',
            'Breadcrumb',
            'Button',
            'Card',
            'Carousel',
            'CascadeSelect',
            'Chart',
            'Checkbox',
            'Chip',
            'ColorPicker',
            'ConfirmationDialog',
            'ConfirmationPopup',
            'ContextMenu',
            'DataView',
            'DatePicker',
            'DeferredContent',
            'Dialog',
            'Divider',
            'Dock',
            'Drawer',
            'DynamicDialog',
            'Editor',
            'Fieldset',
            'FileUpload',
            'Fluid',
            'Gallery',
            'Image',
            'ImageCompare',
            'Inplace',
            'Input',
            'InputGroup',
            'InputNumber',
            'KeyFilter',
            'Knob',
            'Listbox',
            'Mask',
            'MegaMenu',
            'Menu',
            'Message',
            'MeterGroup',
            'MultiSelect',
            'Navbar',
            'OrderList',
            'OrganizationChart',
            'OtpInput',
            'Paginator',
            'Panel',
            'PanelMenu',
            'Password',
            'PickList',
            'Popover',
            'ProgressBar',
            'ProgressSpinner',
            'RadioButton',
            'Rating',
            'Ripple',
            'ScrollPanel',
            'ScrollTop',
            'Select',
            'SelectButton',
            'Skeleton',
            'Slider',
            'SpeedDial',
            'SplitButton',
            'Splitter',
            'Stepper',
            'Table',
            'Tabs',
            'Tag',
            'Terminal',
            'Textarea',
            'TieredMenu',
            'Timeline',
            'Toast',
            'ToggleButton',
            'ToggleSwitch',
            'Toolbar',
            'Tree',
            'TreeSelect',
            'TreeTable',
            'VirtualScroller',
          ],
        },
      ],
      'vue/block-lang': ['error', { script: { lang: 'ts', allowNoLang: true } }],
      'vue/multi-word-component-names': 'off',
    },
  },
]);
