import { cpSync, existsSync, mkdirSync, rmSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const source = resolve(projectRoot, "node_modules/monaco-editor/min/vs");
const destination = resolve(projectRoot, "public/monaco/vs");

if (!existsSync(source)) {
  throw new Error(`Monaco runtime assets are missing: ${source}`);
}

rmSync(destination, { recursive: true, force: true });
mkdirSync(dirname(destination), { recursive: true });
cpSync(source, destination, { recursive: true });
