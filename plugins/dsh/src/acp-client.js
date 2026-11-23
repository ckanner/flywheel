// The companion's public source is bundled by build.mjs. It owns the same ACP
// UI and Remote contract in standalone DSH and in the complete Flywheel install.
const flywheelAcpClient = createAntigravityAcpClient(require);
const applyFlywheelTaskClient = exports.apply;
exports.inject = [...new Set([...exports.inject, ...flywheelAcpClient.inject])];
exports.apply = function applyFlywheelWithAcp(ctx) {
  const result = applyFlywheelTaskClient(ctx);
  flywheelAcpClient.apply(ctx, {
    viewId: 'flywheel-antigravity', order: 1001,
    packageName: '@ckanner/dsh-subagent-antigravity',
  });
  return result;
};
