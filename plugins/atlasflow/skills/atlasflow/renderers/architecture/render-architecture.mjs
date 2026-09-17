import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { loadDiagram, writeDiagram } from '../shared/cli.mjs';
import { renderArchitecturePlan } from './render-plan.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const layoutJsonMode = process.argv.includes('--layout-json');
const cliArgs = process.argv.filter((arg) => arg !== '--layout-json');
const { diagram: arch, template, outPath } = loadDiagram({
  rendererDir: __dirname,
  diagramType: 'architecture',
  defaultExample: 'web-app.architecture.json',
  argv: cliArgs,
});

const rendered = renderArchitecturePlan(arch);
if (layoutJsonMode) {
  console.log(JSON.stringify(rendered.layout, null, 2));
  process.exit(0);
}
writeDiagram({ outPath, template, meta: arch.meta, footerLabel: 'Architecture diagram', svg: rendered.svg, cards: arch.cards });
