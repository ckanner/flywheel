package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ckanner/flywheel/templates"
)

// MigrationOptions selects the same complete file as LoadReadOnly. Cwd and
// TaskDir are already physical directories, with Cwd at the Git project root.
type MigrationOptions struct {
	Preset       string
	Cwd          string
	TaskDir      string
	Apply        bool
	ExpectSHA256 string
}

type MigrationChange struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

// Raw configuration and backup contents never enter the report. The diagnostic
// layer renders only the same public binding fields as config inspect.
type Migration struct {
	Protocol        int               `json:"protocol"`
	Kind            string            `json:"kind"`
	Preset          string            `json:"preset"`
	Source          string            `json:"source"`
	Scope           string            `json:"scope"`
	Target          string            `json:"target"`
	SHA256          string            `json:"sha256"`
	CandidateSHA256 string            `json:"candidateSha256,omitempty"`
	Changed         bool              `json:"changed"`
	Applied         bool              `json:"applied"`
	Backup          string            `json:"backup,omitempty"`
	Changes         []MigrationChange `json:"changes"`
	Blockers        []string          `json:"blockers"`
	Notes           []string          `json:"notes"`
	Original        *Config           `json:"-"`
	Candidate       *Config           `json:"-"`
}

type migrationFile interface {
	io.WriteCloser
	Name() string
	Chmod(os.FileMode) error
	Sync() error
}

// Narrow seams exercise disk/close/publication failures against real files.
var (
	createMigrationTemp      = func(dir, pattern string) (migrationFile, error) { return os.CreateTemp(dir, pattern) }
	renameMigrationFile      = os.Rename
	linkMigrationFile        = os.Link
	removeMigrationFile      = os.Remove
	readMigrationFile        = os.ReadFile
	absMigrationPath         = filepath.Abs
	legacySubscriptionsTable = func() ([]byte, error) { return templates.FS.ReadFile("flywheel.legacy-subscriptions.json") }
)

func migrationHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func migrationObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	value := map[string]json.RawMessage{}
	if len(raw) != 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, errors.New("configuration sections must be JSON objects")
		}
	}
	if value == nil {
		value = map[string]json.RawMessage{}
	}
	return value, nil
}

func migrationNote(key string) bool { return key == "_note" || strings.HasPrefix(key, "_note_") }

func migrationSet(object map[string]json.RawMessage, key, path string, value json.RawMessage, changes *[]MigrationChange) {
	before := object[key]
	var a, b bytes.Buffer
	// Both values originate in parsed JSON; Compact cannot fail. Compare scalar
	// fields, not serialized whole documents, so formatting never causes a rewrite.
	_ = json.Compact(&a, before)
	_ = json.Compact(&b, value)
	if bytes.Equal(a.Bytes(), b.Bytes()) {
		return
	}
	action := "set"
	if value == nil {
		delete(object, key)
		action = "remove"
	} else {
		object[key] = value
	}
	*changes = append(*changes, MigrationChange{Path: path, Action: action})
}

func migrationScope(source, taskDir, cwd string) string {
	switch source {
	case "(embedded legal table)":
		return "embedded"
	case filepath.Join(taskDir, "flywheel.config.json"):
		return "task"
	case filepath.Join(cwd, "flywheel.config.json"):
		return "project"
	default:
		return "global"
	}
}

