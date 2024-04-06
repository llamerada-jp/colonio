module.exports = {
  mode: "development",

  entry: {
    colonio: "./src/main.ts"
  },
  output: {
    path: `${__dirname}/output`,
    filename: "[name].js",
    library: "Colonio"
  },
  module: {
    rules: [{
      test: /\.ts$/,
      use: "ts-loader"
    }]
  },
  resolve: {
    extensions: [".ts", ".js"]
  }
};