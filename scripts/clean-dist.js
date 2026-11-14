const fs = require("fs");
const path = require("path");

const dir = path.join(__dirname, "../web-dist");
if (!fs.existsSync(dir)) process.exit(0);

for (const name of fs.readdirSync(dir)) {
    if (name === ".gitkeep") continue;
    fs.rmSync(path.join(dir, name), { recursive: true, force: true });
}
