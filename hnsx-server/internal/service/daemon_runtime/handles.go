package daemon_runtime

// issue_svc_handle is a tiny adapter so the runtime declares its
// service subset as an interface (testable) while callers can pass a
// concrete *issue.Service trivially.
type issue_svc_handle struct {
	IssueListerAndUpdater
}

type agent_svc_handle struct {
	AgentGetter
}