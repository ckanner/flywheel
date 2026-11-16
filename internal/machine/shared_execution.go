package machine

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ckanner/flywheel/internal/contract"
	"github.com/ckanner/flywheel/internal/verify"
)

func (c *Context) sharedPrompt(id string) string {
	if !c.sharedActive(id) {
		return ""
	}
	scopes := map[string]*contract.PacketSchedule{}
	for _, peer := range c.State.SharedExecution.Packets {
		if c.sharedActive(peer) {
			scopes[peer] = c.Packets[peer].Schedule
		}
	}
	raw, _ := json.Marshal(scopes) // validated contract fields only
	return "\n## 共享执行批次\n本批次可能同时在这个目录执行多个独立任务；只修改自己声明的写入范围，保留其他任务的文件。不要在并行阶段自行提交、合并或切换共享目录的分支。全部任务收尾后，Go 会针对共同候选重新测试、独立 Review，并统一提交。若需要独立 Worktree，必须在实现前按执行目录登记协议提出。批次任务与范围：\n" + string(raw) + "\n"
}

func (c *Context) sharedActive(id string) bool {
	group := c.State.SharedExecution
	packet := c.State.Packets[id]
	return group != nil && slices.Contains(group.Packets, id) && packet != nil &&
		(packet.Worktree == nil || packet.Worktree.Mode != "worktree")
}

func (c *Context) sharedInvocationScope(role, id string) ([]string, error) {
	if !c.sharedActive(id) {
		return nil, nil
	}
	for _, peer := range c.State.SharedExecution.Packets {
		if c.sharedActive(peer) && (c.Packets[peer] == nil || c.Packets[peer].Schedule == nil) {
			return nil, fmt.Errorf("shared execution requires the original declared write scope")
		}
	}
	if role != "execute" {
		return nil, nil
	}
	packet := c.Packets[id]
	scope := slices.Clone(packet.Schedule.Writes)
	sort.Strings(scope)
	return scope, nil
}

func (c *Context) sessionCandidateFingerprint(cwd string, scope []string) (string, error) {
	if len(scope) == 0 {
		return c.candidateFingerprint(cwd)
	}
	return c.candidateFingerprint(cwd, func(path string) bool { return !contract.InWriteScope(path, scope) })
}

func (c *Context) sharedVerificationPolicy(packet *contract.Packet) any {
	peers := c.State.Packets[packet.PacketID].SharedPeers
	if len(peers) == 0 {
		return nil
	}
	policy := map[string]*contract.Packet{}
	for _, id := range peers {
		policy[id] = c.Packets[id]
	}
	return policy
}

// Shared files are attributed to declared scopes, never to an observed OS
// writer. The whole cohort union and forbidden paths remain hard boundaries.
func (c *Context) sharedBoundaryFiles(packet *contract.Packet, ordinary []string) ([]string, []verify.BoundaryViolation) {
	if !c.sharedActive(packet.PacketID) {
		return ordinary, nil
	}
	scopes := []string{}
	for _, id := range c.State.SharedExecution.Packets {
		if !c.sharedActive(id) {
			continue
		}
		peer := c.Packets[id]
		if peer == nil || peer.Schedule == nil {
			return nil, []verify.BoundaryViolation{{Type: "shared-scope-unavailable", Path: id}}
		}
		scopes = append(scopes, peer.Schedule.Writes...)
	}
	files, ok := verify.GetChangedFiles(c.Cwd, c.State.SharedExecution.BaseRef)
	if !ok {
		return nil, []verify.BoundaryViolation{{Type: "shared-diff-unavailable"}}
	}
	selected := []string{}
	violations := []verify.BoundaryViolation{}
	for _, file := range files {
		if c.snapshotExcluded(file) {
			continue
		}
		if !contract.InWriteScope(file, scopes) {
			violations = append(violations, verify.BoundaryViolation{Type: "shared-write-scope", Path: file})
		}
		if contract.InWriteScope(file, packet.Schedule.Writes) {
			selected = append(selected, file)
		} else {
			forbidden := verify.CheckBoundaries([]string{file}, contract.Boundaries{MustNotTouch: packet.Boundaries.MustNotTouch}, c.effectiveTestFilePatterns())
			violations = append(violations, forbidden.Violations...)
		}
	}
	return selected, violations
}

func (c *Context) unitAccepted(packet *contract.Packet) bool {
	evidence := c.State.Packets[packet.PacketID].Evidence
	review, _ := evidence.Review.(map[string]any)
	return c.currentL1Evidence(packet) && review["verdict"] == "pass" && evidenceIdentity(evidence.Review) == evidenceIdentity(evidence.L1)
}

