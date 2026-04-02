package translations

// Canonical translation key constants for all labeled strings in the GLITCH UI.
// Use these constants when looking up translations to avoid typo-prone raw
// string literals scattered across the codebase.
const (
	// ── Panel titles ─────────────────────────────────────────────────────────
	KeyPipelinesTitle    = "pipelines_panel_title"
	KeyAgentRunnerTitle  = "agent_runner_panel_title"
	KeySignalBoardTitle  = "signal_board_panel_title"
	KeyActivityFeedTitle = "activity_feed_panel_title"
	KeyInboxTitle        = "inbox_panel_title"
	KeyCronTitle         = "cron_panel_title"

	// ── Header / modal titles ─────────────────────────────────────────────────
	KeyDeckHeader = "deck_header_title"
	KeyQuitModalTitle    = "quit_modal_title"
	KeyHelpModalTitle    = "help_modal_title"
	KeyThemePickerTitle  = "theme_picker_title"

	// ── Quit modal ────────────────────────────────────────────────────────────
	KeyQuitConfirmMessage = "quit_modal_message"

	// ── Help modal body ───────────────────────────────────────────────────────
	KeyHelpChordNote        = "help_chord_note"
	KeyHelpSectionSystem    = "help_section_system"
	KeyHelpSectionWorkspace = "help_section_workspace"
	KeyHelpSectionWindows   = "help_section_windows"
	KeyHelpSectionNav       = "help_section_nav"
	KeyHelpSectionPanels    = "help_section_panels"

	// Help key binding descriptions
	KeyHelpBindHelp    = "help_bind_help"
	KeyHelpBindQuit    = "help_bind_quit"
	KeyHelpBindDetach  = "help_bind_detach"
	KeyHelpBindReload  = "help_bind_reload"
	KeyHelpBindThemes  = "help_bind_themes"
	KeyHelpBindJump    = "help_bind_jump"
	KeyHelpBindNewWin  = "help_bind_new_win"
	KeyHelpBindPrevWin = "help_bind_prev_win"
	KeyHelpBindSplitR  = "help_bind_split_right"
	KeyHelpBindNavPane = "help_bind_nav_pane"
	KeyHelpBindKill    = "help_bind_kill"
	KeyHelpBindTabNav  = "help_bind_tab_nav"
	KeyHelpBindEnter   = "help_bind_enter"
	KeyHelpBindEsc     = "help_bind_esc"

	// Help panel descriptions
	KeyHelpPanelPipelines    = "help_panel_pipelines"
	KeyHelpPanelAgentRunner  = "help_panel_agent_runner"
	KeyHelpPanelSignalBoard  = "help_panel_signal_board"
	KeyHelpPanelActivityFeed = "help_panel_activity_feed"
	KeyHelpPanelCron         = "help_panel_cron"

	// ── Theme picker ──────────────────────────────────────────────────────────
	KeyThemePickerDarkTab  = "theme_picker_dark_tab"
	KeyThemePickerLightTab = "theme_picker_light_tab"

	// ── Rerun modal ───────────────────────────────────────────────────────────
	KeyRerunContextLabel = "rerun_context_label"
	KeyRerunCwdLabel     = "rerun_cwd_label"

	// ── Welcome onboarding phases (scripted fallback) ─────────────────────────
	KeyWelcomePhaseIntro     = "welcome_phase_intro"
	KeyWelcomePhaseUseCase   = "welcome_phase_use_case"
	KeyWelcomePhaseProviders = "welcome_phase_providers"
	KeyWelcomePhasePipeline  = "welcome_phase_pipeline"
	KeyWelcomePhaseNav       = "welcome_phase_nav"
	KeyWelcomePhaseBrain     = "welcome_phase_brain"
	KeyWelcomePhaseDone      = "welcome_phase_done"
)