// Migrate is deliberately separate from installation and loading. Only Apply
// writes; unknown/custom routes remain a reviewable blocker, never a guessed
// model substitution. Existing unused channels/profiles remain user-owned.
func Migrate(options MigrationOptions) (report *Migration, resultErr error) {
	report = &Migration{Protocol: 1, Kind: "config-migration", Preset: options.Preset,
		Changes: []MigrationChange{}, Blockers: []string{}, Notes: []string{
			"仅更新明确列出的模型、渠道和权限配置；账号授权、额度与模型服务能力未验证。",
			"配置在下次引擎启动时加载；不会停止或重启正在运行的任务。",
			"未被新绑定引用的自定义 profiles/channels 保留，但不会作为自动后备渠道。",
		}}
	if options.Preset != "subscriptions" && options.Preset != "subscription-cli" {
		return report, errors.New("--preset must be subscriptions or subscription-cli")
	}
	presetTable := legacySubscriptionsTable
	if options.Preset == "subscription-cli" {
		presetTable = embeddedTable
		report.Notes[0] = "更新最终订阅 CLI 模型分工；保留已有权限、验证、预算和调度策略。账号权益与余额仍须由原生客户端确认。"
	}
	if options.ExpectSHA256 != "" {
		decoded, err := hex.DecodeString(options.ExpectSHA256)
		if err != nil || len(decoded) != sha256.Size {
			return report, errors.New("--expect-sha256 must be a SHA-256 hexadecimal digest")
		}
	}
	source := FindConfigFile(options.TaskDir, options.Cwd)
	if source == "" {
		var err error
		source, err = GlobalConfigPath()
		if err != nil {
			return report, fmt.Errorf("resolve global migration destination: %w", err)
		}
	}
	// Abs is infallible for an absolute path; otherwise it can fail when cwd was
	// removed. A relative FLYWHEEL_CONFIG retains the loader's process-cwd meaning.
	target, err := absMigrationPath(source)
	if err != nil {
		return report, err
	}
	report.Source, report.Target = target, target
	raw, info, err := readMigrationSource(target)
	if err != nil {
		return report, err
	}
	if info == nil {
		report.Source = "(embedded legal table)"
		raw, err = presetTable()
		if err != nil {
			return report, err
		}
	}
	report.Scope = migrationScope(report.Source, options.TaskDir, options.Cwd)
	report.SHA256 = migrationHash(raw)
	if options.ExpectSHA256 != "" && !strings.EqualFold(options.ExpectSHA256, report.SHA256) {
		return report, errors.New("configuration changed since preview; inspect the new migration before applying")
	}
	root, err := migrationObject(raw)
	if err != nil || len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return report, errors.New("effective configuration is not a JSON object; migration did not change it")
	}
	sections := map[string]map[string]json.RawMessage{}
	for _, section := range []string{"loops", "routes", "channels", "permissions"} {
		sections[section], err = migrationObject(root[section])
		if err != nil {
			return report, err
		}
	}
	routeNames := []string{"default", "frontend", "hard"}
	if options.Preset == "subscription-cli" {
		routeNames = append(routeNames, "complex", "backend", "batch")
	}
	for _, section := range []string{"loops", "routes"} {
		allowed := loopRoles
		if section == "routes" {
			allowed = append(slices.Clone(routeNames), "multimodal")
		}
		for key := range sections[section] {
			if !migrationNote(key) && !slices.Contains(allowed, key) {
				report.Blockers = append(report.Blockers, section+"."+key+": 自定义绑定不在 subscriptions 预设内；请先明确其模型分配")
			}
		}
	}
	slices.Sort(report.Blockers)
	if len(report.Blockers) != 0 {
		return report, nil
	}
	report.Original, err = parseConfig(raw, report.Source)
	if err != nil {
		return report, errors.New("effective configuration is invalid; fix its fields before migrating (contents were not changed)")
	}
	if options.Preset == "subscription-cli" && report.Original.JEV.Enabled && report.Original.JEV.Backend != "subscription" {
		report.Blockers = append(report.Blockers, "jev: enabled HTTP backend conflicts with subscription-only access; disable it before migrating, then review the new subscription target before enabling JEV again")
		return report, nil
	}
	presetRaw, err := presetTable()
	if err != nil {
		return report, err
	}
	preset, err := migrationObject(presetRaw)
	if err != nil {
		return report, err
	}
	if options.Preset == "subscription-cli" {
		if err = migrateSubscriptionCLI(root, preset, &report.Changes); err != nil {
			return report, err
		}
	} else {
		if err = migrateSubscriptions(sections, preset, &report.Changes); err != nil {
			return report, err
		}
		if _, exists := sections["routes"]["multimodal"]; exists {
			migrationSet(sections["routes"], "multimodal", "routes.multimodal", nil, &report.Changes)
			report.Notes = append(report.Notes, "停用旧 multimodal 路由；不会把未经验证的附件任务改派给普通 Flash，仍引用该路由的任务需先重新规划。")
		}
		for _, section := range []string{"loops", "routes", "channels", "permissions"} {
			root[section] = configJSON(sections[section])
		}
		migrationSet(root, "judge-panel", "judge-panel", json.RawMessage("null"), &report.Changes)
	}
	candidate, _ := json.MarshalIndent(root, "", "  ") // RawMessages came from valid JSON only.
	candidate = append(candidate, '\n')
	report.Candidate, err = parseConfig(candidate, target)
	if err != nil {
		return report, errors.New("migration candidate is invalid; original configuration was preserved")
	}
	if options.Preset == "subscription-cli" && report.Candidate.PermissionMode("investigate", "antigravity") != "read-only" {
		report.Notes = append(report.Notes, "permissions.bindings.loops.investigate was preserved; evidence-only Antigravity analysis requires read-only, so review and change this explicit override before running investigation analysis")
	}
	if options.Preset == "subscription-cli" && report.Original.ExecutionPolicy == nil {
		var fixed []string
		for name, binding := range report.Candidate.Routes {
			if report.Candidate.PermissionMode("execute", binding.Harness, "routes."+name) != "danger-full-access" {
				fixed = append(fixed, name)
			}
		}
		if len(fixed) > 0 {
			slices.Sort(fixed)
			report.Notes = append(report.Notes, "execution-policy candidates were not added for routes "+strings.Join(fixed, ", ")+": preserved permission is not danger-full-access; fixed route bindings remain unchanged")
		}
	}
	report.Changed = info == nil || len(report.Changes) > 0
	if !report.Changed {
		candidate = raw // Never reformat a semantically unchanged user file.
	}
	report.CandidateSHA256 = migrationHash(candidate)
	if info == nil {
		report.Changes = append(report.Changes, MigrationChange{Path: "$file", Action: "create"})
	}
	if !options.Apply || !report.Changed {
		return report, nil
	}
	resultErr = applyMigration(report, raw, candidate, info)
	return report, resultErr
}

