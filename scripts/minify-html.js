const fs = require("fs");
const path = require("path");
const { minify } = require("@swc/html");

const SRC_DIR = path.join(__dirname, "../web_raw");
const DEST_DIR = path.join(__dirname, "../web");

const FILES = ["index.html", "rules.html", "settings.html", "search.html"];

async function processFile(file) {
    const srcPath = path.join(SRC_DIR, file);
    const destPath = path.join(DEST_DIR, file);
    const content = fs.readFileSync(srcPath, "utf-8");
    const { code } = await minify(content, {
        collapseWhitespaces: "all",
        removeComments: true,
        minifyJs: true,
        minifyCss: true,
        removeEmptyAttributes: true,
        removeRedundantAttributes: "all",
        tagOmission: false
    });
    const destDir = path.dirname(destPath);
    fs.mkdirSync(destDir, { recursive: true });
    fs.writeFileSync(destPath, code, "utf-8");
}

async function main() {
    for (const file of FILES) {
        await processFile(file);
    }
}

main();
