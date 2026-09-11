#!/usr/bin/env node

import { spawnSync } from 'node:child_process';
import crypto from 'node:crypto';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const SCRIPT_DIR = path.dirname(fileURLToPath(import.meta.url));
const DEFAULT_PLUGIN_ROOT = path.resolve(SCRIPT_DIR, '..');
const PLUGIN_NAME = 'atlasflow';
const MARKETPLACE_NAME = 'analytix-hub';
const DEFAULT_PACKAGE_BASE_URL = 'https://analytix.top/api/agent/plugins/packages';
const SEMVER_PATTERN = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/u;

function text(value) {
  return String(value == null ? '' : value).trim();
}

function parseArgs(argv) {
  const options = {
    json: false,
    force: false,
    writeHub: false,
    pluginRoot: DEFAULT_PLUGIN_ROOT,
    outRoot: '',
    archivePath: '',
    packageBaseUrl: DEFAULT_PACKAGE_BASE_URL
  };

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === '--json') options.json = true;
    else if (arg === '--force') options.force = true;
    else if (arg === '--write-hub') options.writeHub = true;
    else if (arg === '--plugin-root') {
      options.pluginRoot = path.resolve(text(argv[index + 1]));
      index += 1;
    } else if (arg === '--out-root') {
      options.outRoot = path.resolve(text(argv[index + 1]));
      index += 1;
    } else if (arg === '--archive') {
      options.archivePath = path.resolve(text(argv[index + 1]));
      index += 1;
    } else if (arg === '--package-base-url') {
      options.packageBaseUrl = text(argv[index + 1]).replace(/\/+$/u, '');
      index += 1;
    } else if (arg === '-h' || arg === '--help') {
      printHelp();
      process.exit(0);
    } else {
      throw new Error(`Unknown argument: ${arg}`);
    }
  }
  return options;
}

function printHelp() {
  console.log(`Usage: node scripts/prepare-hub-package.mjs [options]

Prepares the AtlasFlow plugin package and Analytix Hub catalog files.
It creates a checksum-bound tar.gz package that the analytix plugin page can
discover through marketplace.json and install through the Hub downloader.

Options:
  --plugin-root <path>       Plugin root. Default: current plugin.
  --out-root <path>          Output root. Default: /tmp/analytix-hub-atlasflow-<version>
  --archive <path>           Output tar.gz. Default: /tmp/atlasflow-<version>.tar.gz
  --package-base-url <url>   Public URL prefix for the tar.gz.
                             Default: ${DEFAULT_PACKAGE_BASE_URL}
  --write-hub                Update pluginRoot/hub/*.json with generated catalog files.
  --force                    Replace existing output/archive.
  --json                     Print JSON.
`);
}

function readJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, 'utf8'));
}

function writeJson(filePath, value) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, `${JSON.stringify(value, null, 2)}\n`, 'utf8');
}

function assertInsideTmp(targetPath) {
  const resolved = path.resolve(targetPath);
  const tmpRoot = path.resolve(os.tmpdir());
  if (resolved !== tmpRoot && !resolved.startsWith(`${tmpRoot}${path.sep}`)) {
    throw new Error(`Refusing to write outside temporary directory: ${resolved}`);
  }
}

function copyPluginSource(pluginRoot, destinationPluginRoot) {
  fs.cpSync(pluginRoot, destinationPluginRoot, {
    recursive: true,
    dereference: false,
    filter(source) {
      const base = path.basename(source);
      const relativePath = path.relative(pluginRoot, source).split(path.sep).join('/');
      if (base === '.DS_Store' || base === 'node_modules' || base === '.git') return false;
      if (relativePath === 'hub' || relativePath.startsWith('hub/')) return false;
      if (relativePath === 'scripts' || relativePath.startsWith('scripts/')) return false;
      if (relativePath === 'dist' || relativePath.startsWith('dist/')) return false;
      if (relativePath === 'output' || relativePath.startsWith('output/')) return false;
      return true;
    }
  });
}

