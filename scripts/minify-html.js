const fs = require("fs");
const path = require("path");
const { minify } = require("@swc/html");

const SRC_DIR = path.join(__dirname, "../web");
const DEST_DIR = path.join(__dirname, "../web-dist");

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

function copyStatic() {
    const srcDir = path.join(SRC_DIR, "assets");
    const destDir = path.join(DEST_DIR, "assets");
    fs.mkdirSync(destDir, { recursive: true });
    for (const file of fs.readdirSync(srcDir)) {
        if (file.endsWith(".svg")) {
            fs.copyFileSync(path.join(srcDir, file), path.join(destDir, file));
        }
    }
}

async function main() {
    for (const file of FILES) {
        await processFile(file);
    }
    copyStatic();
}

main();
