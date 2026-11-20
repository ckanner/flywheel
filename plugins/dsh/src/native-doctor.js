// Import-only availability checks. Do not instantiate an SDK, an Agent, LCU,
// a desktop client, or a model. Actual admission is owned by each invocation.
const loadModule = specifier => import(specifier, specifier.endsWith('/package.json') ? { with: { type: 'json' } } : {});

export async function nativeDoctor({ load = loadModule } = {}) {
  const composition = { package: 'dsh-preset-generator', version: null, protocol: null, available: false,
    presets: [], executor: 'ptc', namedPresetMounted: false };
  const lcu = { package: 'dsh-plugin-lcu', version: null, managedProtocol: null, available: false };
  try {
    const api = await load('dsh-preset-generator/composition');
    const manifest = (await load('dsh-preset-generator/package.json')).default;
    composition.version = manifest.version;
    composition.protocol = api.COMPOSITION_PROTOCOL;
    if (manifest.name !== composition.package || manifest.version !== '0.4.0' || api.COMPOSITION_PROTOCOL !== 1
        || typeof api.validateSdkPtcAdmission !== 'function' || typeof api.inspectPtcBundle !== 'function') throw new Error();
    for (const preset of ['daily', 'heavy']) {
      const value = api.createSdkPtcComposition({ runtimeProfile: 'sdk', preset });
      if (value.version !== 1 || value.runtimeProfile !== 'sdk' || value.requirements?.executor !== 'ptc'
          || value.provenance?.namedPresetMounted !== false || !/^[a-f0-9]{64}$/.test(value.digest)) throw new Error();
      composition.presets.push(preset);
    }
    composition.available = true;
  } catch { composition.available = false; }
  try {
    const api = await load('dsh-plugin-lcu/managed');
    const manifest = (await load('dsh-plugin-lcu/package.json')).default;
    lcu.version = manifest.version;
    if (manifest.name !== lcu.package || !['0.5.0', '0.5.1'].includes(manifest.version)
        || typeof api.attachManagedLcu !== 'function' || typeof api.ManagedLcuError !== 'function') throw new Error();
    lcu.managedProtocol = 1;
    lcu.available = true;
  } catch { lcu.available = false; }
  return { protocol: 1, type: 'native-doctor', harness: 'dsh', nativeVersion: 1,
    available: composition.available, composition, lcu, inferenceTested: false, admissionTested: false,
    nativeAdmission: 'requires-selected-sdk-runtime-and-effective-capability-probe' };
}