function sha256File(filePath) {
  const hash = crypto.createHash('sha256');
  hash.update(fs.readFileSync(filePath));
  return hash.digest('hex');
}

function createArchive(outRoot, archivePath, force) {
  assertInsideTmp(archivePath);
  if (fs.existsSync(archivePath)) {
    if (!force) throw new Error(`Archive already exists; pass --force to replace: ${archivePath}`);
    fs.rmSync(archivePath, { force: true });
  }
  const result = spawnSync('tar', ['-C', path.join(outRoot, 'plugins'), '-czf', archivePath, PLUGIN_NAME], {
    encoding: 'utf8'
  });
  if (result.status !== 0) {
    throw new Error(text(result.stderr || result.stdout) || 'tar failed');
  }
  return {
    path: archivePath,
    sha256: sha256File(archivePath),
    size_bytes: fs.statSync(archivePath).size
  };
}

function marketplace(manifest, packageUrl, archive) {
  const manifestInterface = manifest.interface || {};
  return {
    name: MARKETPLACE_NAME,
    interface: {
      displayName: 'Analytix Hub'
    },
    platform: 'mac-arm64',
    plugins: [
      {
        name: PLUGIN_NAME,
        version: text(manifest.version),
        source: {
          source: 'download',
          url: packageUrl,
          sha256: archive.sha256
        },
        policy: {
          installation: 'AVAILABLE',
          authentication: 'ON_INSTALL'
        },
        category: text(manifestInterface.category) || 'Productivity',
        interface: manifestInterface
      }
    ]
  };
}

function skillCatalog(manifest) {
  const version = text(manifest.version);
  return {
    platform: 'mac-arm64',
    generatedAt: '2026-07-05T00:00:00.000Z',
    skills: [
      {
        id: 'atlasflow:atlasflow',
        skillName: 'atlasflow',
        displayName: 'AtlasFlow',
        shortDescription: 'Core diagram generation: architecture, workflow, sequence, dataflow, lifecycle.',
        category: 'Diagramming',
        sourceKind: 'plugin',
        scope: 'plugin',
        pluginName: PLUGIN_NAME,
        version,
        upstreamMarketplaceName: MARKETPLACE_NAME,
        skillPath: 'skills/atlasflow/SKILL.md',
        interface: {
          displayName: 'AtlasFlow',
          shortDescription: 'Professional diagram generation core Skill.'
        },
        skill: {
          description: 'Create professional architecture, workflow, sequence, dataflow, and lifecycle diagrams.'
        }
      },
      {
        id: 'atlasflow:atlasflow-analytix-domain',
        skillName: 'atlasflow-analytix-domain',
        displayName: 'AtlasFlow analytix domain',
        shortDescription: 'Public-security, official-document, case-project, and general business graph routing.',
        category: 'Case Analysis',
        sourceKind: 'plugin',
        scope: 'plugin',
        pluginName: PLUGIN_NAME,
        version,
        upstreamMarketplaceName: MARKETPLACE_NAME,
        skillPath: 'skills/atlasflow-analytix-domain/SKILL.md',
        interface: {
          displayName: 'AtlasFlow analytix domain',
          shortDescription: 'Maps public-security, official-document, case, and general tasks to AtlasFlow diagrams.'
        },
        skill: {
          description: 'Route public-security, official-document, case-project, and general business diagram requests into AtlasFlow JSON IR.'
        }
      }
    ]
  };
}

function installPolicy() {
  return {
    platform: 'mac-arm64',
    requiredPlugins: [],
    requiredSkills: []
  };
}

function assertCatalogIsDownloadBacked(catalog) {
  const source = catalog.plugins?.[0]?.source || {};
  if (source.source !== 'download' || !source.url || !source.sha256) {
    throw new Error('Generated marketplace must use source: download with url and sha256.');
  }
}

