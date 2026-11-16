package diagnostic

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ckanner/flywheel/internal/config"
	"github.com/ckanner/flywheel/internal/drivers"
	"github.com/ckanner/flywheel/internal/watchdog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dshNativeDoctorResponse = `{"protocol":1,"type":"native-doctor","harness":"dsh","nativeVersion":1,"available":true,"composition":{"package":"dsh-preset-generator","version":"0.4.0","protocol":1,"available":true,"presets":["daily","heavy"],"executor":"ptc","namedPresetMounted":false},"lcu":{"package":"dsh-plugin-lcu","version":"0.5.0","managedProtocol":1,"available":true},"inferenceTested":false,"admissionTested":false,"nativeAdmission":"requires-selected-sdk-runtime-and-effective-capability-probe"}`
const kimiNativeDoctorResponse = `{"protocol":1,"type":"doctor","available":true,"cliVersion":"1.18.0","backendProtocol":"kimi-server/v1","inferenceTested":false,"nativeAdmission":"requires-actual-server-schema-and-effective-session-probe","recoverySupported":false}`

func TestNativeDoctorSeparatesLocalInterfacesFromRuntimeAdmission(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{
 "permissions":{"mode":"full-access"},
 "native-profiles":{
   "daily":{"harness":"dsh","options":{"version":1,"preset":"daily"}},
   "heavy":{"harness":"dsh","options":{"version":1,"preset":"heavy","agents":{"enabled":true},"lcu":{"enabled":true}}},
   "codex":{"harness":"codex-pro","options":{"version":1,"agents":{"enabled":true}}},
   "claude":{"harness":"claude-code-pro","options":{"version":1,"workflows":{"enabled":true}}},
   "kimi":{"harness":"kimi","options":{"version":1,"mode":"swarm"}}
 },
 "loops":{
   "plan":{"harness":"dsh","provider":"p","model":"m","channel":"c","native-profile":"daily"},
   "intake":{"harness":"dsh","provider":"p","model":"m","channel":"c","native-profile":"heavy"},
   "review":{"harness":"codex-pro","model":"m","channel":"c","native-profile":"codex"},
   "garden":{"harness":"claude-code-pro","model":"m","channel":"c","native-profile":"claude"}
 },
 "routes":{"default":{"harness":"kimi","model":"m","channel":"c","native-profile":"kimi"},
   "second":{"harness":"kimi","model":"m","channel":"c","native-profile":"kimi"}}
}`)
	t.Setenv(drivers.KimiCommandEnv, "/custom/kimi with spaces")
	t.Setenv("NATIVE_DOCTOR_PRIVATE_KEY", "must-not-leak")
	for _, probe := range []bool{false, true} {
		runtime.commands = nil
		runtime.command = func(call commandCall) watchdog.Result {
			if len(call.args) == 1 && (call.args[0] == "--doctor" || call.args[0] == "--native-doctor") {
				assert.Nil(t, call.opts.InteractStdin, "import checks cannot send model prompts")
				assert.NotContains(t, strings.Join(call.opts.Env, "\n"), "must-not-leak")
				response := map[string]string{
					drivers.DSHCommand():           dshNativeDoctorResponse,
					drivers.CodexProCommand():      strings.Replace(codexProDoctorResponse, `"machineProtocol":1`, `"machineProtocol":1,"nativeVersion":1`, 1),
					drivers.ClaudeCodeProCommand(): strings.Replace(claudeCodeProDoctorResponse, `"machineProtocol":1`, `"machineProtocol":1,"nativeVersion":1`, 1),
					drivers.KimiNativeCommand():    kimiNativeDoctorResponse,
				}[call.binary]
				require.NotEmpty(t, response)
				if call.binary == drivers.KimiNativeCommand() {
					assert.Contains(t, call.opts.Env, drivers.KimiCommandEnv+"=/custom/kimi with spaces")
				}
				return watchdog.Result{ExitCode: new(0), Stdout: response}
			}
			return defaultCommand(t, root, call)
		}
		report := Doctor(context.Background(), root, "", probe)
		require.True(t, report.OK, "%+v", report.Checks)
		assert.False(t, report.InferenceTested)
		assert.Empty(t, runtime.probes, "--probe must not advertise legacy SDK initialization as native admission")
		for _, binding := range report.Configuration.Bindings {
			require.NotEmpty(t, binding.NativeProfile)
			assert.Equal(t, "pass", checkByID(t, report, binding.ID+".native.manifest").Status)
			assert.Equal(t, "unknown", checkByID(t, report, binding.ID+".native.admission").Status)
		}
		assert.Equal(t, "skipped", checkByID(t, report, "loops.plan.sdk").Status)
		assert.Equal(t, "skipped", checkByID(t, report, "loops.intake.sdk").Status)
		assert.Equal(t, "pass", checkByID(t, report, "loops.intake.native.lcu").Status)
		counts := map[string]int{}
		for _, call := range runtime.commands {
			if call.args[len(call.args)-1] == "--doctor" || call.args[len(call.args)-1] == "--native-doctor" {
				counts[call.binary]++
			}
		}
		for _, worker := range []string{drivers.DSHCommand(), drivers.CodexProCommand(), drivers.ClaudeCodeProCommand(), drivers.KimiNativeCommand()} {
			assert.Equal(t, 1, counts[worker], "interface imports are deduplicated independently from profile admission")
		}
	}
}

func TestDSHNativeDoctorRejectsUnverifiedManifestFacts(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{}`)
	for _, tc := range []struct {
		name   string
		result watchdog.Result
	}{
		{"malformed", watchdog.Result{ExitCode: new(0), Stdout: "private-loader-detail"}},
		{"nonzero", watchdog.Result{ExitCode: new(1), Stdout: dshNativeDoctorResponse}},
		{"truncated", watchdog.Result{ExitCode: new(0), Stdout: dshNativeDoctorResponse, Truncated: true}},
		{"cleanup unconfirmed", watchdog.Result{ExitCode: new(0), Stdout: dshNativeDoctorResponse, CleanupError: "private-loader-detail"}},
		{"inference happened", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(dshNativeDoctorResponse, `"inferenceTested":false`, `"inferenceTested":true`, 1)}},
		{"admission incorrectly claimed", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(dshNativeDoctorResponse, `"admissionTested":false`, `"admissionTested":true`, 1)}},
		{"missing admission assertion", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(dshNativeDoctorResponse, `,"admissionTested":false`, "", 1)}},
		{"desktop preset claimed", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(dshNativeDoctorResponse, `"namedPresetMounted":false`, `"namedPresetMounted":true`, 1)}},
		{"wrong composition version", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(dshNativeDoctorResponse, `0.4.0`, `0.3.0`, 1)}},
		{"missing heavy preset", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(dshNativeDoctorResponse, `["daily","heavy"]`, `["daily"]`, 1)}},
		{"wrong executor", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(dshNativeDoctorResponse, `"ptc"`, `"native"`, 1)}},
		{"native version absent", watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(dshNativeDoctorResponse, `,"nativeVersion":1`, "", 1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime.commands = nil
			runtime.command = func(call commandCall) watchdog.Result {
				if call.args[len(call.args)-1] == "--native-doctor" {
					return tc.result
				}
				return defaultCommand(t, root, call)
			}
			r := Report{OK: true, Cwd: root}
			cache := map[string]bool{}
			cfg := &config.Config{NativeProfiles: map[string]config.NativeProfile{"p": {Harness: "dsh", Options: json.RawMessage(`{"version":1,"preset":"heavy"}`)}}}
			b := Binding{ID: "loops.plan", Harness: "dsh", NativeProfile: "p"}
			r.nativeProfile(context.Background(), cfg, b, cache)
			assert.False(t, r.OK)
			assert.Equal(t, "fail", checkByID(t, r, "loops.plan.native.manifest").Status)
			assert.Equal(t, "unknown", checkByID(t, r, "loops.plan.native.admission").Status)
			before := len(runtime.commands)
			r.nativeProfile(context.Background(), cfg, b, cache)
			assert.Len(t, runtime.commands, before, "a failed import is not retried for every stage")
			raw, err := json.Marshal(r)
			require.NoError(t, err)
			assert.NotContains(t, string(raw), "private-loader-detail")
		})
	}
}

