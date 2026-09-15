// Runs on the HOST before container creation.
// Node is used because the devcontainer CLI ships it, which avoids every host-shell
// difference; array form in devcontainer.json avoids shell quoting entirely.
// Deliberately never fails the container start: a missing personal layer is a
// supported configuration that degrades to pi defaults.
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const src = process.env.PI_DC_PERSONAL || path.join(os.homedir(), ".pi", "devcontainer");
const dest = path.join(process.cwd(), ".devcontainer", ".personal");

fs.rmSync(dest, { recursive: true, force: true });
fs.mkdirSync(dest, { recursive: true });

if (fs.existsSync(src)) {
  fs.cpSync(src, dest, { recursive: true });
  console.log(`personal layer: copied ${fs.readdirSync(dest).join(", ")} from ${src}`);
} else {
  console.log(`personal layer: none at ${src} — continuing with pi defaults`);
}
