export default {
  tools: {
    swc: {
      jsc: {
        transform: {
          react: {
            development: false,
            refresh: false,
            runtime: 'automatic'
          }
        }
      }
    },
    cssExtract: {
      loaderOptions: {},
      pluginOptions: {
        ignoreOrder: true
      }
    },
    rspack: {
      module: {
        rules: [
          {
            test: /src[\\/].*\.js$/,
            type: 'javascript/auto',
            use: [
              {
                loader: 'builtin:swc-loader',
                options: {
                  jsc: {
                    parser: {
                      syntax: 'ecmascript',
                      jsx: true
                    },
                    transform: {
                      react: {
                        runtime: 'automatic',
                        development: true,
                        refresh: true
                      }
                    }
                  }
                }
              }
            ]
          }
        ]
      }
    }
  },
  html: {
    meta: {
      charset: {
        charset: 'utf-8'
      },
      viewport: 'width=device-width, initial-scale=1.0'
    },
    title: 'Rsbuild App',
    inject: 'head',
    mountId: 'root',
    crossorigin: false,
    outputStructure: 'flat',
    scriptLoading: 'defer',
    implementation: 'js',
    template: './index.html'
  },
  resolve: {
    alias: {
      '@swc/helpers': '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@swc/helpers',
      '@': '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/src',
      '@visactor/react-vchart': '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/node_modules/@visactor/react-vchart',
      '@visactor/vchart': '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/node_modules/@visactor/vchart',
      '@visactor/vutils-extension': '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/node_modules/@visactor/vchart/node_modules/@visactor/vutils-extension',
      '@visactor/vrender-components': '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/node_modules/@visactor/vchart/node_modules/@visactor/vrender-components',
      '@visactor/vrender-kits': '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/node_modules/@visactor/vchart/node_modules/@visactor/vrender-kits',
      '@visactor/vrender-core': '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/node_modules/@visactor/vchart/node_modules/@visactor/vrender-core',
      '@visactor/vdataset': '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/node_modules/@visactor/vchart/node_modules/@visactor/vdataset',
      '@visactor/vscale': '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/node_modules/@visactor/vchart/node_modules/@visactor/vscale',
      '@visactor/vutils': '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/node_modules/@visactor/vchart/node_modules/@visactor/vutils',
      '@douyinfe/semi-ui/dist/css/semi.css': '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@douyinfe/semi-ui/dist/css/semi.css',
      'date-fns': '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@douyinfe/semi-foundation/node_modules/date-fns'
    },
    aliasStrategy: 'prefer-tsconfig',
    extensions: [
      '.ts',
      '.tsx',
      '.mjs',
      '.js',
      '.jsx',
      '.json'
    ]
  },
  source: {
    define: {
      'import.meta.env.VITE_REACT_APP_SERVER_URL': '""'
    },
    preEntry: [],
    decorators: {
      version: '2023-11'
    },
    entry: {
      index: './src/index.jsx'
    }
  },
  output: {
    target: 'web',
    cleanDistPath: 'auto',
    distPath: {
      root: 'dist',
      css: 'static/css',
      svg: 'static/svg',
      font: 'static/font',
      html: './',
      wasm: 'static/wasm',
      image: 'static/image',
      media: 'static/media',
      assets: 'static/assets',
      favicon: './',
      js: 'static/js'
    },
    assetPrefix: '/',
    filename: {},
    charset: 'utf8',
    polyfill: 'off',
    dataUriLimit: {
      svg: 4096,
      font: 4096,
      image: 4096,
      media: 4096,
      assets: 4096
    },
    legalComments: 'linked',
    injectStyles: false,
    manifest: false,
    sourceMap: {
      js: undefined,
      css: false,
      extract: false
    },
    filenameHash: true,
    inlineScripts: false,
    inlineStyles: false,
    cssModules: {
      auto: true,
      namedExport: false,
      exportGlobals: false,
      exportLocalsConvention: 'camelCase'
    },
    emitAssets: true,
    minify: false,
    module: false
  },
  security: {
    nonce: '',
    sri: {
      enable: false
    }
  },
  splitChunks: {},
  performance: {
    printFileSize: true,
    removeConsole: false,
    buildCache: {
      cacheDigest: [
        undefined
      ]
    }
  },
  logLevel: 'info',
  mode: 'production',
  plugins: [
    {
      name: 'rsbuild:basic',
      setup() {}
    },
    {
      name: 'rsbuild:entry',
      setup() {}
    },
    {
      name: 'rsbuild:source-map',
      setup() {}
    },
    {
      name: 'rsbuild:cache',
      setup() {}
    },
    {
      name: 'rsbuild:target',
      setup() {}
    },
    {
      name: 'rsbuild:output',
      setup() {}
    },
    {
      name: 'rsbuild:resolve',
      setup() {}
    },
    {
      name: 'rsbuild:file-size',
      setup() {}
    },
    {
      name: 'rsbuild:clean-output',
      setup() {}
    },
    {
      name: 'rsbuild:asset',
      setup() {}
    },
    {
      name: 'rsbuild:html',
      setup() {}
    },
    {
      name: 'rsbuild:appIcon',
      setup() {}
    },
    {
      name: 'rsbuild:wasm',
      setup() {}
    },
    {
      name: 'rsbuild:node-addons',
      setup() {}
    },
    {
      name: 'rsbuild:define',
      setup() {}
    },
    {
      name: 'rsbuild:css',
      setup() {}
    },
    {
      name: 'rsbuild:minimize',
      setup() {}
    },
    {
      name: 'rsbuild:progress',
      setup() {}
    },
    {
      name: 'rsbuild:worker',
      setup() {}
    },
    {
      name: 'rsbuild:swc',
      setup() {}
    },
    {
      name: 'rsbuild:externals',
      setup() {}
    },
    {
      name: 'rsbuild:split-chunks',
      setup() {}
    },
    {
      name: 'rsbuild:inline-chunk',
      setup() {}
    },
    {
      name: 'rsbuild:rsdoctor',
      setup() {}
    },
    {
      name: 'rsbuild:resource-hints',
      setup() {}
    },
    {
      name: 'rsbuild:server',
      setup() {}
    },
    {
      name: 'rsbuild:manifest',
      setup() {}
    },
    {
      name: 'rsbuild:module-federation',
      setup() {}
    },
    {
      name: 'rsbuild:rspack-profile',
      setup() {}
    },
    {
      name: 'rsbuild:lazy-compilation',
      apply: 'serve',
      setup() {}
    },
    {
      name: 'rsbuild:sri',
      setup() {}
    },
    {
      name: 'rsbuild:nonce',
      setup() {}
    },
    {
      name: 'rsbuild:react',
      setup() {}
    }
  ],
  _privateMeta: {
    configFilePath: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/rsbuild.config.ts'
  },
  root: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic',
  dev: {
    hmr: true,
    liveReload: true,
    browserLogs: {
      stackTrace: 'summary'
    },
    watchFiles: [
      {
        paths: [
          '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/rsbuild.config.ts'
        ],
        type: 'reload-server'
      }
    ],
    assetPrefix: '/',
    writeToDisk: false,
    cliShortcuts: true,
    client: {
      path: '/rsbuild-hmr',
      port: '',
      host: '',
      overlay: true,
      reconnect: 100,
      logLevel: 'info'
    },
    lazyCompilation: {
      imports: true,
      entries: false
    }
  },
  server: {
    port: 3000,
    host: '0.0.0.0',
    open: false,
    base: '/',
    htmlFallback: 'index',
    compress: true,
    printUrls: true,
    strictPort: false,
    cors: {
      origin: /^https?:\/\/(?:(?:[^:]+\.)?localhost|127\.0\.0\.1|\[::1\])(?::\d+)?$/
    },
    middlewareMode: false,
    proxy: {
      '/api': {
        target: 'http://localhost:3000',
        changeOrigin: true
      },
      '/mj': {
        target: 'http://localhost:3000',
        changeOrigin: true
      },
      '/pg': {
        target: 'http://localhost:3000',
        changeOrigin: true
      }
    },
    publicDir: [
      {
        name: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/public',
        copyOnBuild: 'auto',
        watch: false,
        ignore: []
      }
    ]
  }
}