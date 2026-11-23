# Third-party notices

## DeepSeek Harness UI patterns

Flywheel's plugin-owned Button, Input, and ongoing StateDot treatments are
adapted from DeepSeek Harness at commit
`b830ec950faba0a46dda564be3e2d854afed94f1`:

- `packages/client/ui-primitives/src/Button.tsx` and its stylesheet
- `packages/client/ui-primitives/src/Input.tsx` and its stylesheet
- `packages/client/ui-primitives/src/StateDot.tsx` and its stylesheet

[Upstream source](https://github.com/deepseek-ai/deepseek-harness/tree/b830ec950faba0a46dda564be3e2d854afed94f1/packages/client/ui-primitives/src)

These adapted portions retain the upstream MIT license below. Flywheel's own
source is also licensed under MIT; see [LICENSE](LICENSE).

MIT License

Copyright (c) 2026 DeepSeek

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## Bundled Codex Pro public package

This distribution bundles `dsh-subagent-codex-pro@0.10.2`, Copyright (c) 2026 ckanner, under the MIT License. Its complete license is retained at `node_modules/dsh-subagent-codex-pro/LICENSE` inside the package. Source identity and tarball integrity are recorded in the [vendor index](vendor/README.md) and [Codex record](vendor/codex-pro.json). The bundled package is the upstream public machine API implementation, including its compiled supervisor; it contains no native Codex executable or user credentials.

## Bundled PTC composition source

This distribution also bundles `dsh-preset-generator@0.4.0`, Copyright (c) 2026 ckanner, under the MIT License. Its original license remains at `node_modules/dsh-preset-generator/LICENSE`. The public composition module shares the desktop daily/heavy policy with the owned SDK worker. Source identity and actual tarball integrity are recorded in the [generator record](vendor/preset-generator.json); no DSH application resources, native executable or credentials are bundled.

## Bundled Antigravity CLI adapter

This distribution bundles `@ckanner/dsh-subagent-antigravity@0.3.0`, Copyright (c) 2026
ckanner, under the MIT License. Its license remains at
`node_modules/@ckanner/dsh-subagent-antigravity/LICENSE`. Exact source and archive identities
are in the [Antigravity record](vendor/antigravity-acp.json); the complete adapter source is in
the standalone [dsh-subagent-antigravity repository](https://github.com/ckanner/dsh-subagent-antigravity). Its pinned `ajv@8.17.1` schema validator and transitive
runtime dependencies retain their licenses in the bundled dependency tree. No native
agy executable, account credentials or private Google application resources are included.

## Go image decoding and text support

The native engines include `golang.org/x/image@v0.48.0` for bounded WebP input decoding
and `golang.org/x/text@v0.44.0` as a dependency. Both modules use the BSD 3-Clause
license reproduced below; their exact module checksums are retained in `go.sum`.

Copyright 2009 The Go Authors.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google LLC nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

## Bundled Agent Client Protocol SDK

The Antigravity companion includes `@agentclientprotocol/sdk@2.9.1`, copyright
Zed Industries, under Apache-2.0. Its source is
[agentclientprotocol/typescript-sdk](https://github.com/agentclientprotocol/typescript-sdk/tree/v2.9.1).
The complete upstream license is shipped in the installed SDK's `LICENSE` file.
The companion also uses the pinned MIT-licensed Zod 4.3.6 schema library already
used by Flywheel. Google Antigravity executable bytes are verified against the
companion's provenance manifest and are not distributed inside this package.
