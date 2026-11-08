# Third-party notices

## DeepSeek Harness UI patterns

Flywheel's plugin-owned Button, Input, and ongoing StateDot treatments are
adapted from DeepSeek Harness at commit
`b830ec950faba0a46dda564be3e2d854afed94f1`:

- `packages/client/ui-primitives/src/Button.tsx` and its stylesheet
- `packages/client/ui-primitives/src/Input.tsx` and its stylesheet
- `packages/client/ui-primitives/src/StateDot.tsx` and its stylesheet

[Upstream source](https://github.com/deepseek-ai/deepseek-harness/tree/b830ec950faba0a46dda564be3e2d854afed94f1/packages/client/ui-primitives/src)

These adapted portions retain the upstream MIT license below. Other Flywheel
source remains under the repository's existing license.

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

This distribution bundles `dsh-subagent-codex-pro@0.9.0`, Copyright (c) 2026 ckanner, under the MIT License. Its complete license is retained at `node_modules/dsh-subagent-codex-pro/LICENSE` inside the packed plugin. Source identity and tarball integrity are recorded in `vendor/README.md` and `vendor/codex-pro.json` in the source repository. The bundled package is the upstream public machine API implementation, including its compiled supervisor; it contains no native Codex executable or user credentials.
