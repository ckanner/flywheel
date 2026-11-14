import assert from 'node:assert/strict';
import { crc32, inflateSync } from 'node:zlib';

export const TEST_PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAC0lEQVR4nGNgAAIAAAUAAXpeqz8AAAAASUVORK5CYII=', 'base64');
export const NORMALIZED_PNG = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4z8DwHwAFAAH/iZk9HQAAAABJRU5ErkJggg==', 'base64');

// Validate the actual small PNG fixtures before publishing an attachment ref.
// This catches corrupt IDAT/CRC literals without trusting a file extension.
export function validatePng(data) {
  assert.deepEqual(data.subarray(0, 8), Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]));
  let offset = 8, ended = false;
  const encoded = [];
  while (offset < data.length) {
    const size = data.readUInt32BE(offset);
    const body = data.subarray(offset + 4, offset + 8 + size);
    assert.equal(crc32(body), data.readUInt32BE(offset + 8 + size), 'PNG chunk CRC');
    const kind = body.subarray(0, 4).toString();
    if (kind === 'IHDR') { assert.equal(body.readUInt32BE(4), 1); assert.equal(body.readUInt32BE(8), 1); }
    if (kind === 'IDAT') encoded.push(body.subarray(4));
    if (kind === 'IEND') ended = true;
    offset += size + 12;
  }
  assert.equal(offset, data.length); assert.ok(ended);
  assert.equal(inflateSync(Buffer.concat(encoded)).length, 5);
}
