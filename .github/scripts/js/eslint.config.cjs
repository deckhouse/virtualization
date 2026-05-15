module.exports = [
  {
    ignores: ['node_modules/**'],
  },
  {
    files: ['**/*.js'],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: 'commonjs',
      globals: {
        __dirname: 'readonly',
        afterEach: 'readonly',
        beforeEach: 'readonly',
        Buffer: 'readonly',
        console: 'readonly',
        describe: 'readonly',
        expect: 'readonly',
        fetch: 'readonly',
        global: 'readonly',
        jest: 'readonly',
        module: 'readonly',
        process: 'readonly',
        require: 'readonly',
        setTimeout: 'readonly',
        test: 'readonly',
      },
    },
    rules: {
      'consistent-return': 'error',
      'no-shadow': 'error',
      'no-unused-vars': ['error', {argsIgnorePattern: '^_'}],
    },
  },
];
