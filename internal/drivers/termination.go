package drivers

// These errors are reported by the worker after it has rejected advisory
// metadata, cancelled the same adapter call and awaited its cleanup receipt.
// A valid report is not damaged transport: independent receipt facts remain
// useful, while the cancellation used for cleanup is not the initiating cause.
func machineWorkerObservationError(code string) bool {
	switch code {
	case "INVALID_ACTIVITY", "INVALID_NATIVE_EVENT", "OUTPUT_LIMIT":
		return true
	default:
		return false
	}
}

// mergeWorkerTermination keeps the initiating cause separate from the cancel
// mechanism used to stop a child. A watchdog timeout commonly produces a
// perfectly valid cancelled receipt; that receipt does not make it a user stop.
// An actual cancellation of the parent's context remains authoritative.
func mergeWorkerTermination(out *Result, cancelled, timedOut, truncated bool) {
	out.TimedOut = out.TimedOut || timedOut
	out.Truncated = out.Truncated || truncated
	out.Cancelled = out.Cancelled || (cancelled && !out.TimedOut && !out.Truncated)
}

func timeoutMessage(label string, out Result) string {
	if out.IdleTimedOut {
		return label + " idle timeout: no observable provider activity within the configured interval"
	}
	if out.WallTimedOut {
		return label + " wall timeout: the configured invocation deadline elapsed"
	}
	return label + " timed out in the provider runtime"
}
