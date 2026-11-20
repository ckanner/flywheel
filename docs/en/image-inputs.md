# Frozen image inputs

[Documentation](../README.md) · English · [简体中文](../zh-CN/image-inputs.md)

Use a packet's optional `input-images` field when implementation needs a screenshot,
diagram or other visual reference. Flywheel reads the declared files, validates the
frozen bytes, and sends native image content blocks through the Claude Code Pro
public machine API and official Agent SDK. The default `multimodal` route selects
Claude. A path in `context-files` or ordinary prompt text does not create an image
attachment.

## Declare the actual files

Save the authorized reference as a regular file in the task's repository checkout,
for example `references/layout.png`. Keep it available for the whole packet,
including retries, verification and pending-result recovery. Compute its SHA-256:

```bash
sha256sum references/layout.png
# macOS: shasum -a 256 references/layout.png
```

Add the following field to the packet frontmatter and replace the checksum
placeholder with that command's **actual 64-character lowercase hexadecimal**
result. The path, checksum and media type are part of the frozen contract; use
the normal reviewed contract preparation or revision path.

```yaml
route: multimodal
input-images:
  version: 1
  images:
    - path: references/layout.png
      sha256: "<actual lowercase SHA-256 of references/layout.png>"
      media-type: image/png
```

`version` and `images` are required when `input-images` is present. Every image
requires exactly `path`, `sha256` and `media-type`. Omit the whole field for a
text-only packet; an empty array or a null field is invalid. The order of this
array is the order of the native image blocks. Execute's text prompt also names
the ordered frozen references, so instructions can identify “the first image”
without pretending that a filename is visual content.

Paths use `/` and are relative to the **task's original repository checkout**
(`--cwd`), independently of a packet worktree or an explicitly selected execution
checkout. Absolute paths, `..`, non-normalized paths, backslashes, drive/URI
prefixes, control characters, `.git` components and duplicate paths are rejected.
Every path component must be free of symbolic links. Directories, empty files,
FIFOs and other special files are rejected. Flywheel confines reads to that
checkout and bounds the read even if a file changes while being opened.

The contract does not import files from a chat or clipboard. Copy the actual
authorized bytes into the selected repository before declaring the reference;
do not invent a checksum or replace an unavailable attachment with a textual
description.

## Limits and format verification

| Boundary | Limit |
|---|---:|
| Images per packet | 1–4 |
| Bytes per image | 4 MiB |
| Combined image bytes | 8 MiB |
| Width or height | 8,192 pixels |
| Width × height | 16,000,000 pixels per image |
| Claude prompt | 1 MiB |
| Complete Claude JSONL request, including newline | 16 MiB |

Byte limits apply to the actual file bytes, before base64 encoding. PNG, JPEG,
GIF and WebP use `image/png`, `image/jpeg`, `image/gif` and `image/webp`, respectively.
Flywheel checks the image header and dimensions before decoding the raster, then
requires the actual decoded format to match the declaration. Renaming a text file
to `.png` or changing its MIME label does not pass. The GIF decoder validates its
first raster frame; use a static image when the requirement depends on a specific
frame or on the contents of an animation.

The worker checks image count, canonical base64 and the same byte limits again.
The final encoded request must fit the 16 MiB frame, including JSON escaping of
the text prompt. That limit applies only to Claude: other workers retain their
existing 1 MiB request limits. Reduce image resolution or compress the original
file before freezing its checksum if necessary; Flywheel does not silently resize,
recompress, truncate or omit an attachment.

## Execute, Review and permissions

Only **Execute** receives these image content blocks. Its current binding must use
`claude-code-pro`, whether selected directly, by a profile or by an execution
policy. The default `multimodal` route supplies that binding. A different route
can be used if it explicitly selects the same supported harness. Unsupported
backends, including an incompatible fallback destination, stop before dispatch;
they do not receive a text-only substitute.

The [Claude prerequisites](claude-code-pro.md) still apply: matching companion,
existing native login, subscription admission when configured, and explicit
`danger-full-access` permission. Declaring an image changes none of these choices.
Images are sent only after the same-process subscription checks allow the task
prompt.

Plan, independent Review and Judge do not automatically receive image bytes.
Review receives the frozen reference metadata, implementation and test evidence,
with an explicit instruction not to claim native visual inspection. If a required
visual property lacks a reproducible check or other verifiable evidence, Review
must return `verification-gap`. Passing code tests alone does not prove visual
fidelity. Put necessary rendering, screenshot comparison or human visual acceptance
requirements into the actual task contract.

## Frozen evidence and safe recovery

Before dispatch, Flywheel verifies SHA-256 and image validity against the packet.
It rechecks the original source when checkpointing Execute's result, before
reusing verification evidence, and before applying a recorded terminal result.
A changed, missing or invalid source prevents that result from becoming current
acceptance evidence, even if a packet worktree contains an older copy.

Changing any declared image path, checksum, media type or array order changes the
goal/contract identity. A reviewed revision therefore rebuilds the affected role
history; it cannot silently resume a session that saw different images. Model,
effort and effective binding changes retain their existing history boundaries.

Invocation state and receipts retain a versioned manifest of each image's MIME
type, SHA-256 and byte count. They do not retain original image bytes or base64.
The public companion computes the same manifest from the bytes actually sent;
Flywheel rejects a missing or different manifest in both normal and recovered
receipts. The native CLI may retain the image in its authorized session history.

An unresolved invocation is reconciled **before** reading a new source or
dispatching another prompt. Public recovery queries only the original request,
run and private journal using its original settings; the query contains no image
bytes or new prompt. Original image metadata remains local to receipt validation.
Known usage can be accounted once even when changed source files make the old
output unusable. Preserve the original task and recovery evidence; switching
models, changing images or selecting fresh history cannot clear an unknown call.

## Offline end-to-end verification

The source regression below runs packet loading and Go Execute through the built
worker, pinned public companion and real Agent SDK. A fake native CLI receives the
actual PNG block and only then writes an artifact and a valid done-report. It
also tests lost Go receipts, original read-only recovery and changed-source
rejection without a second prompt:

```bash
FLYWHEEL_TEST_NODE_BRIDGE=1 go test ./internal/machine \
  -run '^TestPacketImagesClaudeSDKOfflineBridge$' -count=1
```

This test requires the built DSH worker and its exact reviewed Claude companion.
It proves transport, ownership, accounting and recovery behavior without an
account or a model inference; it does not establish a model's visual accuracy.
