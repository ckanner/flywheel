import type { ClientContext, FlywheelConsoleRemote, RemoteEnvelope } from './context.ts';
import type { ControlResponse, ModelCatalogReport } from './types.ts';

const PACKAGE = '@ckanner/flywheel';
const REMOTE_NAMESPACE = 'flywheelConsole';
const REMOTE_SERVICE_KEY = 'flywheelConsole';
const RESULT_CODEC = { mode: 'src-json' as const };

const QUERY_ACTIONS = new Set([
  'models', 'images', 'runs', 'status', 'config', 'config-edit',
  'metrics', 'history', 'node', 'delivery', 'doctor', 'planning',
  'design-candidates', 'ux-review', 'ui-prototype', 'read-artifact', 'traces', 'trace-detail',
]);

const ACT_ACTIONS = new Set([
  'planning', 'investigate', 'launch', 'start', 'stop', 'step',
  'config-save', 'session', 'packet', 'deliver', 'open-terminal', 'open-artifact', 'save-attachment',
]);

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function validateInputObject(value: unknown, allowedActions: Set<string>, label: string): Record<string, unknown> {
  if (!isPlainObject(value) || typeof value.action !== 'string' || !allowedActions.has(value.action)) {
    throw new Error(`Invalid ${label} request`);
  }
  return value;
}

function validateOptionalPath(value: unknown): string | undefined {
  if (value === undefined) return undefined;
  if (typeof value !== 'string' || value.trim() === '' || value.includes('\0')) {
    throw new Error('Expected a non-empty directory path');
  }
  return value;
}

function parameterCodec(
  typeSymbol: string,
  validator: (value: unknown) => unknown,
): { mode: 'strict'; typeSymbol: string; create: () => { parse: (value: unknown) => unknown } } {
  return {
    mode: 'strict',
    typeSymbol,
    create: () => ({ parse: validator }),
  };
}

export interface RemoteDescriptor {
  id: string;
  service: string;
  namespace: string;
  method: string;
  invocation: { kind: 'direct' };
  parameters: Array<{
    name: string;
    wire: string;
    source: 'json';
    codec: { mode: 'strict'; typeSymbol: string; create: () => { parse: (value: unknown) => unknown } };
    acceptsUndefined?: true;
  }>;
  result: { mode: 'src-json' };
  sourceLocation: { file: string; line: number; column: number };
}

export interface RemoteContribution {
  package: string;
  descriptors: RemoteDescriptor[];
}

/**
 * Hand-written Direct Typert Remote contribution for `flywheelConsole`.
 * Works across all 4 native DSH slots without requiring an active chat session.
 */
export function remoteContribution(): RemoteContribution {
  return {
    package: PACKAGE,
    descriptors: [
      {
        id: `${PACKAGE}#${REMOTE_NAMESPACE}/query`,
        service: REMOTE_SERVICE_KEY,
        namespace: REMOTE_NAMESPACE,
        method: 'query',
        invocation: { kind: 'direct' },
        parameters: [
          {
            name: 'input',
            wire: 'input',
            source: 'json',
            codec: parameterCodec(
              `${PACKAGE}#${REMOTE_NAMESPACE}/query:input`,
              v => validateInputObject(v, QUERY_ACTIONS, 'flywheelConsole.query'),
            ),
          },
        ],
        result: RESULT_CODEC,
        sourceLocation: { file: 'src/console-read.js', line: 172, column: 3 },
      },
      {
        id: `${PACKAGE}#${REMOTE_NAMESPACE}/act`,
        service: REMOTE_SERVICE_KEY,
        namespace: REMOTE_NAMESPACE,
        method: 'act',
        invocation: { kind: 'direct' },
        parameters: [
          {
            name: 'input',
            wire: 'input',
            source: 'json',
            codec: parameterCodec(
              `${PACKAGE}#${REMOTE_NAMESPACE}/act:input`,
              v => validateInputObject(v, ACT_ACTIONS, 'flywheelConsole.act'),
            ),
          },
        ],
        result: RESULT_CODEC,
        sourceLocation: { file: 'src/console-read.js', line: 197, column: 3 },
      },
      {
        id: `${PACKAGE}#${REMOTE_NAMESPACE}/listRepoImages`,
        service: REMOTE_SERVICE_KEY,
        namespace: REMOTE_NAMESPACE,
        method: 'listRepoImages',
        invocation: { kind: 'direct' },
        parameters: [
          {
            name: 'cwd',
            wire: 'cwd',
            source: 'json',
            codec: parameterCodec(`${PACKAGE}#${REMOTE_NAMESPACE}/listRepoImages:cwd`, validateOptionalPath),
            acceptsUndefined: true,
          },
        ],
        result: RESULT_CODEC,
        sourceLocation: { file: 'src/console-read.js', line: 245, column: 3 },
      },
    ],
  };
}

export class FlywheelRemoteError extends Error {
  readonly code: string;
  readonly dispatch?: string;
  readonly action?: string | null;

  constructor(message: string, opts?: { code?: string; dispatch?: string; action?: string | null }) {
    super(message);
    this.name = 'FlywheelRemoteError';
    this.code = opts?.code ?? 'REMOTE_ERROR';
    this.dispatch = opts?.dispatch;
    this.action = opts?.action;
  }
}

