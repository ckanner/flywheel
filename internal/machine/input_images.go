package machine

import (
	"fmt"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/imageinput"
)

// Called only after pending invocation reconciliation. A new path/checksum may
// never become a replacement prompt for an unresolved native request.
func (c *Context) resolveInvocationImages(role, scope string, args *drivers.CallArgs) error {
	if len(args.Images) != 0 {
		return fmt.Errorf("managed image inputs must come from the frozen packet, not caller-supplied bytes")
	}
	packet := c.Packets[scope]
	if role != "execute" || packet == nil || packet.InputImages == nil {
		return nil
	}
	if args.Binding.Harness != "claude-code-pro" {
		return fmt.Errorf("packet %s declares input-images but harness %q cannot accept them; select the Claude multimodal route", packet.PacketID, args.Binding.Harness)
	}
	images, err := imageinput.Load(c.Cwd, packet.InputImages)
	if err != nil {
		return err
	}
	args.Images = images
	return nil
}

// The immutable input is checked before and after Execute, before reusing
// verification, and before replaying a terminal result. A source image in the
// main checkout must not disappear behind a packet worktree's older copy.
func (c *Context) inputImageGuard(packet *contract.Packet) (string, error) {
	if packet == nil || packet.InputImages == nil {
		return "", nil
	}
	images, err := imageinput.Load(c.Cwd, packet.InputImages)
	if err != nil {
		return "", err
	}
	return sessionHash(imageinput.Describe(images)), nil
}
