import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const schemasDir = path.resolve(__dirname, '../../schemas');

const validators = new Map();

try {
  const { default: Ajv2020 } = await import('ajv/dist/2020.js');
  const ajv = new Ajv2020({ allErrors: true, strict: true });
  const commonPath = path.join(schemasDir, 'common.schema.json');
  if (fs.existsSync(commonPath)) {
    ajv.addSchema(JSON.parse(fs.readFileSync(commonPath, 'utf8')));
  }
  for (const type of ['workflow', 'sequence', 'dataflow', 'lifecycle', 'architecture']) {
    const schema = JSON.parse(fs.readFileSync(path.join(schemasDir, `${type}.schema.json`), 'utf8'));
    validators.set(type, ajv.compile(schema));
  }
} catch (err) {
  if (err && err.code === 'ERR_MODULE_NOT_FOUND') {
    throw new Error(
      'atlasflow: schema validation is required; the validator dependency is unavailable. '
      + 'Run "npm ci" in the skill folder before rendering or validating diagrams.'
    );
  } else {
    throw err;
  }
}

// "/nodes/3/label" reads much better as "/nodes/3 (id: "router") /label" for the
// LLM fixing the JSON; resolve the nearest enclosing element's id or label.
function annotatePath(instancePath, data) {
  if (!instancePath) return '/';
  let node = data;
  let hint = null;
  for (const seg of instancePath.split('/').slice(1)) {
    if (node == null || typeof node !== 'object') break;
    node = node[/^\d+$/.test(seg) ? Number(seg) : seg];
    if (node && typeof node === 'object' && !Array.isArray(node)) {
      const tag = node.id ?? node.label;
      if (tag != null) hint = String(tag);
    }
  }
  return hint != null ? `${instancePath} (id/label: ${JSON.stringify(hint)})` : instancePath;
}

function formatErrors(errors, data) {
  return errors.map((e) => {
    const where = annotatePath(e.instancePath, data);
    const detail = e.params && Object.keys(e.params).length
      ? ' ' + JSON.stringify(e.params)
      : '';
    return `  ${where} ${e.message}${detail}`;
  }).join('\n');
}

export function validateSchema(diagramType, data) {
  const validate = validators.get(diagramType);
  if (!validate) {
    throw new Error(`validateSchema: unknown diagram type "${diagramType}"`);
  }
  if (!validate(data)) {
    throw new Error(
      `${diagramType} schema validation failed:\n${formatErrors(validate.errors, data)}`
    );
  }
}