function unwrapEnvelope(raw: RemoteEnvelope | undefined): ControlResponse {
  if (!raw) throw new FlywheelRemoteError('Empty response from flywheelConsole');
  if (raw.ok === false) {
    const msg = typeof raw.error === 'string' ? raw.error : raw.error?.message ?? 'RPC call failed';
    throw new FlywheelRemoteError(msg, { code: 'RPC_FAILED', dispatch: 'unknown' });
  }
  const result = raw.value?.result ?? raw.result;
  if (!result || typeof result.text !== 'string') {
    throw new FlywheelRemoteError('Malformed flywheelConsole RPC envelope');
  }
  let parsed: ControlResponse;
  try {
    parsed = JSON.parse(result.text) as ControlResponse;
  } catch {
    throw new FlywheelRemoteError('Invalid JSON payload from flywheelConsole');
  }
  if (result.kind === 'error' || parsed.type === 'flywheel-control-error') {
    throw new FlywheelRemoteError(parsed.message ?? 'Flywheel control error', {
      code: 'CONTROL_ERROR',
      dispatch: parsed.dispatch,
      action: parsed.action,
    });
  }
  return parsed;
}

const MODEL_ENTRY_KEYS = new Set(['id', 'label', 'efforts', 'defaultEffort']);

/**
 * Validates that a model catalog response strictly honors requested harness/channel
 * and never invents entitlement or inference proof.
 */
export function validateModelCatalog(
  report: unknown,
  request: { version: number; harness: string; channel?: string; provider?: string },
): ModelCatalogReport {
  if (!isPlainObject(report)) throw new Error('invalid model catalog: expected object');
  if (report.harness !== request.harness) throw new Error('invalid model catalog: harness mismatch');
  if (request.channel !== undefined && report.channel !== request.channel) {
    throw new Error('invalid model catalog: channel mismatch');
  }
  if (request.provider !== undefined && report.provider !== request.provider) {
    throw new Error('invalid model catalog: provider mismatch');
  }
  if (report.inferencePerformed !== false) {
    throw new Error('invalid model catalog: inferencePerformed must be false');
  }
  if (report.accountEntitlement !== undefined && report.accountEntitlement !== 'unknown') {
    throw new Error('invalid model catalog: accountEntitlement cannot claim confirmed entitlement');
  }
  if (!Array.isArray(report.models)) throw new Error('invalid model catalog: models must be an array');
  for (const entry of report.models) {
    if (!isPlainObject(entry) || typeof entry.id !== 'string' || entry.id.trim() === '') {
      throw new Error('invalid model catalog: malformed model entry');
    }
    for (const key of Object.keys(entry)) {
      if (!MODEL_ENTRY_KEYS.has(key)) {
        throw new Error(`invalid model catalog: unexpected model field ${key}`);
      }
    }
  }
  return report as unknown as ModelCatalogReport;
}

export class FlywheelRemoteClient {
  private readonly ctx: ClientContext;
  private readonly ready: Promise<string | undefined>;

  constructor(ctx: ClientContext, ready: Promise<string | undefined>) {
    this.ctx = ctx;
    this.ready = ready;
  }

  private async getNamespace(): Promise<FlywheelConsoleRemote> {
    const mountError = await this.ready;
    if (mountError) {
      throw new FlywheelRemoteError(`Failed to mount flywheelConsole remote: ${mountError}`, {
        code: 'MOUNT_FAILED',
        dispatch: 'not-dispatched',
      });
    }
    // Use ctx.get('remote.flywheelConsole') first: Cordis throws on undeclared
    // nested proxy access (`ctx.remote.flywheelConsole`) unless injected.
    const ns =
      (typeof this.ctx.get === 'function'
        ? this.ctx.get<FlywheelConsoleRemote>('remote.flywheelConsole')
        : undefined) ??
      (this.ctx.remote && 'flywheelConsole' in this.ctx.remote
        ? this.ctx.remote.flywheelConsole
        : undefined);
    if (!ns) {
      throw new FlywheelRemoteError('flywheelConsole remote namespace is unavailable', {
        code: 'UNAVAILABLE',
        dispatch: 'not-dispatched',
      });
    }
    return ns;
  }

  async query(input: Record<string, unknown>): Promise<ControlResponse> {
    const ns = await this.getNamespace();
    const res = await ns.query(input);
    const parsed = unwrapEnvelope(res);
    if (input.action === 'models' && isPlainObject(input.models)) {
      validateModelCatalog(
        parsed.modelCatalog,
        input.models as { version: number; harness: string; channel?: string; provider?: string },
      );
    }
    return parsed;
  }

  async act(input: Record<string, unknown>): Promise<ControlResponse> {
    const ns = await this.getNamespace();
    const res = await ns.act(input);
    return unwrapEnvelope(res);
  }

  async listRepoImages(cwd?: string): Promise<ControlResponse> {
    const ns = await this.getNamespace();
    const res = await ns.listRepoImages(cwd);
    return unwrapEnvelope(res);
  }
}