func TestDSHNativeDoctorRequiresLCUOnlyForAnEnabledProfile(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{}`)
	runtime.command = func(call commandCall) watchdog.Result {
		if call.args[len(call.args)-1] == "--native-doctor" {
			return watchdog.Result{ExitCode: new(0), Stdout: strings.Replace(dshNativeDoctorResponse, `"version":"0.5.0"`, `"version":"0.4.0"`, 1)}
		}
		return defaultCommand(t, root, call)
	}
	for _, options := range []string{`{"version":1,"preset":"heavy","lcu":{"enabled":false}}`, `{"version":1,"preset":"heavy","lcu":{"enabled":true}}`, `{`} {
		r := Report{OK: true, Cwd: root}
		cfg := &config.Config{NativeProfiles: map[string]config.NativeProfile{"p": {Harness: "dsh", Options: json.RawMessage(options)}}}
		r.nativeProfile(context.Background(), cfg, Binding{ID: "r", Harness: "dsh", NativeProfile: "p"}, map[string]bool{})
		assert.Equal(t, strings.Contains(options, "false"), r.OK)
		if strings.Contains(options, "true") {
			assert.Equal(t, "fail", checkByID(t, r, "r.native.lcu").Status)
		}
	}
}

func TestKimiNativeDoctorDoesNotTreatCLIVersionAsSessionAdmission(t *testing.T) {
	root, runtime := diagnosticFixture(t, `{}`)
	t.Setenv(drivers.KimiCommandEnv, "")
	for _, response := range []string{
		`private-command-output`,
		strings.Replace(kimiNativeDoctorResponse, `"backendProtocol":"kimi-server/v1"`, `"backendProtocol":"other"`, 1),
		strings.Replace(kimiNativeDoctorResponse, `"cliVersion":"1.18.0"`, `"cliVersion":"product 1.18.0"`, 1),
		strings.Replace(kimiNativeDoctorResponse, `"inferenceTested":false`, `"inferenceTested":true`, 1),
		strings.Replace(kimiNativeDoctorResponse, `,"inferenceTested":false`, "", 1),
		strings.Replace(kimiNativeDoctorResponse, `"recoverySupported":false`, `"recoverySupported":true`, 1),
		strings.Replace(kimiNativeDoctorResponse, `requires-actual-server-schema-and-effective-session-probe`, `passed`, 1),
		kimiNativeDoctorResponse,
	} {
		runtime.commands = nil
		runtime.command = func(call commandCall) watchdog.Result {
			if call.args[len(call.args)-1] == "--doctor" {
				return watchdog.Result{ExitCode: new(0), Stdout: response}
			}
			return defaultCommand(t, root, call)
		}
		r := Report{OK: true, Cwd: root}
		cache := map[string]bool{}
		b := Binding{ID: "r", Harness: "kimi", NativeProfile: "p"}
		r.nativeProfile(context.Background(), &config.Config{}, b, cache)
		assert.Equal(t, response == kimiNativeDoctorResponse, r.OK, response)
		assert.Equal(t, "unknown", checkByID(t, r, "r.native.admission").Status)
		before := len(runtime.commands)
		r.nativeProfile(context.Background(), &config.Config{}, b, cache)
		assert.Len(t, runtime.commands, before)
		raw, err := json.Marshal(r)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "private-command-output")
	}
}

func TestNativeDoctorHonorsMissingWorkersAndCancellation(t *testing.T) {
	for _, harness := range []string{"dsh", "kimi"} {
		for _, condition := range []string{"worker", "node", "cancelled", "cancel during version"} {
			t.Run(harness+"/"+condition, func(t *testing.T) {
				root, runtime := diagnosticFixture(t, `{}`)
				worker := drivers.DSHCommand()
				if harness == "kimi" {
					worker = drivers.KimiNativeCommand()
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				switch condition {
				case "worker":
					runtime.missing[worker] = true
				case "node":
					runtime.missing["node"] = true
				case "cancelled":
					cancel()
				case "cancel during version":
					runtime.command = func(call commandCall) watchdog.Result { cancel(); return defaultCommand(t, root, call) }
				}
				r := Report{OK: true, Cwd: root}
				cache := map[string]bool{}
				if harness == "kimi" {
					assert.False(t, r.kimiNative(ctx, cache))
				} else {
					assert.False(t, r.dshNative(ctx, cache))
				}
				for _, call := range runtime.commands {
					assert.Equal(t, []string{"--version"}, call.args)
				}
				assert.Empty(t, runtime.probes)
			})
		}
	}
}

func TestProNativeDoctorRequiresTheVersionedNativeInterface(t *testing.T) {
	for _, harness := range []string{"codex-pro", "claude-code-pro"} {
		for _, nativeVersion := range []string{"", `,"nativeVersion":2`, `,"nativeVersion":1`} {
			t.Run(harness+nativeVersion, func(t *testing.T) {
				root, runtime := diagnosticFixture(t, `{}`)
				response := codexProDoctorResponse
				if harness == "claude-code-pro" {
					response = claudeCodeProDoctorResponse
				}
				response = strings.Replace(response, `"machineProtocol":1`, `"machineProtocol":1`+nativeVersion, 1)
				runtime.command = func(call commandCall) watchdog.Result {
					if call.args[len(call.args)-1] == "--doctor" {
						return watchdog.Result{ExitCode: new(0), Stdout: response}
					}
					return defaultCommand(t, root, call)
				}
				r := Report{OK: true, Cwd: root}
				cache := map[string]bool{}
				b := Binding{ID: "r", Harness: harness, NativeProfile: "p", PermissionMode: "danger-full-access"}
				if harness == "codex-pro" {
					r.codexPro(context.Background(), b, cache)
				} else {
					r.claudeCodePro(context.Background(), b, cache)
				}
				assert.True(t, r.OK, "the non-native public machine API remains available")
				r.nativeProfile(context.Background(), &config.Config{}, b, cache)
				assert.Equal(t, nativeVersion == `,"nativeVersion":1`, r.OK)
				assert.Equal(t, "unknown", checkByID(t, r, "r.native.admission").Status)
			})
		}
	}
	r := Report{OK: true}
	r.nativeProfile(context.Background(), &config.Config{}, Binding{ID: "r", Harness: "unsupported", NativeProfile: "p"}, map[string]bool{})
	assert.False(t, r.OK)
}

func TestNativeKimiSessionInspectionMatchesEffectiveHistorySelection(t *testing.T) {
	for _, tc := range []struct {
		name, native, configured, want string
		resident, roleFresh, supported bool
	}{
		{"native agent resident", "agent", "fresh", "scoped", true, false, true},
		{"native swarm resident", "swarm", "fresh", "scoped", true, false, true},
		{"native ordinary scope", "agent", "scoped", "scoped", true, false, true},
		{"native resident disabled", "agent", "scoped", "fresh", false, false, true},
		{"explicit fresh wins", "swarm", "scoped", "fresh", true, true, true},
		{"explicit fresh over resident", "agent", "fresh", "fresh", true, true, true},
		{"tower no resume", "tower", "scoped", "fresh", true, false, false},
		{"tower no resident", "tower", "fresh", "fresh", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Sessions: config.SessionsConfig{Mode: tc.configured}, NativeProfiles: map[string]config.NativeProfile{
				"native": {Harness: "kimi", Options: json.RawMessage(`{"version":1,"mode":"` + tc.native + `"}`)},
			}}
			if tc.roleFresh {
				cfg.Sessions.Roles = map[string]string{"plan": "fresh"}
			}
			binding := config.LoopBinding{Harness: "kimi", NativeProfile: "native", ResidentSession: tc.resident, ResidentSessionSet: true}
			policy := sessionPolicy(cfg, "plan", "kimi", binding)
			require.Equal(t, tc.want, policy.Mode)
			require.Equal(t, tc.supported, policy.Supported)
			require.NotEqual(t, "legacy-plan", policy.Mode)
			if tc.native == "tower" {
				require.Equal(t, "native-tower-fresh-only", policy.Reason)
			}
			if tc.want == "scoped" && tc.configured == "fresh" {
				require.Equal(t, "native-kimi-plan-resident", policy.Reason)
			}
		})
	}
	for _, profile := range []config.NativeProfile{
		{}, {Harness: "codex-pro", Options: json.RawMessage(`{"version":1,"mode":"agent"}`)},
		{Harness: "kimi", Options: json.RawMessage(`{`)}, {Harness: "kimi", Options: json.RawMessage(`{"version":2,"mode":"agent"}`)},
		{Harness: "kimi", Options: json.RawMessage(`{"version":1,"mode":"unknown"}`)},
	} {
		cfg := &config.Config{NativeProfiles: map[string]config.NativeProfile{"native": profile}}
		policy := sessionPolicy(cfg, "plan", "kimi", config.LoopBinding{NativeProfile: "native"})
		require.False(t, policy.Supported)
		require.Equal(t, "invalid-native-profile", policy.Reason)
	}
	root, _ := diagnosticFixture(t, `{"sessions":{"mode":"fresh","policy-version":1},"native-profiles":{"native":{"harness":"kimi","options":{"version":1,"mode":"agent"}}},"loops":{"plan":{"harness":"kimi","model":"m","channel":"c","native-profile":"native","resident-session":true}}}`)
	view, err := Inspect(root, "")
	require.NoError(t, err)
	require.Len(t, view.Bindings, 2)
	binding := bindingByID(t, view, "loops.plan")
	require.Equal(t, "scoped", binding.Session.Mode)
	require.Equal(t, "native-kimi-plan-resident", binding.Session.Reason)
}