function preparePackage(options) {
  const pluginRoot = path.resolve(options.pluginRoot);
  const manifest = readJson(path.join(pluginRoot, '.codex-plugin', 'plugin.json'));
  const pluginName = text(manifest.name);
  const version = text(manifest.version);
  const blockers = [
    pluginName !== PLUGIN_NAME ? `plugin name mismatch: ${pluginName || '(missing)'}` : '',
    !SEMVER_PATTERN.test(version) ? `version is not strict semver: ${version || '(missing)'}` : ''
  ].filter(Boolean);
  if (blockers.length) throw new Error(blockers.join('; '));

  const outRoot = path.resolve(options.outRoot || path.join(os.tmpdir(), `analytix-hub-atlasflow-${version}`));
  const archivePath = path.resolve(options.archivePath || path.join(os.tmpdir(), `${PLUGIN_NAME}-${version}.tar.gz`));
  assertInsideTmp(outRoot);
  if (fs.existsSync(outRoot)) {
    if (!options.force) throw new Error(`Output root already exists; pass --force to replace: ${outRoot}`);
    fs.rmSync(outRoot, { recursive: true, force: true });
  }
  fs.mkdirSync(path.join(outRoot, 'plugins'), { recursive: true });
  const destinationPluginRoot = path.join(outRoot, 'plugins', PLUGIN_NAME);
  copyPluginSource(pluginRoot, destinationPluginRoot);

  const archive = createArchive(outRoot, archivePath, options.force);
  const packageUrl = `${options.packageBaseUrl.replace(/\/+$/u, '')}/${path.basename(archivePath)}`;
  const generatedMarketplace = marketplace(manifest, packageUrl, archive);
  const generatedSkills = skillCatalog(manifest);
  const generatedInstallPolicy = installPolicy();
  assertCatalogIsDownloadBacked(generatedMarketplace);

  const hubRoot = path.join(outRoot, 'hub');
  writeJson(path.join(hubRoot, 'marketplace.json'), generatedMarketplace);
  writeJson(path.join(hubRoot, 'skills.json'), generatedSkills);
  writeJson(path.join(hubRoot, 'install-policy.json'), generatedInstallPolicy);
  writeJson(path.join(outRoot, '.agents', 'plugins', 'marketplace.json'), generatedMarketplace);

  if (options.writeHub) {
    writeJson(path.join(pluginRoot, 'hub', 'marketplace.json'), generatedMarketplace);
    writeJson(path.join(pluginRoot, 'hub', 'skills.json'), generatedSkills);
    writeJson(path.join(pluginRoot, 'hub', 'install-policy.json'), generatedInstallPolicy);
  }

  return {
    ok: true,
    plugin: `${PLUGIN_NAME}@${version}`,
    package_url: packageUrl,
    source_root: outRoot,
    plugin_root: destinationPluginRoot,
    hub_root: hubRoot,
    marketplace_path: path.join(hubRoot, 'marketplace.json'),
    skill_catalog_path: path.join(hubRoot, 'skills.json'),
    install_policy_path: path.join(hubRoot, 'install-policy.json'),
    archive,
    wrote_repo_hub_files: options.writeHub,
    boundaries: {
      archive_uses_download_source: true,
      checksum_required: true,
      writes_release_payload_under_tmp: true,
      runtime_cache_sync_path: false
    }
  };
}

try {
  const result = preparePackage(parseArgs(process.argv.slice(2)));
  if (result.ok) {
    if (process.argv.includes('--json')) console.log(JSON.stringify(result, null, 2));
    else {
      console.log(`Prepared ${result.plugin}`);
      console.log(`Archive: ${result.archive.path}`);
      console.log(`SHA-256: ${result.archive.sha256}`);
      console.log(`Hub files: ${result.hub_root}`);
      console.log(`Package URL: ${result.package_url}`);
    }
  }
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
}
