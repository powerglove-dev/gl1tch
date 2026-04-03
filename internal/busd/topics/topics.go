// Package topics defines canonical busd event topic constants for pipeline
// and cron lifecycle events. Import this package instead of using string
// literals for topic names.
package topics

const (
	// Pipeline run lifecycle
	RunStarted   = "pipeline.run.started"
	RunCompleted = "pipeline.run.completed"
	RunFailed    = "pipeline.run.failed"

	// Pipeline step lifecycle
	StepStarted = "pipeline.step.started"
	StepDone    = "pipeline.step.done"
	StepFailed  = "pipeline.step.failed"

	// Cron job lifecycle
	CronJobStarted   = "cron.job.started"
	CronJobCompleted = "cron.job.completed"

	// Cron entry management
	CronEntryUpdated = "cron.entry.updated"

	// Agent run lifecycle
	AgentRunStarted   = "agent.run.started"
	AgentRunCompleted = "agent.run.completed"
	AgentRunFailed    = "agent.run.failed"

	// Agent clarification request / reply
	ClarificationRequested = "agent.run.clarification"
	ClarificationReply     = "agent.run.clarification.reply"

	// Game run scoring
	GameRunScored = "game.run.scored"

	// Game event signals
	GameAchievementUnlocked = "game.achievement.unlocked"
	GameICEEncountered       = "game.ice.encountered"
	GameQuestEvent           = "game.quest.event"
	GameBountyCompleted      = "game.bounty.completed"

	// Workflow run lifecycle
	WorkflowRunStarted   = "workflow.run.started"
	WorkflowRunCompleted = "workflow.run.completed"
	WorkflowRunFailed    = "workflow.run.failed"

	// Workflow step lifecycle
	WorkflowStepStarted = "workflow.step.started"
	WorkflowStepDone    = "workflow.step.done"
	WorkflowStepFailed  = "workflow.step.failed"

	// Supervisor notification topics
	NotificationErrorDiagnosed    = "notification.error.diagnosed"
	NotificationAgentLoopComplete = "notification.agent.loop.completed"
)
