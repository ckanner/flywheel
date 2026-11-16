export const designRef = { revision: 2, digest: 'b'.repeat(64) };
export const previousRef = { revision: 1, digest: 'a'.repeat(64) };
export function planningState(patch = {}) {
  return {
    version: 1, phase: 'awaiting-design-approval', brief: { version: 1, id: 'migration', goal: 'Design a recoverable queue', policy: 'review-first' },
    policy: 'review-first', 'input-revision': 2, 'max-rounds': 3, rounds: 2,
    requests: [{ id: 'design-request', scope: 'design', 'scope-id': '', rounds: 2, 'max-rounds': 3, status: 'reviewed' }],
    designs: [
      { ref: previousRef, 'input-revision': 1, document: '# Previous design\nA queue without a checkpoint.', requirements: [], decisions: [], milestones: [] },
      { ref: designRef, 'input-revision': 2, document: '# Recoverable queue\nPersist the checkpoint before acknowledgement.', requirements: [{ id: 'R-01', title: 'Recovery retains work', milestone: 'M01', verification: 'queue test' }], decisions: [{ id: 'D-01', kind: 'fixed', statement: 'Persist before acknowledgement', rationale: 'Preserve recoverability' }], milestones: [] },
    ], plans: [], reviews: [{ id: 'review-2', kind: 'design', target: designRef, verdict: 'pass', summary: 'Checkpoint closes the loss window', role: 'design-review', binding: 'loops.design-review', invocation: 'independent-review', findings: [{ id: 'F-01', severity: 'major', target: 'checkpoint', evidence: 'The previous acknowledgement preceded persistence', impact: 'A restart could lose work', status: 'resolved', resolution: 'Checkpoint now precedes acknowledgement' }] }],
    approvals: [], feedback: [], 'current-design': designRef, 'current-plans': {}, 'start-authorized': false, ...patch,
  };
}
export function planningTask(planning = planningState(), revision = 7) {
  return { revision, status: 'paused', packets: {}, planning };
}
