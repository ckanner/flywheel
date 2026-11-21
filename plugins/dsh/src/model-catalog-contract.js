// Shared by the host bridge and browser. Pure data validation only; this file
// must not import a runtime, account provider, filesystem or process module.
export function validModelCatalogRequest(value) {
  const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
  const identifier = value => typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9._:/-]{0,255}$/.test(value);
  return object(value) && Object.keys(value).every(key => ['version', 'harness', 'provider', 'channel'].includes(key)) &&
    value.version === 1 && identifier(value.harness) &&
    (value.provider === undefined || identifier(value.provider)) && (value.channel === undefined || identifier(value.channel));
}

export function validModelCatalogReport(value, request) {
  const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
  const identifier = value => typeof value === 'string' && /^[A-Za-z0-9][A-Za-z0-9._:/-]{0,255}$/.test(value);
  const text = (value, limit) => typeof value === 'string' && value.trim() !== '' && value.length <= limit && !/[\p{Cc}\p{Cf}]/u.test(value);
  const sources = { antigravity: 'agy models', 'codex-pro': 'codex app-server model/list', 'claude-code-pro': 'Claude Agent SDK initialization.models' };
  const source = request && Object.hasOwn(sources, request.harness) ? sources[request.harness] : 'unsupported';
  const efforts = ['', 'none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'];
  if (!validModelCatalogRequest(request) || !object(value) ||
      Object.keys(value).some(key => !['version', 'harness', 'provider', 'channel', 'status', 'source', 'scope', 'models', 'truncated', 'inferencePerformed', 'accountEntitlement', 'error', 'limitations'].includes(key)) ||
      ['version', 'harness', 'provider', 'channel'].some(key => value[key] !== request[key]) ||
      value.source !== source || value.scope !== 'native-client-catalog' ||
      value.source === 'unsupported' && value.status !== 'unsupported' ||
      !['available', 'unavailable', 'unsupported'].includes(value.status) || value.inferencePerformed !== false ||
      value.accountEntitlement !== 'unknown' || typeof value.truncated !== 'boolean' ||
      !Array.isArray(value.models) || value.models.length > 256 || value.status !== 'available' && value.models.length !== 0 ||
      !Array.isArray(value.limitations) || value.limitations.length > 16 || value.limitations.some(item => !text(item, 1024)) ||
      value.error !== undefined && (!object(value.error) || Object.keys(value.error).some(key => !['code', 'message'].includes(key)) ||
        !identifier(value.error.code) || !text(value.error.message, 1024)) || value.status === 'available' && value.error !== undefined) return false;
  const seen = new Set();
  return value.models.every(model => {
    if (!object(model) || Object.keys(model).some(key => !['id', 'label', 'description', 'efforts', 'defaultEffort'].includes(key)) ||
        !identifier(model.id) || seen.has(model.id) || !text(model.label, 256) || model.description !== undefined && !text(model.description, 1024) ||
        model.efforts !== undefined && (!Array.isArray(model.efforts) || model.efforts.length > 10 || new Set(model.efforts).size !== model.efforts.length || model.efforts.some(item => !efforts.includes(item))) ||
        model.defaultEffort !== undefined && !model.efforts?.includes(model.defaultEffort)) return false;
    seen.add(model.id);
    return true;
  });
}
