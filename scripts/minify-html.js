const fs = require('fs');
const path = require('path');
const { minify } = require('html-minifier-terser');

const SRC_DIR = path.join(__dirname, '../web_raw');
const DEST_DIR = path.join(__dirname, '../web');

const FILES = [
    'index.html',
    'rules.html',
    'settings.html'
];

async function processFile(file) {
    const srcPath = path.join(SRC_DIR, file);
    const destPath = path.join(DEST_DIR, file);
    const content = fs.readFileSync(srcPath, 'utf-8');
    const result = await minify(content, {
        collapseWhitespace: true,
        removeComments: true,
        minifyJS: true,
        minifyCSS: true,
        removeEmptyAttributes: true,
        removeRedundantAttributes: true,
        useShortDoctype: true
    });
    const destDir = path.dirname(destPath);
    fs.mkdirSync(destDir, { recursive: true });
    fs.writeFileSync(destPath, result, 'utf-8');
}

async function main() {
    for (const file of FILES) {
        await processFile(file);
    }
}

main();