export default {
  target: [
    'web',
    'browserslist:last 1 chrome version,last 1 firefox version,last 1 safari version'
  ],
  name: 'web',
  context: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic',
  mode: 'production',
  infrastructureLogging: {
    level: 'error'
  },
  watchOptions: {
    aggregateTimeout: 0
  },
  experiments: {
    sourceImport: true
  },
  devtool: false,
  cache: {
    type: 'persistent',
    version: 'web-development-1d8fc6ceb1f94c63',
    storage: {
      type: 'filesystem',
      directory: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/node_modules/.cache/rspack'
    },
    buildDependencies: [
      '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/package.json',
      '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/rsbuild.config.ts',
      '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/tailwind.config.js'
    ]
  },
  output: {
    devtoolModuleFilenameTemplate: '[relative-resource-path]',
    devtoolFallbackModuleFilenameTemplate: '[relative-resource-path]?[hash]',
    path: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/dist',
    filename: 'static/js/[name].[contenthash:10].js',
    chunkFilename: 'static/js/async/[name].[contenthash:10].js',
    publicPath: '/',
    environment: {
      'const': false
    },
    assetModuleFilename: 'static/assets/[name].[contenthash:10][ext]',
    webassemblyModuleFilename: 'static/wasm/[contenthash:10].module.wasm'
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
    extensions: [
      '.ts',
      '.tsx',
      '.mjs',
      '.js',
      '.jsx',
      '.json'
    ]
  },
  module: {
    parser: {
      javascript: {
        typeReexportsPresence: 'tolerant'
      }
    },
    rules: [
      /* config.module.rule('mjs') */
      {
        test: /\.m?js/,
        resolve: {
          fullySpecified: false
        }
      },
      /* config.module.rule('css') */
      {
        test: /\.css$/,
        dependency: {
          not: 'url'
        },
        oneOf: [
          /* config.module.rule('css').oneOf('css-text') */
          {
            'with': {
              type: 'text'
            },
            type: 'asset/source'
          },
          /* config.module.rule('css').oneOf('css-url') */
          {
            resourceQuery: /[?&]url(?:&|=|$)/,
            use: [
              /* config.module.rule('css').oneOf('css-url').use('css-url') */
              {
                loader: '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@rsbuild/core/dist/cssUrlLoader.mjs',
                options: {
                  filename: 'static/css/[name].[contenthash:10].css',
                  modules: {
                    auto: true,
                    namedExport: false,
                    exportGlobals: false,
                    exportLocalsConvention: 'camelCase',
                    localIdentName: '[local]-[hash:base64:6]'
                  }
                }
              },
              /* config.module.rule('css').oneOf('css-url').use('css') */
              {
                loader: '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@rsbuild/core/compiled/css-loader/index.js',
                options: {
                  modules: false,
                  sourceMap: false,
                  exportType: 'string',
                  importLoaders: 2
                }
              },
              /* config.module.rule('css').oneOf('css-url').use('lightningcss') */
              {
                loader: 'builtin:lightningcss-loader',
                options: {
                  targets: [
                    'last 1 chrome version',
                    'last 1 firefox version',
                    'last 1 safari version'
                  ],
                  errorRecovery: true
                }
              },
              /* config.module.rule('css').oneOf('css-url').use('postcss') */
              {
                loader: '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@rsbuild/core/compiled/postcss-loader/index.js',
                options: {
                  implementation: '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@rsbuild/core/compiled/postcss/index.js',
                  postcssOptions: {
                    file: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/postcss.config.js',
                    options: {
                      cwd: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic',
                      env: 'development'
                    },
                    plugins: [
                      {
                        postcssPlugin: 'tailwindcss',
                        plugins: [
                          function () { /* omitted long function */ }
                        ]
                      },
                      {
                        browsers: undefined,
                        info: function info() { /* omitted long function */ },
                        options: {},
                        postcssPlugin: 'autoprefixer',
                        prepare: function prepare() { /* omitted long function */ }
                      }
                    ],
                    config: false
                  },
                  sourceMap: false
                }
              }
            ],
            resolve: {
              preferRelative: true
            }
          },
          /* config.module.rule('css').oneOf('css-inline') */
          {
            resourceQuery: /[?&]inline(?:&|=|$)/,
            sideEffects: true,
            use: [
              /* config.module.rule('css').oneOf('css-inline').use('css') */
              {
                loader: '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@rsbuild/core/compiled/css-loader/index.js',
                options: {
                  modules: false,
                  sourceMap: false,
                  exportType: 'string',
                  importLoaders: 2
                }
              },
              /* config.module.rule('css').oneOf('css-inline').use('lightningcss') */
              {
                loader: 'builtin:lightningcss-loader',
                options: {
                  targets: [
                    'last 1 chrome version',
                    'last 1 firefox version',
                    'last 1 safari version'
                  ],
                  errorRecovery: true
                }
              },
              /* config.module.rule('css').oneOf('css-inline').use('postcss') */
              {
                loader: '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@rsbuild/core/compiled/postcss-loader/index.js',
                options: {
                  implementation: '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@rsbuild/core/compiled/postcss/index.js',
                  postcssOptions: {
                    file: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/postcss.config.js',
                    options: {
                      cwd: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic',
                      env: 'development'
                    },
                    plugins: [
                      {
                        postcssPlugin: 'tailwindcss',
                        plugins: [
                          function () { /* omitted long function */ }
                        ]
                      },
                      {
                        browsers: undefined,
                        info: function info() { /* omitted long function */ },
                        options: {},
                        postcssPlugin: 'autoprefixer',
                        prepare: function prepare() { /* omitted long function */ }
                      }
                    ],
                    config: false
                  },
                  sourceMap: false
                }
              }
            ],
            resolve: {
              preferRelative: true
            }
          },
          /* config.module.rule('css').oneOf('css-raw') */
          {
            type: 'asset/source',
            resourceQuery: /[?&]raw(?:&|=|$)/
          },
          /* config.module.rule('css').oneOf('css') */
          {
            sideEffects: true,
            use: [
              /* config.module.rule('css').oneOf('css').use('mini-css-extract') */
              {
                loader: '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@rspack/core/dist/cssExtractLoader.js'
              },
              /* config.module.rule('css').oneOf('css').use('css') */
              {
                loader: '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@rsbuild/core/compiled/css-loader/index.js',
                options: {
                  modules: {
                    auto: true,
                    namedExport: false,
                    exportGlobals: false,
                    exportLocalsConvention: 'camelCase',
                    localIdentName: '[local]-[hash:base64:6]'
                  },
                  sourceMap: false,
                  importLoaders: 2
                }
              },
              /* config.module.rule('css').oneOf('css').use('lightningcss') */
              {
                loader: 'builtin:lightningcss-loader',
                options: {
                  targets: [
                    'last 1 chrome version',
                    'last 1 firefox version',
                    'last 1 safari version'
                  ],
                  errorRecovery: true
                }
              },
              /* config.module.rule('css').oneOf('css').use('postcss') */
              {
                loader: '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@rsbuild/core/compiled/postcss-loader/index.js',
                options: {
                  implementation: '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@rsbuild/core/compiled/postcss/index.js',
                  postcssOptions: {
                    file: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/postcss.config.js',
                    options: {
                      cwd: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic',
                      env: 'development'
                    },
                    plugins: [
                      {
                        postcssPlugin: 'tailwindcss',
                        plugins: [
                          function () { /* omitted long function */ }
                        ]
                      },
                      {
                        browsers: undefined,
                        info: function info() { /* omitted long function */ },
                        options: {},
                        postcssPlugin: 'autoprefixer',
                        prepare: function prepare() { /* omitted long function */ }
                      }
                    ],
                    config: false
                  },
                  sourceMap: false
                }
              }
            ],
            resolve: {
              preferRelative: true
            }
          }
        ]
      },
      /* config.module.rule('js') */
      {
        test: /\.(?:js|jsx|mjs|cjs|ts|tsx|mts|cts)$/,
        dependency: {
          not: 'url'
        },
        include: [
          {
            not: /[\\/]node_modules[\\/]/
          },
          /\.(?:ts|tsx|jsx|mts|cts)$/
        ],
        oneOf: [
          /* config.module.rule('js').oneOf('js-worker') */
          {
            resourceQuery: /[?&]worker(?:&|=|$)/,
            type: 'javascript/auto',
            use: [
              /* config.module.rule('js').oneOf('js-worker').use('worker-query') */
              {
                loader: '/Users/huangzx/Documents/my_work/new-api-sd/web/node_modules/@rsbuild/core/dist/workerLoader.mjs'
              }
            ]
          },
          /* config.module.rule('js').oneOf('js-text') */
          {
            'with': {
              type: 'text'
            },
            type: 'asset/source'
          },
          /* config.module.rule('js').oneOf('js-raw') */
          {
            resourceQuery: /[?&]raw(?:&|=|$)/,
            type: 'asset/source'
          },
          /* config.module.rule('js').oneOf('js') */
          {
            type: 'javascript/auto',
            use: [
              /* config.module.rule('js').oneOf('js').use('swc') */
              {
                loader: 'builtin:swc-loader',
                options: {
                  detectSyntax: 'auto',
                  jsc: {
                    externalHelpers: true,
                    parser: {
                      decorators: true
                    },
                    experimental: {
                      cacheRoot: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/node_modules/.cache/.swc',
                      keepImportAttributes: true
                    },
                    output: {
                      charset: 'utf8'
                    },
                    transform: {
                      legacyDecorator: false,
                      decoratorVersion: '2023-11',
                      react: {
                        development: false,
                        refresh: false,
                        runtime: 'automatic'
                      }
                    }
                  },
                  isModule: 'unknown',
                  env: {
                    targets: [
                      'last 1 chrome version',
                      'last 1 firefox version',
                      'last 1 safari version'
                    ]
                  },
                  collectTypeScriptInfo: {
                    typeExports: true,
                    exportedEnum: true
                  }
                }
              }
            ]
          }
        ]
      },
      /* config.module.rule('js-data-uri') */
      {
        mimetype: {
          or: [
            'text/javascript',
            'application/javascript'
          ]
        },
        use: [
          /* config.module.rule('js-data-uri').use('swc') */
          {
            loader: 'builtin:swc-loader',
            options: {
              detectSyntax: 'auto',
              jsc: {
                externalHelpers: true,
                parser: {
                  decorators: true
                },
                experimental: {
                  cacheRoot: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/node_modules/.cache/.swc',
                  keepImportAttributes: true
                },
                output: {
                  charset: 'utf8'
                },
                transform: {
                  legacyDecorator: false,
                  decoratorVersion: '2023-11',
                  react: {
                    development: false,
                    refresh: false,
                    runtime: 'automatic'
                  }
                }
              },
              isModule: 'unknown',
              env: {
                targets: [
                  'last 1 chrome version',
                  'last 1 firefox version',
                  'last 1 safari version'
                ]
              },
              collectTypeScriptInfo: {
                typeExports: true,
                exportedEnum: true
              }
            }
          }
        ],
        resolve: {
          fullySpecified: false
        }
      },
      /* config.module.rule('image') */
      {
        test: /\.(?:png|jpg|jpeg|pjpeg|pjp|gif|bmp|webp|ico|apng|avif|tif|tiff|jfif|cur)$/i,
        oneOf: [
          /* config.module.rule('image').oneOf('image-asset-url') */
          {
            type: 'asset/resource',
            resourceQuery: /[?&]url(?:&|=|$)/,
            generator: {
              filename: 'static/image/[name].[contenthash:10][ext]'
            }
          },
          /* config.module.rule('image').oneOf('image-asset-inline') */
          {
            type: 'asset/inline',
            resourceQuery: /[?&]inline(?:&|=|$)/
          },
          /* config.module.rule('image').oneOf('image-asset-text') */
          {
            type: 'asset/source',
            'with': {
              type: 'text'
            }
          },
          /* config.module.rule('image').oneOf('image-asset-raw') */
          {
            type: 'asset/source',
            resourceQuery: /[?&]raw(?:&|=|$)/
          },
          /* config.module.rule('image').oneOf('image-asset') */
          {
            type: 'asset',
            parser: {
              dataUrlCondition: {
                maxSize: 4096
              }
            },
            generator: {
              filename: 'static/image/[name].[contenthash:10][ext]'
            }
          }
        ]
      },
      /* config.module.rule('svg') */
      {
        test: /\.svg$/i,
        oneOf: [
          /* config.module.rule('svg').oneOf('svg-asset-url') */
          {
            type: 'asset/resource',
            resourceQuery: /[?&]url(?:&|=|$)/,
            generator: {
              filename: 'static/svg/[name].[contenthash:10].svg'
            }
          },
          /* config.module.rule('svg').oneOf('svg-asset-inline') */
          {
            type: 'asset/inline',
            resourceQuery: /[?&]inline(?:&|=|$)/
          },
          /* config.module.rule('svg').oneOf('svg-asset-text') */
          {
            type: 'asset/source',
            'with': {
              type: 'text'
            }
          },
          /* config.module.rule('svg').oneOf('svg-asset-raw') */
          {
            type: 'asset/source',
            resourceQuery: /[?&]raw(?:&|=|$)/
          },
          /* config.module.rule('svg').oneOf('svg-asset') */
          {
            type: 'asset',
            parser: {
              dataUrlCondition: {
                maxSize: 4096
              }
            },
            generator: {
              filename: 'static/svg/[name].[contenthash:10].svg'
            }
          }
        ]
      },
      /* config.module.rule('media') */
      {
        test: /\.(?:mp4|webm|ogg|mov|mp3|wav|flac|aac|m4a|opus)$/i,
        oneOf: [
          /* config.module.rule('media').oneOf('media-asset-url') */
          {
            type: 'asset/resource',
            resourceQuery: /[?&]url(?:&|=|$)/,
            generator: {
              filename: 'static/media/[name].[contenthash:10][ext]'
            }
          },
          /* config.module.rule('media').oneOf('media-asset-inline') */
          {
            type: 'asset/inline',
            resourceQuery: /[?&]inline(?:&|=|$)/
          },
          /* config.module.rule('media').oneOf('media-asset-text') */
          {
            type: 'asset/source',
            'with': {
              type: 'text'
            }
          },
          /* config.module.rule('media').oneOf('media-asset-raw') */
          {
            type: 'asset/source',
            resourceQuery: /[?&]raw(?:&|=|$)/
          },
          /* config.module.rule('media').oneOf('media-asset') */
          {
            type: 'asset',
            parser: {
              dataUrlCondition: {
                maxSize: 4096
              }
            },
            generator: {
              filename: 'static/media/[name].[contenthash:10][ext]'
            }
          }
        ]
      },
      /* config.module.rule('font') */
      {
        test: /\.(?:woff|woff2|eot|ttf|otf|ttc)$/i,
        oneOf: [
          /* config.module.rule('font').oneOf('font-asset-url') */
          {
            type: 'asset/resource',
            resourceQuery: /[?&]url(?:&|=|$)/,
            generator: {
              filename: 'static/font/[name].[contenthash:10][ext]'
            }
          },
          /* config.module.rule('font').oneOf('font-asset-inline') */
          {
            type: 'asset/inline',
            resourceQuery: /[?&]inline(?:&|=|$)/
          },
          /* config.module.rule('font').oneOf('font-asset-text') */
          {
            type: 'asset/source',
            'with': {
              type: 'text'
            }
          },
          /* config.module.rule('font').oneOf('font-asset-raw') */
          {
            type: 'asset/source',
            resourceQuery: /[?&]raw(?:&|=|$)/
          },
          /* config.module.rule('font').oneOf('font-asset') */
          {
            type: 'asset',
            parser: {
              dataUrlCondition: {
                maxSize: 4096
              }
            },
            generator: {
              filename: 'static/font/[name].[contenthash:10][ext]'
            }
          }
        ]
      },
      /* config.module.rule('json') */
      {
        test: /\.json$/i,
        oneOf: [
          /* config.module.rule('json').oneOf('json-asset-text') */
          {
            type: 'asset/source',
            'with': {
              type: 'text'
            }
          },
          /* config.module.rule('json').oneOf('json-asset-raw') */
          {
            type: 'asset/source',
            resourceQuery: /[?&]raw(?:&|=|$)/
          }
        ]
      },
      /* config.module.rule('wasm') */
      {
        test: /\.wasm$/,
        dependency: 'url',
        type: 'asset/resource',
        generator: {
          filename: 'static/wasm/[contenthash:10].module.wasm'
        }
      },
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
  },
  optimization: {
    minimize: false,
    splitChunks: {
      chunks: 'all',
      cacheGroups: {
        react: {
          name: 'lib-react',
          test: /node_modules[\\/](?:react|react-dom|scheduler)[\\/]/,
          priority: 0
        },
        router: {
          name: 'lib-router',
          test: /node_modules[\\/](?:react-router|react-router-dom|history|@remix-run[\\/]router)[\\/]/,
          priority: 0
        }
      }
    }
  },
  plugins: [
    /* config.plugin('mini-css-extract') */
    new CssExtractRspackPlugin(
      {
        filename: 'static/css/[name].[contenthash:10].css',
        chunkFilename: 'static/css/async/[name].[contenthash:10].css',
        ignoreOrder: true
      }
    ),
    /* config.plugin('RsbuildCorePlugin') */
    {},
    /* config.plugin('html-index') */
    new HtmlRspackPlugin(
      {
        meta: {
          viewport: 'width=device-width, initial-scale=1.0'
        },
        chunks: [
          'index'
        ],
        inject: 'head',
        filename: 'index.html',
        entryName: 'index',
        templateParameters: function () { /* omitted long function */ },
        scriptLoading: 'defer',
        template: '/Users/huangzx/Documents/my_work/new-api-sd/web/classic/index.html',
        title: 'Rsbuild App'
      }
    ),
    /* config.plugin('rsbuild-html-plugin') */
    new RsbuildHtmlPlugin(
      (entryName)=>extraDataMap.get(entryName),
      ()=>HtmlPlugin
    ),
    /* config.plugin('define') */
    new DefinePlugin(
      {
        'import.meta.env': {
          MODE: '"production"',
          DEV: false,
          PROD: true,
          SSR: false,
          BASE_URL: '"/"',
          ASSET_PREFIX: '""'
        },
        'process.env.BASE_URL': '"/"',
        'process.env.ASSET_PREFIX': '""',
        'import.meta.env.VITE_REACT_APP_SERVER_URL': '""'
      }
    )
  ],
  performance: {
    hints: false
  },
  entry: {
    index: [
      './src/index.jsx'
    ]
  }
}