// Workers have quiesced before this serial path. Repairs may invalidate an
// earlier peer's proof; nothing is committed until the entire candidate passes.
func (c *Context) processSharedExecution(pauseBefore string) (stopped bool, runErr error) {
	defer func() {
		if runErr != nil && c.fatal == nil && c.State.SharedExecution != nil {
			c.State.SharedExecution.LastError = string([]rune(runErr.Error())[:min(2000, len([]rune(runErr.Error())))])
			if c.State.Status == "running" {
				c.State.Status = "paused"
			}
			c.save()
			runErr = errors.Join(runErr, c.err())
		}
	}()
	if err := c.reconcileSessionReceipts(); err != nil {
		return false, err
	}
	group := c.State.SharedExecution
	if group.Cwd != c.Cwd {
		return false, fmt.Errorf("shared execution directory changed")
	}
	members := []*contract.Packet{}
	ids := []string{}
	for _, id := range group.Packets {
		packet := c.Packets[id]
		if packet == nil {
			return false, fmt.Errorf("shared execution contract %s disappeared", id)
		}
		if !c.sharedActive(id) {
			continue
		}
		if packet.Schedule == nil {
			return false, fmt.Errorf("shared execution scope %s disappeared", id)
		}
		members = append(members, packet)
		ids = append(ids, id)
	}
	if len(members) == 0 {
		c.State.SharedExecution = nil
		c.save()
		return false, c.err()
	}
	changed := !slices.Equal(group.Packets, ids)
	group.Packets = ids
	for _, packet := range members {
		ps := c.State.Packets[packet.PacketID]
		if !slices.Equal(ps.SharedPeers, ids) {
			ps.SharedPeers = slices.Clone(ids)
			changed = true
		}
	}
	if changed {
		c.save()
		if err := c.err(); err != nil {
			return false, err
		}
	}
	for _, packet := range members {
		if c.unitAccepted(packet) {
			continue
		}
		ps := c.State.Packets[packet.PacketID]
		review, _ := ps.Evidence.Review.(map[string]any)
		if review["verdict"] == "pass" && c.doneReportValid(packet) {
			ps.Status = "executed" // peer repair changed the combined candidate; reverify without re-executing this peer
			c.save()
		}
		outcome, err := c.processPacket(packet, processOpts{PauseBefore: pauseBefore, DeferCommit: true})
		return outcome == "stop" || outcome == "pause", err
	}
	return false, c.commitSharedExecution(members)
}

func (c *Context) commitSharedExecution(members []*contract.Packet) error {
	if err := c.admitReviewedBoundary(); err != nil {
		return err
	}
	if err := c.err(); err != nil {
		return err
	}
	for _, packet := range members {
		if !c.unitAccepted(packet) {
			return fmt.Errorf("%w: shared peer is not accepted", ErrCommitFailed)
		}
	}
	before, err := c.candidateFingerprint(c.Cwd)
	if err != nil {
		return err
	}
	messagePacket := *members[0]
	messagePacket.Goal = "deliver shared batch " + strings.Join(c.State.SharedExecution.Packets, ", ")
	committed := c.commitUnitChangesIn(c.Cwd, &messagePacket, c.commitExcludes(), c.State.SharedExecution.BaseRef, "")
	if committed.Error != "" {
		return fmt.Errorf("%w: %s", ErrCommitFailed, committed.Error)
	}
	head, err := verify.PersistedHead(c.Cwd, c.ledgerExcludes())
	if err != nil {
		return fmt.Errorf("%w: shared tree: %v", ErrCommitFailed, err)
	}
	after, err := c.candidateFingerprint(c.Cwd)
	if err != nil || after != before {
		return fmt.Errorf("%w: shared candidate changed during commit", ErrCommitFailed)
	}
	for _, packet := range members {
		if !c.unitAccepted(packet) {
			return fmt.Errorf("%w: shared verification inputs changed during commit", ErrCommitFailed)
		}
	}
	originalState := c.State
	c.State = jsonClone(originalState)
	at := c.nowISO()
	for _, packet := range members {
		ps := c.State.Packets[packet.PacketID]
		ps.Commit = map[string]any{"sha": head, "message": committed.Message, "at": at, "shared-peers": slices.Clone(ps.SharedPeers)}
		ps.Status, ps.Parallel, ps.UpdatedAt = "completed", false, &at
	}
	c.State.SharedExecution = nil
	c.touchProgress()
	c.save() // one CAS publishes every member's completion, never a partial group
	if err := c.err(); err != nil {
		c.State = originalState // Git may have committed; never optimistically report a failed state publication as completed
		return err
	}
	return nil
}
