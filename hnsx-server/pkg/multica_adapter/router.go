package multica_adapter

import (
	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/pkg/api"
)

// Adapter exposes Multica's REST + WS API contract on top of HnsX's Go control
// plane. It is mounted by HarnessX Server when MulticaMode is enabled.
//
// The adapter does not duplicate business logic — every handler funnels
// through the existing api.Server handlers, translating Multica's JSON shapes
// to and from HnsX's.
type Adapter struct {
	app     *app.Application
	api     *api.Server
	daemons *daemonRegistry
}

// New constructs an Adapter wrapping the given HnsX api.Server.
func New(application *app.Application, apiServer *api.Server) *Adapter {
	return &Adapter{
		app:     application,
		api:     apiServer,
		daemons: newDaemonRegistry(),
	}
}

// Mount registers all Multica-shaped routes on r. Multica routes are
// mounted at their original paths (e.g. /api/workspaces) so Multica's
// Next.js / CLI / Daemon work unchanged.
//
// Accepts gin.IRouter so the api.Server can pass its root engine; we cast
// back to *gin.Engine internally since Multica routes are flat enough that
// IRouter covers everything.
func (a *Adapter) Mount(r gin.IRouter) {
	eng, ok := r.(*gin.Engine)
	if !ok {
		// Fall back to GroupFunc-style registration when a non-engine router
		// is supplied; covers the common case where api.Server passes its
		// root gin.Engine.
		a.mountOnGroup(r.(gin.IRouter))
		return
	}
	a.mountOnGroup(eng)
}

func (a *Adapter) mountOnGroup(r gin.IRouter) {
	// /api/me
	r.GET("/api/me", a.GetMe)

	// /api/workspaces (collection + per-id)
	r.GET("/api/workspaces", a.ListWorkspaces)
	r.POST("/api/workspaces", a.CreateWorkspace)
	wg := r.Group("/api/workspaces/:id")
	{
		wg.GET("", a.GetWorkspace)
		wg.GET("/members", a.ListMembers)
		wg.POST("/leave", a.LeaveWorkspace)
		wg.PUT("", a.UpdateWorkspace)
		wg.PATCH("", a.UpdateWorkspace)
	}

	// /api/agents — collection lives under workspace, but Multica also
	// exposes /api/agent-templates and /api/agent-task-snapshot.
	wg2 := r.Group("/api/workspaces/:id")
	{
		wg2.GET("/agents", a.ListAgents)
		wg2.POST("/agents", a.CreateAgent)
		wg2.GET("/agents/:agentId", a.GetAgent)
		wg2.PATCH("/agents/:agentId", a.UpdateAgent)
		wg2.DELETE("/agents/:agentId", a.DeleteAgent)
		wg2.GET("/agent-templates", a.ListAgentTemplates)
	}

	// /api/issues — collection + per-id.
	ig := r.Group("/api/issues")
	{
		ig.GET("", a.ListIssues)
		ig.POST("", a.CreateIssue)
		ip := ig.Group(":id")
		{
			ip.GET("", a.GetIssue)
			ip.PATCH("", a.UpdateIssue)
			ip.DELETE("", a.DeleteIssue)
			ip.POST("/assign", a.AssignIssue)
			ip.GET("/comments", a.ListComments)
			ip.POST("/comments", a.CreateComment)
		}
		ig.POST("/:id/squad-evaluated", a.RecordSquadLeaderEvaluation)
	}

	// /api/squads — collection + per-id.
	sg := r.Group("/api/squads")
	{
		sg.GET("", a.ListSquads)
		sg.POST("", a.CreateSquad)
		sp := sg.Group(":id")
		{
			sp.GET("", a.GetSquad)
			sp.PATCH("", a.UpdateSquad)
			sp.DELETE("", a.DeleteSquad)
			sp.GET("/members", a.ListSquadMembers)
			sp.POST("/members", a.AddSquadMember)
		}
	}

	// /api/daemon — daemon protocol (HTTP + WS).
	dg := r.Group("/api/daemon")
	{
		dg.POST("/register", a.DaemonRegister)
		dg.POST("/heartbeat", a.DaemonHeartbeat)
		dg.POST("/deregister", a.DaemonDeregister)
		dg.GET("/ws", a.DaemonWebSocket)
		dg.GET("/workspaces/:workspaceId/repos", a.GetWorkspaceRepos)
		dg.POST("/runtimes/:runtimeId/tasks/claim", a.ClaimTask)
		dg.POST("/runtimes/:runtimeId/tasks/:taskId/complete", a.CompleteTask)
		dg.POST("/runtimes/:runtimeId/tasks/:taskId/fail", a.FailTask)
		dg.POST("/runtimes/:runtimeId/tasks/:taskId/progress", a.ReportProgress)
		dg.POST("/runtimes/:runtimeId/tasks/:taskId/messages", a.ReportTaskMessage)
		dg.POST("/runtimes/:runtimeId/tasks/:taskId/usage", a.ReportTaskUsage)
	}

	// /api/autopilots — minimal surface for P0 smoke.
	ap := r.Group("/api/autopilots")
	{
		ap.GET("", a.ListAutopilots)
		ap.POST("", a.CreateAutopilot)
	}

	// /api/chat — minimal surface (Next.js references it on dashboards).
	r.GET("/api/chat/pinned-agents", a.ListChatPinnedAgents)

	// /api/harnessx/domains — HnsX Domain registry (W13). Multica's UI
	// surfaces this as a tab inside an agent or squad detail page; the
	// shape mirrors Multica's AgentResponse so it can render side-by-side.
	r.GET("/api/harnessx/domains", a.ListDomains)
	r.POST("/api/harnessx/domains", a.RegisterDomain)
	r.GET("/api/harnessx/domains/:id", a.GetDomain)
	r.DELETE("/api/harnessx/domains/:id", a.DeleteDomain)
	r.POST("/api/harnessx/domains/:id/run", a.RunDomain)

	// /api/harnessx/approvals — approval center (W6/W11).
	r.GET("/api/harnessx/approvals", a.ListApprovals)
	r.POST("/api/harnessx/approvals/:id/approve", a.ApproveApproval)
	r.POST("/api/harnessx/approvals/:id/reject", a.RejectApproval)

	// /api/harnessx/cost/dashboard — team cost dashboard.
	r.GET("/api/harnessx/cost/dashboard", a.CostDashboard)

	// /api/harnessx/audit — audit log export.
	r.GET("/api/harnessx/audit", a.AuditLog)
}
