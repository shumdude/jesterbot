package constants

const (
	SceneMenu                 = "menu"
	SceneRegName              = "reg_name"
	SceneRegOffset            = "reg_offset"
	SceneRegMorning           = "reg_morning"
	SceneAddActivity          = "activity_add"
	SceneEditActivity         = "activity_edit"
	SceneSetActivityTimes     = "activity_times"
	SceneSetActivityWindow    = "activity_window"
	SceneUpdateMorning        = "settings_morning"
	SceneUpdateDayEnd         = "settings_day_end"
	SceneUpdateReminder       = "settings_interval"
	SceneUpdateTick           = "settings_tick"
	SceneUpdateOneOffReminder = "settings_oneoff"
	SceneAddOneOffTitle       = "oneoff_title"
	SceneAddOneOffPriority    = "oneoff_priority"
	SceneAddOneOffItems       = "oneoff_items"
)

const (
	HandlerRegistrationName     = "registration_name"
	HandlerRegistrationOffset   = "registration_offset"
	HandlerRegistrationMorning  = "registration_morning"
	HandlerOpenToday            = "open_today"
	HandlerOpenActivities       = "open_activities"
	HandlerOpenOneOff           = "open_oneoff"
	HandlerOpenSettings         = "open_settings"
	HandlerOpenStats            = "open_stats"
	HandlerFinishDay            = "finish_day"
	HandlerBackActivityDetail   = "back_activity_detail"
	HandlerBackOneOffPriority   = "back_oneoff_priority"
	HandlerAddActivity          = "activity_add_input"
	HandlerEditActivity         = "activity_edit_input"
	HandlerSetActivityTimes     = "activity_times_input"
	HandlerSetActivityWindow    = "activity_window_input"
	HandlerUpdateMorning        = "settings_morning_input"
	HandlerUpdateDayEnd         = "settings_day_end_input"
	HandlerUpdateReminder       = "settings_interval_input"
	HandlerUpdateTick           = "settings_tick_input"
	HandlerUpdateOneOffReminder = "settings_oneoff_input"
	HandlerOneOffTitle          = "oneoff_title_input"
	HandlerOneOffItems          = "oneoff_items_input"
	HandlerOneOffNoItems        = "oneoff_items_none"
)

const (
	ShowIfRegistered   = "registered"
	ShowIfUnregistered = "unregistered"
)

const (
	NSRegistration = "reg"
	KeyName        = "name"
	KeyUTCOffset   = "utc_offset"
)

const (
	NSActivity      = "activity"
	KeyActivityID   = "activity_id"
	KeyActivityPage = "page"
)

const (
	NSOneOff     = "oneoff"
	KeyTaskTitle = "title"
	KeyPriority  = "priority"
)
