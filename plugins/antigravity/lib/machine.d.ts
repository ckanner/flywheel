export const MACHINE_PROTOCOL_VERSION: 1
export const MACHINE_LIFECYCLE_VERSION: 1
export const MACHINE_ACTIVITY_VERSION: 1
export const MACHINE_CAPABILITIES: Readonly<Record<string, unknown>>
export const MACHINE_LIFECYCLE_CAPABILITIES: Readonly<{version:1;durableJournal:true;readOnlyRecovery:true;threadReadyBeforeDispatch:false}>
export const MACHINE_ACTIVITY_CAPABILITIES: Readonly<{version:1;metadataOnly:true;rootAndChildren:true}>
export class MachineInputError extends Error {}
export interface MachineRequest {
  protocol:1; requestId:string; cwd:string; prompt:string; command?:string;
  /** Exact agy models slug, including the effort suffix for adjustable models. */
  model:string; effort:'low'|'medium'|'high'|'xhigh';
  executionMode:'default'|'accept-edits'|'plan';
  permissionMode:'request-review'|'always-proceed';
  /** false means the native settings must also have terminal sandbox disabled. */
  sandbox:boolean;
  /** Creates a native custom agent; verifies init.agent and empty init.tools before sending any prompt. */
  toolPolicy?:'native'|'evidence-only';
  sessionMode:'new'|'resume'; sessionId?:string; sessionName?:string;
  timeoutMs?:number; disposeGraceMs?:number; maxOutputBytes?:number; maxFrameBytes?:number;
  outputSchema?:Record<string, unknown>;
  subscriptionPolicy:{version:1;mode:'subscription-only'};
}
export interface MachineLifecycleEvent {
  lifecycleVersion:1;type:'lifecycle';requestId:string;runId:string;sequence:number;
  event:'dispatch-intent'|'dispatch-possible'|'terminal'|'cleanup-confirmed';
  threadId:string|null;turnId:null;persistent:true;configuration:MachineReceipt['configuration']|null;
  dispatch:'not-dispatched'|'possible'|'accepted';outcome:'unknown'|'completed'|'failed'|'cancelled';cleanupConfirmed:boolean;
}
export interface MachineRuntime {
  env?:Record<string,string|undefined>;signal?:AbortSignal;
  onProgress?:(text:string)=>void;
  onActivity?:(event:{version:1;sequence:number;kind:'provider-progress';source:'antigravity';scope:'root'|'child';eventTypes:string[];observedEvents:number})=>void;
  onLifecycle?:(event:MachineLifecycleEvent)=>void|Promise<void>;
  /** An unused directory below an existing canonical private parent. Required for durable recovery. */
  recoveryDirectory?:string;
}
export interface MachineReceipt {
  protocol:1;type:'result';requestId:string;runId:string;ok:boolean;
  requested:Record<string,unknown>;
  execution:{pid:number|null;processGroupId:number|null;ownership:'process-group'|'none';dispatch:'not-dispatched'|'dispatched';turnDispatch:'not-dispatched'|'possible'|'accepted';outcome:'not-started'|'completed'|'failed'|'unknown';terminalReceived:boolean;exitCode:number|null;signal:string|null;cancelled:boolean;timedOut:boolean;truncated:boolean};
  cleanup:{confirmed:boolean;method:'not-started'|'process-group'};
  providerWork:{state:'not-dispatched'|'completed'|'failed'|'unknown';querySupported:false;cancelSupported:false;replaySafe:boolean};
  session:{mode:'new'|'resume';id:string|null;resumed:boolean;resumable:boolean;name:string|null;nativeNameStatus:'unknown'};
  configuration:{source:'cli-init'|null;observed:{cwd:string|null;model:string|null;permissionMode:string|null;agent:string|null;tools:string[]|null;effort:null;executionMode:null;sandbox:null;cliVersion:string|null}|null;matched:boolean;fingerprint:string|null};
  subscription:{mode:'subscription-only';status:'verified-local-controls'|'unknown';authentication:'native-account-unverified';creditsFallback:false;extraBilling:'unknown';configFingerprint:string|null};
  output:Array<{type:'text';text:string}>;structured?:unknown;
  usage:{status:'complete'|'partial'|'unknown';scope:'conversation'|'unknown';inputTokens:number|null;outputTokens:number|null;thinkingTokens:number|null;cacheReadTokens:number|null;cacheCreationTokens:null;totalTokens:number|null};
  native:{children:Array<Record<string,unknown>>;aggregateUsageStatus:'unknown';artifacts:[];managedAgent?:{name:string;path:string;digest:string;ownerRunId:string;cleanup:'confirmed'|'unknown'}};
  reportedCostUSD:null;costUSD:null;costStatus:'unknown';
  failure:null|{stage:string;category:string;detail:string};
  recovery:MachineRecoveryRequest;
}
export function parseMachineRequest(input:unknown):Readonly<MachineRequest>
export function runMachine(input:MachineRequest,runtime?:MachineRuntime):Promise<MachineReceipt>
export interface MachineRecoveryRequest {protocol:1;requestId:string;runId:string;recoveryDirectory:string}
export function recoverMachine(input:MachineRecoveryRequest):Promise<{protocol:1;type:'recovery';requestId:string;runId:string;status:'terminal'|'pending'|'unknown';receipt:MachineReceipt|null;lifecycle:MachineLifecycleEvent|null;reason:string}>
/** Runs only --version, --help and models. Does not test inference, login or remaining quota. */
export function doctor(input:{command?:string;cwd:string;model?:string;effort?:string},runtime?:Pick<MachineRuntime,'env'|'signal'>):Promise<Record<string,unknown>>
