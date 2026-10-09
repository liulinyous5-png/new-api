package dto

type Notify struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Values  []any  `json:"values"`
}

const ContentValueParam = "{{value}}"

const (
	NotifyTypeQuotaExceed             = "quota_exceed"
	NotifyTypeChannelUpdate           = "channel_update"
	NotifyTypeChannelTest             = "channel_test"
	NotifyTypeTTFTAlert               = "ttft_alert"
	NotifyTypeChannelAutoDisableAlert = "channel_auto_disable_alert"
	NotifyTypeChannelErrorAlert       = "channel_error_alert"
)

func NewNotify(t string, title string, content string, values []any) Notify {
	return Notify{
		Type:    t,
		Title:   title,
		Content: content,
		Values:  values,
	}
}
