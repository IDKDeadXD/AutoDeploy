import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import { mkdir, writeFile } from "node:fs/promises";
import { basename, dirname, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const dist = resolve(root, "dist");
const explicitVersion = process.argv[2];

function environmentFor(command, overrides = {}) {
	const env = { ...process.env, ...overrides };
	if (command === "gh") {
		delete env.GH_TOKEN;
		delete env.GITHUB_TOKEN;
	}
	return env;
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: root,
    encoding: "utf8",
    stdio: options.quiet ? ["ignore", "pipe", "pipe"] : "inherit",
    env: environmentFor(command, options.env),
  });
  if (result.error?.code === "ENOENT") {
    throw new Error(`${command} is not installed or is not on PATH`);
  }
  if (result.status !== 0) {
    const detail = options.quiet ? result.stderr.trim() : "";
    throw new Error(`${command} ${args.join(" ")} failed${detail ? `: ${detail}` : ""}`);
  }
	return result.stdout?.trim() ?? "";
}

function normalizeVersion(value) {
  const normalized = value.replace(/^v/, "");
  if (!/^\d+\.\d+\.\d+$/.test(normalized)) {
    throw new Error("version must use MAJOR.MINOR.PATCH, for example 0.1.1");
  }
  return normalized;
}

function nextPatch(tag) {
  const [major, minor, patch] = normalizeVersion(tag).split(".").map(Number);
  return `${major}.${minor}.${patch + 1}`;
}

function latestVersion() {
  try {
    return run("gh", ["release", "view", "--json", "tagName", "--jq", ".tagName"], { quiet: true });
  } catch {
    return "";
  }
}

function sourceVersion() {
	const source = readFileSync(resolve(root, "internal", "cli", "cli.go"), "utf8");
	const match = source.match(/Version\s*=\s*"(\d+\.\d+\.\d+)"/);
	if (!match) {
		throw new Error("could not find the source version in internal/cli/cli.go");
	}
	return match[1];
}

function requireGitHubAuth() {
	const result = spawnSync("gh", ["auth", "status"], {
		cwd: root,
		encoding: "utf8",
		stdio: ["ignore", "pipe", "pipe"],
		env: environmentFor("gh"),
	});
	if (result.error?.code === "ENOENT") {
		throw new Error("GitHub CLI is not installed. Install it with: winget install --id GitHub.cli");
	}
	if (result.status !== 0) {
		throw new Error("GitHub CLI is not authenticated. Run: gh auth login");
	}
}

function goExecutable() {
	if (process.platform !== "win32") {
		return "go";
	}
	const installed = "C:\\Program Files\\Go\\bin\\go.exe";
	return existsSync(installed) ? installed : "go";
}

function sha256(path) {
  return createHash("sha256").update(readFileSync(path)).digest("hex");
}

if (!existsSync(resolve(root, "go.mod"))) {
  throw new Error("run this command from the Deploy Agent repository");
}

requireGitHubAuth();
run("git", ["diff", "--quiet"]);
run("git", ["diff", "--cached", "--quiet"]);
run("git", ["push", "origin", "HEAD"]);

const latest = latestVersion();
const version = explicitVersion ? normalizeVersion(explicitVersion) : latest ? nextPatch(latest) : sourceVersion();
const tag = `v${version}`;
const commit = run("git", ["rev-parse", "--short", "HEAD"], { quiet: true });
const branch = run("git", ["branch", "--show-current"], { quiet: true });
if (!branch) {
	throw new Error("release publishing requires a checked-out branch");
}
const built = new Date().toISOString().replace(/\.\d{3}Z$/, "Z");
const ldflags = `-s -w -X github.com/idkde/deploy-agent/internal/cli.Version=${version} -X github.com/idkde/deploy-agent/internal/cli.Commit=${commit} -X github.com/idkde/deploy-agent/internal/cli.Built=${built}`;
const go = goExecutable();

await mkdir(dist, { recursive: true });
for (const [arch, asset] of [["amd64", "deploy-linux-amd64"], ["arm64", "deploy-linux-arm64"]]) {
	run(go, ["build", "-trimpath", "-ldflags", ldflags, "-o", resolve(dist, asset), "./cmd/deploy"], {
    env: { GOOS: "linux", GOARCH: arch },
  });
}

const assets = [resolve(dist, "deploy-linux-amd64"), resolve(dist, "deploy-linux-arm64")];
const checksums = `${assets.map((path) => `${sha256(path)}  ${basename(path)}`).join("\n")}\n`;
const checksumPath = resolve(dist, "checksums.txt");
await writeFile(checksumPath, checksums, "utf8");

run("gh", ["release", "create", tag, ...assets, checksumPath, "--title", tag, "--generate-notes", "--target", branch]);
console.log(`Published ${tag}. VPS servers can now run: deploy update`);
