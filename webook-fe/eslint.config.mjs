import nextConfig from 'eslint-config-next';
import nextCoreWebVitals from 'eslint-config-next/core-web-vitals';
import prettierConfig from 'eslint-config-prettier';
import prettierPlugin from 'eslint-plugin-prettier';

const config = [
  // Next.js recommended + core-web-vitals
  ...nextConfig,
  ...nextCoreWebVitals,

  // Prettier (must be last to override formatting rules)
  prettierConfig,

  // Custom rules
  {
    files: ['**/*.ts', '**/*.tsx'],
    plugins: {
      prettier: prettierPlugin,
    },
    settings: {
      'import/resolver': {
        typescript: { project: './tsconfig.json' },
      },
    },
    rules: {
      // Prettier
      'prettier/prettier': 'error',

      // 基础规则
      'no-undef': 'error',
      eqeqeq: ['error', 'always'],
      curly: 'error',
      quotes: ['warn', 'single'],
      semi: ['warn', 'always'],

      // TypeScript 规则
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_' },
      ],
      '@typescript-eslint/no-non-null-assertion': 'off',

      // React Hooks 规则
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',

      // React 规则
      'react/no-unescaped-entities': 'off',

      // 导入规则
      'import/no-duplicates': 'error',
      'import/no-anonymous-default-export': 'off',
      'import/order': [
        'error',
        {
          groups: [
            'builtin',
            'external',
            'internal',
            ['parent', 'sibling', 'index'],
          ],
          pathGroups: [
            {
              pattern: '@/**',
              group: 'internal',
              position: 'before',
            },
          ],
          pathGroupsExcludedImportTypes: ['builtin'],
          'newlines-between': 'always',
          alphabetize: { order: 'asc', caseInsensitive: true },
        },
      ],
    },
  },

  // Ignore patterns
  {
    ignores: ['.next/', 'node_modules/', 'out/'],
  },
];

export default config;
