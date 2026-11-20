export function validImageSelection(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value) &&
    Object.keys(value).length === 1 && Array.isArray(value.paths) && value.paths.length >= 1 && value.paths.length <= 4 &&
    new Set(value.paths).size === value.paths.length && value.paths.every(path => typeof path === 'string' &&
      path.length > 0 && path.length <= 512 && !path.startsWith('/') && !/[\\:\p{C}]/u.test(path) &&
      path.split('/').every(part => part && !['.', '..', '.git'].includes(part)));
}

export function validImageInspection(value, paths) {
  const refs = value?.references, previews = value?.previews;
  return refs?.version === 1 && Array.isArray(refs.images) && refs.images.length === paths.length &&
    Array.isArray(previews) && previews.length === paths.length && refs.images.every((ref, index) => {
      const image = previews[index];
      return ref?.path === paths[index] && /^[a-f0-9]{64}$/.test(ref.sha256) &&
        ['image/png', 'image/jpeg', 'image/gif', 'image/webp'].includes(ref['media-type']) &&
        image?.path === ref.path && image.sha256 === ref.sha256 && image.mimeType === ref['media-type'] &&
        Number.isSafeInteger(image.bytes) && image.bytes > 0 && image.bytes <= 4194304 &&
        ['width', 'height'].every(key => Number.isSafeInteger(image[key]) && image[key] > 0 && image[key] <= 8192) &&
        image.width * image.height <= 16000000 && typeof image.dataUrl === 'string' && image.dataUrl.length <= 150000 &&
        /^data:image\/png;base64,[A-Za-z0-9+/]+={0,2}$/.test(image.dataUrl);
    }) && previews.reduce((total, image) => total + image.bytes, 0) <= 8388608;
}