func migrateSubscriptions(sections map[string]map[string]json.RawMessage, preset map[string]json.RawMessage, changes *[]MigrationChange) error {
	for _, section := range []string{"loops", "routes", "channels", "permissions"} {
		definitions, err := migrationObject(preset[section])
		if err != nil || len(definitions) == 0 {
			return errors.New("embedded subscriptions preset is incomplete")
		}
		for _, name := range slices.Sorted(maps.Keys(definitions)) {
			if migrationNote(name) {
				continue
			}
			if section == "permissions" {
				migrationSet(sections[section], name, section+"."+name, definitions[name], changes)
				continue
			}
			// The original config already passed the ordinary strict loader.
			old, _ := migrationObject(sections[section][name])
			binding, err := migrationObject(definitions[name])
			if err != nil {
				return err
			}
			fields := append(slices.Clone(modelFields), "profile", "resident-session")
			if section == "channels" {
				fields = []string{"env-key", "fallback", "harness", "provider", "models"}
			}
			for _, key := range fields {
				value := binding[key]
				if section == "loops" && name == "plan" && key == "resident-session" && bytes.Equal(bytes.TrimSpace(old[key]), []byte("false")) {
					value = old[key]
				}
				migrationSet(old, key, section+"."+name+"."+key, value, changes)
			}
			sections[section][name] = configJSON(old)
		}
	}
	return nil
}

func readMigrationSource(path string) ([]byte, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, errors.New("migration requires a regular configuration file, not a directory or symbolic link")
	}
	if info.Mode().Perm()&0o444 == 0 {
		return nil, nil, errors.New("configuration file is not readable")
	}
	raw, err := readMigrationFile(path)
	return raw, info, err
}

func unchangedMigrationSource(path string, raw []byte, info os.FileInfo) error {
	current, currentInfo, err := readMigrationSource(path)
	if err != nil {
		return err
	}
	if (info == nil) != (currentInfo == nil) || (info != nil && (!os.SameFile(info, currentInfo) || info.Mode() != currentInfo.Mode() || !bytes.Equal(raw, current))) {
		return errors.New("configuration changed while applying migration; source was not replaced")
	}
	return nil
}

func writeMigrationFile(dir, pattern string, raw []byte, mode os.FileMode) (name string, resultErr error) {
	file, err := createMigrationTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	name = file.Name()
	defer func() {
		_ = file.Close()
		if resultErr != nil {
			_ = removeMigrationFile(name)
		}
	}()
	if err = file.Chmod(mode); err != nil {
		return name, err
	}
	if _, err = io.Copy(file, bytes.NewReader(raw)); err != nil {
		return name, err
	}
	if err = file.Sync(); err != nil {
		return name, err
	}
	return name, file.Close()
}

func applyMigration(report *Migration, raw, candidate []byte, info os.FileInfo) (resultErr error) {
	if info != nil && info.Mode().Perm()&0o222 == 0 {
		return errors.New("configuration file is read-only; migration did not replace it")
	}
	dir := filepath.Dir(report.Target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lockPath := report.Target + ".migrate.lock"
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("cannot acquire migration lock (another migration may be active): %w", err)
	}
	lockInfo, _ := lock.Stat() // A live descriptor returned by OpenFile has an identity.
	_ = lock.Close()
	defer func() {
		current, err := os.Lstat(lockPath)
		if err != nil || !os.SameFile(lockInfo, current) {
			resultErr = errors.Join(resultErr, errors.New("migration lock ownership changed; lock was preserved"))
		} else if err := removeMigrationFile(lockPath); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	if err = unchangedMigrationSource(report.Target, raw, info); err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if info != nil {
		mode = info.Mode().Perm()
		report.Backup, err = writeMigrationFile(dir, filepath.Base(report.Target)+".backup-*", raw, 0o600)
		if err != nil {
			report.Backup = ""
			return fmt.Errorf("backup failed; source was not changed: %w", err)
		}
	}
	temp, err := writeMigrationFile(dir, ".flywheel-migrate-*.tmp", candidate, mode)
	if err != nil {
		return err
	}
	defer func() {
		if err := removeMigrationFile(temp); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	if err = unchangedMigrationSource(report.Target, raw, info); err != nil {
		return err
	}
	if current, err := os.Lstat(lockPath); err != nil || !os.SameFile(lockInfo, current) {
		return errors.New("migration lock ownership changed before publication; source was not replaced")
	}
	if info == nil {
		// Creating an absent global configuration must never replace a racing
		// user's file. Hard-link publication is atomic and exclusive.
		err = linkMigrationFile(temp, report.Target)
	} else {
		// The lock serializes migrations; the second identity+byte comparison
		// catches external edits made while the backup/candidate was written.
		err = renameMigrationFile(temp, report.Target)
	}
	if err != nil {
		return fmt.Errorf("publish migration: %w", err)
	}
	report.Applied = true
	return nil
}
